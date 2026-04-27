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
