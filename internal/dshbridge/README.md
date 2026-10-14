# DSH AIOps bridge POC

This package implements the host half of the opt-in `aiops-dsh-bridge.v1` JSON-RPC protocol over one DSH process's stdio. It is a protocol POC; it does not launch DSH or select an engine in `RuntimeKernel` yet.

## Contract

- Build `Config.Tools` from the current Go-assembled and policy-filtered tool surface. `Start` sends only these schemas and rejects a missing or incompatible DSH extension.
- Call `Prompt` with host-created `SessionID`, `TurnID`, target-binding hash, tool-surface fingerprint, and permission-snapshot hash. The DSH `messageId` is only a queue receipt.
- The injected `ToolDispatcher` is required. Its production implementation must route into `RuntimeKernel` and the existing Go `ToolDispatcher`; it must preserve the trusted binding and wait for durable Go approval/resume before returning a successful tool result. This package has no direct executor or shell fallback.
- Persist `session.event` idempotently by `(sessionID, seq)` in `EventStore`. Cursor advancement must commit with the event, and `turn/end` completes only its bound Go turn after in-flight Go dispatches quiesce. Stream frames are transient and do not advance the event cursor.
- `RecoverTurn` requests only the durable tail after the Go cursor. It never replays a tool call. An ambiguous mutation must remain pending verification in the Go runtime.
- `CancelTurn` cancels the local Go dispatch first, asks DSH to cancel the exact active turn, and waits for in-flight Go dispatches to quiesce. The caller still needs to observe the durable terminal event with `WaitTurn`.

## Verification

Run the no-provider, no-secret protocol fixture with:

```sh
go test ./internal/dshbridge -count=1
```

The pinned upstream server extension and its tests are in `../../docs/superpowers/patches/deepseek-harness-d743-aiops-bridge.patch`, based on `d743267388641bc76f17c45ce8b4c231aed1d32c`. This is a zero-context patch generated from that exact base. Verify the checkout revision, then use `git apply --unidiff-zero --check` before applying it with `git apply --unidiff-zero`. It adds per-turn cancel, replay, stream, Go tool proxying, restricted tool visibility, and Session persistence resume.

## POC limits

The bridge still needs production `RuntimeKernel` turn admission/dispatch integration, `AssistantTransport` projection, a process launcher with stdout/stderr separation and process lifecycle handling, and a persistent `EventStore` adapter. The upstream patch has syntax validation only until DSH workspace dependencies are available; no real model key or production tool was used.
