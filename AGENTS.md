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

## Codex-Style Structured Streaming

Structured streaming in `aiops-v2` has one production path:

```text
TurnItem -> AgentEvent -> AgentEventProjection -> codexProcessTranscript -> ChatProcessFold
```

Agents working on chat, protocol, process UI, runtime items, or replay must extend that path directly. Do not add `StructuredResponsePatch`, `emit_response_events`, `StructuredResponsePanel`, page-local SSE/WebSocket streams, or assistant-final-text parsers for `summary/steps/actions`.

Before handing off structured streaming work, run:

```bash
rg -n "emit_response_events|StructuredResponsePatch|StructuredResponsePanel" internal web/src
rg -n "JSON\\.parse\\(|markdown heading|summary.*steps.*actions" web/src
```

The first command should have no production hits. The second command may find normal JSON parsing for settings, fixtures, or realtime envelopes, but it must not find code that derives process UI from assistant final Markdown/text.
