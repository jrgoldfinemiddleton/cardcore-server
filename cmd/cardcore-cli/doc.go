// Package main provides a non-TTY CLI for scripted cardcore game
// execution. It connects to a cardcore server via HTTP and WebSocket,
// plays according to a JSON script file, and exits with a status code.
//
// Usage
//
//	# Auto-create a 1-human + 3-AI Hearts game and play as the human
//	cardcore-cli -script script.json
//
//	# Create a 4-AI game and observe
//	cardcore-cli -observe
//
//	# Join an existing session as a specific seat
//	cardcore-cli -session-id ID -token TOKEN -seat 0 -script script.json
//
// The -script flag is required for player modes (auto-create and join).
// In observer mode the script is not used.
//
// Script format
//
//	[
//	  {
//	    "phase": "passing",
//	    "action": "pass_cards",
//	    "selector": "first_n",
//	    "selector_args": {"count": 3}
//	  },
//	  {
//	    "phase": "playing",
//	    "action": "play_card",
//	    "selector": "first_legal"
//	  }
//	]
//
// The script is phase-matched, not index-matched. Each entry describes
// what to do when it is the player's turn in the specified phase. A
// single "playing" entry with "first_legal" covers every human turn in
// the playing phase across all tricks and all rounds, regardless of
// server pacing or the number of AI turns between human turns.
//
// Exit codes
//
//	0  game completed normally (game_over phase reached)
//	1  runtime error: missing script entry for current phase, connection closed,
//	   server error, selector resolution failure, or command build failure
//	2  configuration error: invalid flags, script parse failure, or
//	   malformed selector_args
package main
