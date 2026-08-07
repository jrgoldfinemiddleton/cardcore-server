# AI Agent Guidance: API Wire Format

## OVERVIEW
Game-agnostic wire-format types and error codes. Server-side packages import this package directly for the JSON-over-WebSocket protocol envelope. Client packages (`internal/client/`) intentionally mirror the wire types they need rather than importing `internal/api/` so the client engine stays decoupled from server internals.

## STRUCTURE
```
api/
├── api.go           # InboundMessage, ErrorMessage, error codes, validation
├── api_test.go      # Round-trip and validation tests
└── games/hearts/    # Hearts-specific wire DTOs and conversion
    ├── hearts.go    # Card, snapshots, payloads
    ├── convert.go   # Engine-to-wire conversion
    └── hearts_test.go
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Change message envelope | `api.go` | All client/server messages use `InboundMessage`/`ErrorMessage` |
| Add error code | `api.go` | Add to the const block; mirror in `internal/client/errors.go` |
| Change Hearts wire DTO | `games/hearts/hearts.go` | Mirrors engine types in JSON-friendly form |
| Change engine↔wire conversion | `games/hearts/convert.go` | Ranks/suits/phases use lowercase full words |

## CONVENTIONS
- Wire format uses lowercase full words for ranks/suits/phases (`"ace"`, `"spades"`, `"passing"`).
- Conversion functions are bidirectional: `ToWire` and `FromWire`.
- `ValidateInboundMessage` checks required fields and returns descriptive errors.

## ANTI-PATTERNS
- Never put business logic in this package; it is only for envelopes and DTOs.
- Never import server or client internals from `api`.
- Never change error code strings without updating `internal/client/errors.go`.
