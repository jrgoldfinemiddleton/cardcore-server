# Hearts Protocol

This document defines the game-specific message types, snapshot fields,
and phases for [Hearts](https://github.com/jrgoldfinemiddleton/cardcore/blob/main/doc/games/hearts/rules.md). It extends the generic protocol defined in
[`doc/api.md`](../../api.md).

---

## Inbound Message Types

### `play_card`

Play a card to the current trick.

**Payload:**

| Field | Type | Description |
|-------|------|-------------|
| `card` | object | The card to play. |
| `card.rank` | string | Rank name. |
| `card.suit` | string | Suit name. |

**Example:**

```json
{
  "type": "play_card",
  "action_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "seq": 47,
  "payload": {
    "card": { "rank": "queen", "suit": "spades" }
  }
}
```

### `pass_cards`

Pass cards during the passing phase.

**Payload:**

| Field | Type | Description |
|-------|------|-------------|
| `cards` | array of card objects | Exactly 3 cards to pass. |

**Example:**

```json
{
  "type": "pass_cards",
  "action_id": "550e8400-e29b-41d4-a716-446655440000",
  "seq": 3,
  "payload": {
    "cards": [
      { "rank": "queen", "suit": "spades" },
      { "rank": "king", "suit": "spades" },
      { "rank": "ace", "suit": "spades" }
    ]
  }
}
```

---

## Snapshot Fields

The snapshot message (defined in `doc/api.md`) contains the following
game-specific fields for Hearts:

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current game phase (see Phases below). |
| `round_number` | integer | Current round (1-indexed). |
| `trick_number` | integer | Current trick within the round (1-indexed). Only meaningful during `playing` and `trick_complete` phases. |
| `pass_direction` | string | `"left"`, `"right"`, `"across"`, or `"none"`. Indicates which direction cards are passed this round. |
| `turn` | integer | Seat index of the player who must act next. |
| `hearts_broken` | boolean | Whether hearts have been played (to any trick) this round. |
| `hand` | array of card objects | The receiving player's current hand, sorted. |
| `hand_counts` | array of integers | Number of cards in each seat's hand, indexed by seat. |
| `trick` | array of trick entries | Cards played to the current trick so far, in play order. |
| `scores` | array of integers | Cumulative scores per seat across all completed rounds. During an active round, this reflects the total as of the last completed round. |
| `round_points` | array of integers | Penalty points accumulated this round per seat. Resets to zero at the start of each round. |
| `legal_actions` | array of card objects | Cards the player may legally play or pass. Empty if it is not the player's turn. |
| `turn_deadline_ms` | integer | Server-side deadline for the current human turn, as Unix milliseconds since epoch. `0` when no deadline is active (e.g., AI turn, paused state, or timeout disabled). Clients should use this to render an accurate countdown instead of computing a deadline from the session's `turn_timeout_ms`. |

Each trick entry (ordered by play sequence, not by seat index):

| Field | Type | Description |
|-------|------|-------------|
| `seat` | integer | Which seat played this card. |
| `card` | object | The card played. |

---

## Phases

| Phase | Description |
|-------|-------------|
| `passing` | Players are selecting cards to pass. |
| `playing` | Trick-taking in progress. |
| `trick_complete` | A trick has been won. Server-synthesized pause for UX. |
| `round_complete` | A round has ended. Scores updated. |
| `game_over` | Game has ended. Final scores in `scores`. |

The server may introduce additional intermediate phases for UX pacing
in the future.

---

## Legal Actions

During the `passing` phase, `legal_actions` contains all cards in the
player's hand (any 3 may be selected).

During the `playing` phase, `legal_actions` contains the cards the
player may legally play, filtered by Hearts rules:
- Must follow the led suit if able.
- Cannot lead hearts until hearts are broken (unless hand is all hearts).
- Cannot play penalty cards (hearts or Q♠) on the first trick.
- The player holding 2♣ must lead it on the first trick of the first round.

`legal_actions` is empty when it is not the player's turn.

---

## Observer Additions

Observer snapshots use the same structure with the following
differences:

| Field | Difference |
|-------|-----------|
| `hand` | Replaced by `hands`: array of arrays, indexed by seat. All cards visible. |
| `trick_history` | Added: array of completed tricks this round. Each trick is an array of trick entries in play order. |
| `legal_actions` | Shows legal actions for the seat indicated by `turn`. |
| `turn_deadline_ms` | Same semantics as the player snapshot field: active human turn deadline, or `0` when none. |

---

## Error Codes

Hearts uses the generic error codes defined in `doc/api.md`. The
`illegal_move` error's `message` field contains the engine's
explanation of the specific rule violation (e.g., "Must follow suit:
diamonds was led").

---

## Example Snapshot

Player view, seat 0, round 1, trick 3:

```json
{
  "type": "snapshot",
  "seq": 12,
  "phase": "playing",
  "round_number": 1,
  "trick_number": 3,
  "pass_direction": "left",
  "turn": 0,
  "hearts_broken": false,
  "hand": [
    { "rank": "four", "suit": "clubs" },
    { "rank": "jack", "suit": "clubs" },
    { "rank": "seven", "suit": "diamonds" },
    { "rank": "queen", "suit": "diamonds" },
    { "rank": "two", "suit": "hearts" },
    { "rank": "nine", "suit": "hearts" },
    { "rank": "five", "suit": "spades" },
    { "rank": "ten", "suit": "spades" },
    { "rank": "king", "suit": "spades" },
    { "rank": "ace", "suit": "spades" },
    { "rank": "three", "suit": "spades" }
  ],
  "hand_counts": [11, 11, 10, 10],
  "trick": [
    { "seat": 2, "card": { "rank": "king", "suit": "diamonds" } },
    { "seat": 3, "card": { "rank": "five", "suit": "diamonds" } }
  ],
  "scores": [0, 0, 0, 0],
  "round_points": [0, 1, 0, 3],
  "legal_actions": [
    { "rank": "seven", "suit": "diamonds" },
    { "rank": "queen", "suit": "diamonds" }
  ],
  "turn_deadline_ms": 1728451200000
}
```

**Breakdown:** round 1 (no prior rounds, so `scores` is all zeros),
trick 3 in progress. After dealing (13 cards each) and passing (net
zero), tricks 1-2 are complete (2 cards played per seat = 11 remaining
each). Seat 2 led this trick (now 10 cards), seat 3 followed (now 10
cards). Seats 0 and 1 haven't played yet (still 11 cards each).
Diamonds was led, so `legal_actions` shows only the diamonds in seat
0's hand. `round_points` reflects 4 total penalty points taken across
tricks 1-2 (1 for seat 1, 3 for seat 3).
