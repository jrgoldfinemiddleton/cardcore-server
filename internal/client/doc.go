// Package client provides a shared client engine for connecting to the
// cardcore game server over HTTP and WebSocket. It is protocol-agnostic
// and game-specific types live in subpackages (e.g., client/games/hearts).
//
// See ADR-011 (doc/decisions/011-client-snapshot-consumption.md) and
// ADR-013 (doc/decisions/013-error-recovery-responsibilities.md).
package client
