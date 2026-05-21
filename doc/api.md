# Cardcore Server API

This document defines the HTTP and WebSocket API exposed by the
Cardcore server. It is the authoritative reference for client
implementors. The Go data transfer objects (DTOs) in `internal/api/`
are the canonical wire-format definitions; this document describes
their semantics, lifecycle, and usage.

Game-specific protocol details (message types, snapshot fields, phases)
are defined in per-game protocol files listed in the
[Supported Games](#supported-games) section.

## Conventions

- All HTTP endpoints accept and return `application/json`.
- All timestamps are ISO 8601 UTC.
- Field names use `snake_case`.
- Optional fields are omitted from the response when not applicable
  (not set to `null`).
- Unknown fields in request bodies are silently ignored.
- The server binds to `127.0.0.1` on a dynamic port for local play.

---

## Sessions

A **session** is the server's representation of one game from creation
to completion. It holds the engine state, seat assignments,
configuration, and communication channels.

### Lifecycle

```
     POST /sessions       POST /sessions/{id}/start
          │                       │
          ▼                       ▼
       ┌───────┐            ┌──────────┐          engine
       │ draft │───────────▶│  active   │───────▶ reports
       └───────┘            └──────────┘         completion
          │                       │                  │
          │ DELETE                │ DELETE           ▼
          │                       │             ┌──────────┐
          └─────────┐    ┌────────┘             │ finished │
                    ▼    ▼                      └──────────┘
                 ┌──────────┐                        │
                 │ expired  │◀───────────────────────┘
                 └──────────┘      DELETE or process exit
```

| State | Description |
|-------|-------------|
| `draft` | Session created, not yet started. Configuration may be changed. |
| `active` | Game is running. Commands are accepted. Snapshots are emitted. |
| `finished` | Game ended naturally. Read-only. Never auto-expires. |
| `expired` | Session cleaned up. ID is no longer valid. No recovery possible. |

`DELETE` in the diagram refers to a client sending the HTTP request
`DELETE /sessions/{id}`. It is the explicit action that cleans up a
session. Sessions do not auto-expire (except on process exit).

### State transitions

| From | To | Trigger |
|------|----|---------|
| `draft` | `active` | `POST /sessions/{id}/start` |
| `draft` | `expired` | `DELETE /sessions/{id}` or process exit |
| `active` | `finished` | Engine reports game completion |
| `active` | `expired` | `DELETE /sessions/{id}` or process exit |
| `finished` | `expired` | `DELETE /sessions/{id}` or process exit |

---

## HTTP Endpoints

### HTTP Endpoint → Manager Method Mapping

| Endpoint | Manager Method | Success Status | Error Statuses |
|----------|----------------|----------------|----------------|
| `POST /sessions` | `Create(Config)` | `201 Created` | `400 Bad Request` |
| `GET /sessions` | `List()` | `200 OK` | — |
| `GET /sessions/{id}` | `Get(id)` | `200 OK` | `404 Not Found` |
| `PATCH /sessions/{id}` | `Update(id, PatchConfig)` | `200 OK` | `404 Not Found`, `409 Conflict`, `400 Bad Request` |
| `POST /sessions/{id}/start` | `Start(id)` | `200 OK` | `404 Not Found`, `409 Conflict` |
| `DELETE /sessions/{id}` | `Delete(id)` | `204 No Content` | `404 Not Found` |

**Error status mapping:**

| Internal Error | HTTP Status |
|----------------|-------------|
| `ErrNotFound` | `404 Not Found` |
| `ErrNotDraft` / `ErrNotActive` | `409 Conflict` |
| `validateConfig` errors | `400 Bad Request` |
| Generic/unexpected errors | `500 Internal Server Error` |

### Create session

```
POST /sessions
```

Creates a new session in `draft` state.

**Request body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `game` | string | yes | Game identifier (e.g., `"hearts"`). |
| `seats` | array of seat config | yes | One entry per seat. |
| `pacing_delay_ms` | integer | no | Delay in milliseconds between state transitions requiring UX pacing (trick completion, round completion, AI turns). Default: `500`. Use `0` for tests. |

Each seat config:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"human"` or `"ai"`. |
| `ai_type` | string | if `type` is `"ai"` | AI implementation name (e.g., `"random"`, `"heuristic"`). |

A session may have zero human seats (all-AI demo mode). In this case,
no seat tokens are issued and the game is observed via the observer
WebSocket endpoint.

**Response:** `201 Created`

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Opaque session identifier. |
| `seats` | array | One entry per seat. |
| `seats[].index` | integer | Seat index (0-based). |
| `seats[].type` | string | `"human"` or `"ai"`. |
| `seats[].token` | string | Bearer token. Present only for human seats. |

**Errors:**

| Status | Condition |
|--------|-----------|
| `400 Bad Request` | Invalid game identifier, invalid seat config (including game-specific rules like seat count), missing `ai_type` for AI seat, unknown `ai_type`. |

---

### List sessions

```
GET /sessions
```

Returns summary information for all non-expired sessions.

**Response:** `200 OK`

| Field | Type | Description |
|-------|------|-------------|
| `sessions` | array | One entry per session. |
| `sessions[].session_id` | string | Session identifier. |
| `sessions[].game` | string | Game identifier. |
| `sessions[].state` | string | Current lifecycle state. |
| `sessions[].seat_count` | integer | Total number of seats. |
| `sessions[].human_count` | integer | Number of human seats. |

---

### Get session details

```
GET /sessions/{id}
```

Returns full session information. Does not include game state (use the
WebSocket snapshot for that).

**Response:** `200 OK`

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Session identifier. |
| `game` | string | Game identifier. |
| `state` | string | Current lifecycle state. |
| `seats` | array | One entry per seat. |
| `seats[].index` | integer | Seat index. |
| `seats[].type` | string | `"human"` or `"ai"`. |
| `seats[].ai_type` | string | AI implementation name. Present only for AI seats. |
| `pacing_delay_ms` | integer | Configured pacing delay in milliseconds. |

**Errors:**

| Status | Condition |
|--------|-----------|
| `404 Not Found` | Session does not exist or has expired. |

---

### Update session configuration

```
PATCH /sessions/{id}
```

Updates session configuration. Only valid in `draft` state.

**Request body:** Any subset of configurable fields.

| Field | Type | Description |
|-------|------|-------------|
| `seats` | array of seat config | Replace seat configuration. |
| `pacing_delay_ms` | integer | Update pacing delay in milliseconds. |

**Response:** `200 OK` — returns the full session details (same shape
as `GET /sessions/{id}`), plus an optional `seat_tokens` field when the
seat configuration was changed.

| Field | Type | Description |
|-------|------|-------------|
| *(same as `GET /sessions/{id}`)* | | All `SessionInfo` fields. |
| `seat_tokens` | array of `SeatInfo` | **Present only when `seats` was updated.** Contains fresh bearer tokens for the new seat configuration. |

**Errors:**

| Status | Condition |
|--------|-----------|
| `404 Not Found` | Session does not exist or has expired. |
| `409 Conflict` | Session is not in `draft` state. |
| `400 Bad Request` | Invalid configuration (including game-specific rules like seat count or unknown `ai_type`). |

---

### Start game

```
POST /sessions/{id}/start
```

Transitions the session from `draft` to `active`. Initializes the game
engine. No request body.

**Response:** `200 OK`

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Session identifier. |
| `state` | string | `"active"`. |

If a request body is provided, it is silently ignored.

**Errors:**

| Status | Condition |
|--------|-----------|
| `404 Not Found` | Session does not exist or has expired. |
| `409 Conflict` | Session is not in `draft` state. |

---

### Delete session

```
DELETE /sessions/{id}
```

Cancels or quits a session. Valid from any state. Transitions to
`expired`. Closes all WebSocket connections to this session.

**Response:** `204 No Content`

**Errors:**

| Status | Condition |
|--------|-----------|
| `404 Not Found` | Session does not exist or has already expired. |

Note: this endpoint is not idempotent. Calling `DELETE` on an
already-expired session returns `404`. This is a deliberate trade-off
for informativeness over idempotency.

---

## WebSocket: Player Connection

### Upgrade

```
GET /sessions/{id}/ws
```

Upgrades the HTTP connection to a WebSocket. Requires a valid seat
token.

**Headers:**

| Header | Value |
|--------|-------|
| `Authorization` | `Bearer <seat_token>` |

The token is sent as a plaintext hex string in the header value. This
is the standard HTTP bearer token scheme (RFC 6750). Over localhost,
TLS is not used — acceptable because `127.0.0.1` traffic is not
visible to other machines.

**Behavior:**

1. Server validates the token. If invalid or maps to an AI seat,
   responds with `401 Unauthorized` (no WebSocket established).
2. If another WebSocket is already connected for this seat, the
   existing connection is closed (kicked).
3. On successful upgrade, the server immediately sends a `snapshot`
   message with the current game state filtered for this seat.

**Errors (HTTP, before upgrade):**

| Status | Condition |
|--------|-----------|
| `401 Unauthorized` | Missing, invalid, or AI-seat token. |
| `404 Not Found` | Session does not exist or has expired. |

---

## WebSocket: Observer Connection

### Upgrade

```
GET /sessions/{id}/ws/observe
```

Upgrades the HTTP connection to a WebSocket for omniscient observation.
Authentication is not required for observer connections when the server
is bound to localhost. Networked deployments require authentication on
this endpoint.

**Behavior:**

1. On successful upgrade, the server immediately sends a `snapshot`
   message with the full game state (all hands visible).
2. Observer connections are receive-only. The server ignores any
   messages sent by the observer.

A player connection and an observer connection may exist simultaneously
for the same session. They are independent.

**Errors (HTTP, before upgrade):**

| Status | Condition |
|--------|-----------|
| `404 Not Found` | Session does not exist or has expired. |

---

## WebSocket: Inbound Messages (Client → Server)

All inbound messages use a common envelope:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Message type identifier. Game-specific types are defined in each game's protocol file. |
| `action_id` | string | yes | Client-generated unique ID for idempotency. Opaque to the server — any non-empty string up to 256 characters. |
| `seq` | integer | yes | Last `seq` value the client received in a snapshot. |
| `payload` | object | yes | Type-specific payload. Defined by the game's protocol file. |

The server does not validate `action_id` format. It is treated as an
opaque lookup key. Constraints: non-empty, at most 256 characters. A
message with an empty or over-length `action_id` is rejected as
`malformed_message`. Using UUIDs is recommended but not required.

---

## WebSocket: Outbound Messages (Server → Client)

### `snapshot`

Full game state filtered for the receiving client's seat. Sent after
every state change and immediately on WebSocket connection.

Every snapshot contains at minimum:

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `"snapshot"`. |
| `seq` | integer | Monotonic counter. Increments on every state change. |

All remaining snapshot fields are game-specific and defined in the
game's protocol file. See [Supported Games](#supported-games).

### `error`

Sent when a client command is rejected. The WebSocket connection
remains open.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"error"`. |
| `error_code` | string | yes | Machine-readable error code. |
| `message` | string | yes | Human-readable explanation. Suitable for display to the user. |
| `action_id` | string | no | The `action_id` from the rejected command. Omitted only when JSON parsing failed and the field could not be extracted. |
| `current_seq` | integer | yes | The server's current seq value. |

**Example:**

```json
{
  "type": "error",
  "error_code": "illegal_move",
  "message": "Must follow suit: diamonds was led",
  "action_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "current_seq": 12
}
```

---

## Command Rejection

When the server rejects a client command, it sends an `error` message
(except where noted). The WebSocket connection remains open.

| Condition | Error code | Notes |
|-----------|-----------|-------|
| Duplicate `action_id` | *(none)* | Idempotent success: server returns the cached snapshot from the original action. Payload differences on retry are ignored. Client processes the response normally and stops retrying. |
| `seq` behind server | `stale_seq` | Client should resync from the snapshot that immediately follows this error. The server always sends a fresh snapshot after a `stale_seq` error. |
| Not this seat's turn | `out_of_turn` | Returned when a command is sent and it is not the sender's turn — whether it is another human's turn or an AI seat's turn. |
| Illegal move | `illegal_move` | The `message` field contains the game engine's explanation of the rule violation. |
| Wrong phase | `wrong_phase` | e.g., a command sent during the wrong game phase. |
| Game is finished | `game_over` | Session is in `finished` state. |
| Malformed message | `malformed_message` | Bad JSON, missing required fields, unknown type, empty or over-length `action_id`. Includes `action_id` in response when the field was parseable; omits it only when JSON parsing itself failed. |

Authentication failures (`401`) occur at HTTP upgrade time, not as
WebSocket error messages.

### WebSocket close frames

The server closes the WebSocket with a protocol-level close frame only
in unrecoverable situations:

| Close code | Condition |
|-----------|-----------|
| `1009 Message Too Big` | Inbound message exceeds 64 KB size limit. |
| `1008 Policy Violation` | Session deleted while connected. |
| `1011 Internal Error` | Snapshot generation failed after a successful action. Session is unplayable and terminates. *(Planned — not yet implemented.)* |

These are terminal — the connection is gone. The client must
reconnect or exit. No application-level error event precedes the close.

---

## Seat Tokens

- Tokens are issued only for human seats at session creation.
- Generated using `crypto/rand`: 32 bytes of random data, hex-encoded
  to a 64-character string.
- Delivered in the `POST /sessions` response.
- Used in the `Authorization: Bearer <token>` header on WebSocket
  upgrade.
- Each token maps to exactly one session and one seat. The mapping is
  immutable.
- AI seats have no tokens. The server rejects WebSocket upgrades for
  AI seats.

---

## AI Turn Flow

When it is an AI seat's turn:

1. The session goroutine calls the AI's play method internally. No
   WebSocket command is involved.
2. The server applies the AI's move to the engine.
3. The server sends a `snapshot` to all connected clients.
4. If the next turn is also an AI seat, the server waits at least
   `pacing_delay_ms` milliseconds (configured per session) before
   repeating from step 1. The delay is a floor: if the AI's compute
   time exceeds `pacing_delay_ms`, no additional delay is added.
5. This continues until it is a human seat's turn or the game ends.

The client infers "AI is thinking" by checking the `turn` field in the
last snapshot: if `turn` indicates an AI seat, the next snapshot will
arrive when the AI finishes.

---

## Security (localhost deployment)

- Server binds to `127.0.0.1` only. Never `0.0.0.0`.
- HTTP request body limit: 1 MB.
- WebSocket message size limit: 64 KB. Exceeding this closes the
  connection with close code `1009 Message Too Big`.
- Session IDs and seat tokens generated with `crypto/rand`.
- `encoding/json` only — no eval, no deserialization gadgets.
- No SQL, no shell execution, no file I/O from client input.
- Observer endpoint is unauthenticated (localhost only). Networked
  deployments require authentication on this endpoint.
- No TLS for localhost. Networked deployments require TLS.

---

## Card Wire Format

Cards are represented as JSON objects with `rank` and `suit` string
fields (rank first — matching natural card-player language: "queen of
spades").

**Ranks:** `"two"`, `"three"`, `"four"`, `"five"`, `"six"`, `"seven"`,
`"eight"`, `"nine"`, `"ten"`, `"jack"`, `"queen"`, `"king"`, `"ace"`.

**Suits:** `"clubs"`, `"diamonds"`, `"hearts"`, `"spades"`.

**Example:**

```json
{ "rank": "queen", "suit": "spades" }
```

---

## Logging

- Server and TUI use `log/slog` (Go stdlib).
- Per-component prefix: `server`, `tui`.
- Log level configurable via flag or environment variable. Default:
  `info`.
- Server logs to stderr by default. Optional `--log-file` flag for
  persistent file output.
- TUI logs to a file (not stdout, which is the terminal UI). Bubble
  Tea provides `tea.LogToFile()` for this purpose.

---

## WebSocket Protocol Details

- The server uses `coder/websocket` (formerly `nhooyr.io/websocket`).
- Ping/pong is handled at the WebSocket protocol level by the library.
  No application-level keepalive is needed. A slow player will not
  time out.
- WebSocket close frames are reserved for unrecoverable situations
  only (see Command Rejection section). All recoverable errors are
  sent as `error` messages over the open connection.
- Second connection per seat: the existing connection is closed
  (kicked). The new connection receives an initial snapshot.

---

## Supported Games

| Game | Protocol File |
|------|--------------|
| Hearts | [`doc/games/hearts/protocol.md`](games/hearts/protocol.md) |
