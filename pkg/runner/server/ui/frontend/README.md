# Runner Visual Workflow UI (Legacy Debug Only)

This package is retained only as the standalone runner server legacy/debug UI. It is not the product entry for Runner workflow editing in the main AIOps application.

The main application product surface is `/runner`, implemented by `web/src/pages/RunnerStudioPage.vue`, and must call same-origin `/api/runner-studio/*` APIs. Do not add new product features here unless the same capability is first available in Runner Studio.

Migration candidates that can still be reused from this legacy app:

- workflow templates
- graph utils
- mock fixtures
- canvas interaction

Monaco is listed as a dependency for later YAML/JSON editors. Keep it lazy-loaded from feature components so the initial workbench bundle stays small.

## Commands

```sh
npm install
npm run dev
npm run test
npm run build
```

Production builds are written to `../dist`, which is the dist directory served by the runner server and embedded by the `runnerwebembed` build tag. That embedded UI is for runner-server debugging and compatibility only.

The Vite dev server proxies `/api` and `/ws` to `127.0.0.1:18080`.
