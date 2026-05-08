# Repository Instructions

## UI Snapshot Coverage

aiops-v2 follows the Codex rule for user-visible UI changes: any change that affects visible UI must include screenshot snapshot coverage.

For frontend UI changes:

- Add or update a Playwright snapshot test in `web/tests/*snapshot*.spec.js` or another Playwright UI spec that calls `expect(...).toHaveScreenshot(...)`.
- Prefer fixture-driven tests through `web/tests/helpers/uiFixtureHarness.js` so the screenshot is deterministic and does not require a live LLM.
- Do not rely on `page.screenshot({ path })` as coverage. Those files are debug artifacts only; they do not fail when the UI regresses.
- Review snapshot diffs before accepting them.

Commands:

```bash
cd web
npm run test:ui:snapshots
npm run test:ui:snapshots:update
```

Use `test:ui:snapshots:update` only when the UI change is intentional and the new baseline has been reviewed.

## AssistantTransport Structured Streaming

Structured streaming in `aiops-v2` has one React Chat production path:

```text
TurnItem -> AiopsTransportState -> AssistantTransport data stream -> assistant-ui React
```

Agents working on chat, protocol, process UI, runtime items, approval, MCP surfaces, or replay must extend `AiopsTransportState` and AssistantTransport state ops directly. Do not add `StructuredResponsePatch`, `emit_response_events`, `StructuredResponsePanel`, page-local SSE/WebSocket streams, legacy `agent_event` reducers, `AgentEventProjection` selectors, `codexProcessTranscript`, `ChatProcessFold`, or assistant-final-text parsers for `summary/steps/actions`.

`AiopsTransportState.schemaVersion` is `aiops.transport.v2`. React Chat production transcript has one shape: `turn.blockOrder + turn.blocksById`. Do not reintroduce `turn.process`, `turn.final`, `metadata.unstable_state` transcript payloads, page-local chat SSE/WebSocket streams, or final Markdown/text parsing for process UI.

Playwright Chat snapshots are v2-only through `web/tests/react-shell-snapshot.spec.js`. Do not restore old `chat-ui-snapshot.spec.js`, `chat-native-process` fixtures, `chat-process-*`, `.chat-turn-final`, `processRows`, or `processGroups` test assets.

Before handing off structured streaming work, run:

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "AgentEventProjection|agent_event|codexProcessTranscript|ChatProcessFold" web/src
rg -n "JSON\\.parse\\(|markdown heading|summary.*steps.*actions" web/src
rg -n "chat-process-|aiops-process-transcript|processRows|processGroups|\\.chat-turn-final|process-step" web/tests
```

The first two commands should have no React Chat production hits. The JSON/Markdown command may find normal JSON parsing for settings, fixtures, transport envelopes, or API clients, but it must not find code that derives process UI from assistant final Markdown/text.
