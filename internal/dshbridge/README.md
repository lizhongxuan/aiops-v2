# DSH AIOps bridge POC

This package implements the host half of the opt-in `aiops-dsh-bridge.v1` JSON-RPC protocol over one DSH process's stdio. It is a protocol POC; it does not launch DSH or select an engine in `RuntimeKernel` yet.

## Contract

- Build `Config.Tools` from the current Go-assembled and policy-filtered tool surface. `Start` sends only these schemas and rejects a missing or incompatible DSH extension.
- Call `Prompt` with host-created `SessionID`, `TurnID`, target-binding hash, tool-surface fingerprint, and permission-snapshot hash. The DSH `messageId` is only a queue receipt.
- The injected `ToolDispatcher` is required. Its production implementation must route into `RuntimeKernel` and the existing Go `ToolDispatcher`; it must preserve the trusted binding and wait for Go approval/resume before returning a settled tool result. When Go durably creates an approval, `Dispatch` returns `ErrApprovalPending`; the bridge leaves the original DSH JSON-RPC request open and `WaitTurn` returns `ErrTurnSuspendedForApproval` so the host can return its existing blocked result.
- After `ResumeTurn` validates the approval and Go finishes the matching action, the host calls `ResolveToolCall` with the exact host binding and original DSH `callId`. That is the only path that supplies the tool result to the pending Harness request. A dispatcher that supports pending approvals must also implement `PendingApprovalCanceler`, so turn cancellation revokes Go's approval before asking DSH to cancel.
- Persist `session.event` idempotently by `(sessionID, seq)` in `EventStore`. Cursor advancement must commit with the event, and `turn/end` completes only its bound Go turn after in-flight Go dispatches quiesce. Stream frames are transient and do not advance the event cursor.
- `RecoverTurn` requests only the durable tail after the Go cursor. It never replays a tool call. An ambiguous mutation must remain pending verification in the Go runtime.
- `CancelTurn` cancels local Go dispatches, waits for them to quiesce, revokes pending Go approvals through the dispatcher, then asks DSH to cancel the exact active turn. The caller still needs to observe the durable terminal event with `WaitTurn`.
- `WaitTurn` waits on the durable terminal event and pending-approval transitions. It does not depend on a `session/wait` idle response; idle, prompt receipt, and stream frames are not business completion.

## Verification

Run the no-provider, no-secret protocol fixture with:

```sh
go test ./internal/dshbridge -count=1
```

The pinned upstream server extension and its tests are in `../../docs/superpowers/patches/deepseek-harness-d743-aiops-bridge.patch`, based on `d743267388641bc76f17c45ce8b4c231aed1d32c`. This is a zero-context patch generated from that exact base. Verify the checkout revision, then use `git apply --unidiff-zero --check` before applying it with `git apply --unidiff-zero`. It adds per-turn cancel, replay, stream, Go tool proxying, restricted tool visibility, and Session persistence resume.

## POC limits

The bridge still needs production `RuntimeKernel` turn admission/dispatch integration, `AssistantTransport` projection, a process launcher with stdout/stderr separation and process lifecycle handling, and a persistent `EventStore` adapter. Pending DSH RPC handles are in memory; recovery of an open approval across a DSH process restart is not implemented, so a crash during approval must remain blocked for operator reconciliation. The upstream patch has syntax validation only until DSH workspace dependencies are available; no real model key or production tool was used.
