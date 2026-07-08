import { defineConfig } from "vitest/config";
import path from "node:path";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.js",
    exclude: [
      "tests/e2e/**",
      "node_modules/**",
      "tests/ChatPage.spec.js",
      "tests/ChatPage.chatstream.spec.js",
      "tests/assistant-message-single-path.spec.js",
      "tests/chat-choice-ui.spec.js",
      "tests/chat-runtime-folding-snapshot.spec.js",
      "tests/context-compaction-snapshot.spec.js",
      "tests/chat-fixture-ui.spec.js",
      "tests/chat-ui-snapshot.spec.js",
      "tests/chat-ui-visual.spec.js",
      "tests/layout-responsive.spec.js",
      "tests/llm-config-context-window.spec.js",
      "tests/llm-provider-config-snapshot.spec.js",
      "tests/omnibar-paste-ui.spec.js",
      "tests/ops-manual-param-resolution.spec.js",
      "tests/protocol-chat-ui.spec.js",
      "tests/protocol-choice-ui.spec.js",
      "tests/protocol-fixture-ui.spec.js",
      "tests/protocol-host-label.spec.js",
      "tests/protocol-stale-approval.spec.js",
      "tests/protocol-ux-fixes.spec.js",
      "tests/protocol-workspace.spec.js",
      "tests/react-route-smoke.spec.js",
      "tests/react-shell-snapshot.spec.js",
      "tests/runner-studio.spec.js",
      "tests/sidebar-and-layout.spec.js",
      "tests/tool-mcp-slimming.spec.js",
    ],
  },
});
