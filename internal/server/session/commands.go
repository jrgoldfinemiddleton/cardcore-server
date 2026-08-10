package session

import (
	"time"

	"github.com/jrgoldfinemiddleton/cardcore-server/internal/api"
)

// handleCommand dispatches a command to the appropriate handler.
func (s *session) handleCommand(cmd command) {
	switch c := cmd.(type) {
	case playCmd:
		s.handlePlay(c)
	case subscribePlayerCmd:
		s.handleSubscribePlayer(c)
	case subscribeObserverCmd:
		s.handleSubscribeObserver(c)
	case unsubscribeCmd:
		s.handleUnsubscribe(c)
	}
}

// handlePlay processes a human player's action.
func (s *session) handlePlay(c playCmd) {
	defer close(c.resp)

	s.logger.Debug("handlePlay",
		"seat", c.seat,
		"type", c.msg.Type,
		"action_id", c.msg.ActionID,
		"seq", c.msg.Seq,
		"server_seq", s.seq,
	)

	// Pause/resume are session-level meta-commands. Intercept them before
	// seq validation so the same playCmd plumbing can carry them.
	if c.msg.Type == "pause" {
		s.handlePauseCmd(c)
		return
	}
	if c.msg.Type == "resume" {
		s.handleResumeCmd(c)
		return
	}

	if c.msg.Seq < s.seq {
		s.handleStaleSeq(c)
		return
	}

	if cached, ok := s.actionIDs[c.msg.ActionID]; ok {
		s.handleDuplicateAction(c, cached)
		return
	}

	// Reject gameplay commands while paused. This runs after seq validation
	// and action_id dedup so stale commands still resync via stale_seq and
	// replays still return their cached snapshot (ADR-013); only commands
	// that would otherwise reach the engine are rejected.
	if s.paused {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrGamePaused,
			Message:  "game is paused",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}

	res, cmdErr := s.game.HandleAction(c.seat, c.msg)
	if cmdErr != nil {
		s.handleRejectedAction(c, cmdErr)
		return
	}

	s.handleAcceptedAction(c, res)
}

// handleStaleSeq responds to a client whose seq is behind the server.
func (s *session) handleStaleSeq(c playCmd) {
	s.logger.Warn("stale seq",
		"seat", c.seat,
		"client_seq", c.msg.Seq,
		"server_seq", s.seq,
	)
	snap := s.playerSnapshot(c.seat)
	if snap == nil {
		s.terminateOnMarshalFailure(
			"stale seq player snapshot marshal failed",
			"seat", c.seat,
		)
		c.resp <- SubmitResult{
			Err: &api.ErrorMessage{
				Type:       errType,
				ErrorCode:  api.ErrInternal,
				Message:    "session terminated: snapshot generation failed",
				ActionID:   c.msg.ActionID,
				CurrentSeq: s.seq,
			},
		}
		return
	}
	c.resp <- SubmitResult{
		Err: &api.ErrorMessage{
			Type:       errType,
			ErrorCode:  api.ErrStaleSeq,
			Message:    "client seq is behind server",
			ActionID:   c.msg.ActionID,
			CurrentSeq: s.seq,
		},
		Snapshot: snap,
	}
}

// handleDuplicateAction returns a cached snapshot for a replayed action_id.
func (s *session) handleDuplicateAction(c playCmd, cached []byte) {
	s.logger.Warn("duplicate action_id", "action_id", c.msg.ActionID)
	result := SubmitResult{}
	if cached != nil {
		result.Snapshot = cached
	}
	c.resp <- result
}

// handleRejectedAction sends an error response for a game-adapter rejection.
func (s *session) handleRejectedAction(c playCmd, cmdErr *CommandError) {
	s.logger.Warn("action rejected",
		"seat", c.seat,
		"type", c.msg.Type,
		"error_code", cmdErr.Code,
		"message", cmdErr.Message,
	)
	s.sendError(c.seat, cmdErr.Code, cmdErr.Message, c.msg.ActionID)
	c.resp <- SubmitResult{
		Err: &api.ErrorMessage{
			Type:       errType,
			ErrorCode:  cmdErr.Code,
			Message:    cmdErr.Message,
			ActionID:   c.msg.ActionID,
			CurrentSeq: s.seq,
		},
	}
}

// handleAcceptedAction applies a successful action, broadcasts the new state,
// caches the snapshot, and drives subsequent AI turns.
func (s *session) handleAcceptedAction(c playCmd, res StepResult) {
	s.game.SetTurnDeadline(time.Time{})
	if res.Outcome != StepFinished {
		s.scheduleTurnDeadline()
	}
	s.seq++
	s.broadcastSnapshot()
	if s.finished {
		c.resp <- SubmitResult{
			Err: &api.ErrorMessage{
				Type:       errType,
				ErrorCode:  api.ErrInternal,
				Message:    "session terminated: snapshot generation failed",
				ActionID:   c.msg.ActionID,
				CurrentSeq: s.seq,
			},
		}
		return
	}
	// broadcastSnapshot already generated a player snapshot for this
	// seat and would have terminated the session if it failed to
	// marshal, so snap == nil is unreachable here.
	snap := s.playerSnapshot(c.seat)
	s.cacheActionID(c.msg.ActionID, snap)

	c.resp <- SubmitResult{}
	switch res.Outcome {
	case StepContinue:
		s.driveTurns(false)
	case StepPause:
		s.driveTurns(true)
	case StepFinished:
		s.finishWithGrace()
	}
}

// handleTurnTimeout handles a turn timeout by playing an AI move for
// the current human seat.
func (s *session) handleTurnTimeout() {
	seat := s.game.Turn()
	// Guard that the seat is still human (e.g., client disconnected human
	// player since timeout was set).
	if !s.isHumanSeat(seat) {
		s.logger.Debug("turn timeout skipped: seat no longer human", "seat", seat)
		return
	}

	s.logger.Info("turn timeout, playing AI move", "seat", seat)
	s.game.SetTurnDeadline(time.Time{})
	res, err := s.game.AIPlay(seat)
	if err != nil {
		s.logger.Error("AIPlay on timeout failed", "seat", seat, "error", err)
		return
	}
	s.game.SetTurnDeadline(time.Time{})
	if res.Outcome != StepFinished {
		s.scheduleTurnDeadline()
	}
	s.seq++
	s.broadcastSnapshot()
	if s.finished {
		return
	}
	switch res.Outcome {
	case StepContinue:
		s.driveTurns(false)
	case StepPause:
		s.driveTurns(true)
	case StepFinished:
		s.finishWithGrace()
	}
}

// handlePauseCmd pauses the game. Only valid for single-human sessions
// when waiting for a human turn and not already paused.
func (s *session) handlePauseCmd(c playCmd) {
	if s.humanCount() > 1 {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "pause is only available in single-human games",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if !s.waitingForHuman {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "can only pause during your turn",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if s.paused {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "game is already paused",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if s.turnTimeout() > 0 {
		s.pauseRemaining = time.Until(s.turnDeadline)
	}
	s.paused = true
	s.game.SetPaused(true)
	s.game.SetTurnDeadline(time.Time{})
	s.logger.Info("game paused", "seat", c.seat)
	s.seq++
	s.broadcastSnapshot()
	c.resp <- SubmitResult{}
}

// handleResumeCmd resumes the game. Only valid for single-human sessions
// when paused.
func (s *session) handleResumeCmd(c playCmd) {
	if !s.paused {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "game is not paused",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	if s.humanCount() > 1 {
		c.resp <- SubmitResult{Err: &api.ErrorMessage{
			Type: errType, ErrorCode: api.ErrPauseNotAllowed,
			Message:  "resume is only available in single-human games",
			ActionID: c.msg.ActionID, CurrentSeq: s.seq,
		}}
		return
	}
	s.paused = false
	s.game.SetPaused(false)
	if s.waitingForHuman && s.turnTimeout() > 0 {
		s.turnDeadline = time.Now().Add(s.pauseRemaining)
		s.game.SetTurnDeadline(s.turnDeadline)
	}
	s.logger.Info("game resumed", "seat", c.seat)
	s.seq++
	s.broadcastSnapshot()
	c.resp <- SubmitResult{}
}

// autoUnpause resumes a paused game when the human player disconnects.
func (s *session) autoUnpause(seat int) {
	s.logger.Info("auto-unpausing after human disconnect", "seat", seat)
	s.paused = false
	s.game.SetPaused(false)
	if s.waitingForHuman && s.turnTimeout() > 0 {
		s.turnDeadline = time.Now().Add(s.pauseRemaining)
		s.game.SetTurnDeadline(s.turnDeadline)
	}
	s.seq++
	s.broadcastSnapshot()
}
