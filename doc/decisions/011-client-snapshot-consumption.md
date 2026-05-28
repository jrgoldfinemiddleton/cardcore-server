# ADR-011: Client Snapshot Consumption Contract

**Date:** 2026-05-27
**Status:** Accepted

## Context

The server multiplexes snapshots to each player connection from two independent Go channels: the broadcast path (`subCh`, emitted after every state change) and the synchronous response path (`outCh`, returned after a client command). Go channel scheduling may deliver these out of monotonic `seq` order. A client that replaces its state with every snapshot risks displaying stale data.

## Decision

Clients track the highest `seq` received (`maxSeenSeq`) and ignore any snapshot with `seq <= maxSeenSeq`. Each accepted snapshot replaces the client's game state wholesale — no merging, no patching. Duplicate `action_id` responses return the cached snapshot with its original `seq`; the client processes it as any snapshot (ignore if `seq <= maxSeenSeq`, otherwise replace state).

## Consequences

(+) Ordering bugs from channel multiplexing are structurally impossible. (+) Client state logic is trivial: parse, compare seq, replace or discard. (+) Reconnecting is straightforward: the server sends the current snapshot on WebSocket upgrade; the client sets `maxSeenSeq` from that snapshot's `seq`. No special reconnect logic is needed because TCP buffer remnants from the previous connection are filtered by the same rule. (+) A client that receives an out-of-order broadcast after reconnect silently discards it instead of reverting state. (-) Client must maintain `maxSeenSeq` per session — added state and a new bug surface (client forgets to track it, displays stale data silently). (-) Out-of-order snapshots from multiplexing are sent by the server but discarded by the client — wasted marshal CPU and bandwidth for frames that are immediately ignored. (-) A client that misses broadcasts (channel full, slow consumer) has no passive detection. It only discovers it's behind when it sends a command and gets `stale_seq`. (-) The client cannot distinguish "server hasn't sent a snapshot because nothing changed" from "server sent a snapshot that I dropped." Both look like silence.
