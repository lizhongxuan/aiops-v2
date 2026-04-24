import { defineConfig } from "vitest/config";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  plugins: [vue()],
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./tests/setup.js",
    exclude: [
      "tests/e2e/**",
      "node_modules/**",
      "tests/chat-choice-ui.spec.js",
      "tests/chat-fixture-ui.spec.js",
      "tests/chat-ui-visual.spec.js",
      "tests/layout-responsive.spec.js",
      "tests/omnibar-paste-ui.spec.js",
      "tests/protocol-chat-ui.spec.js",
      "tests/protocol-choice-ui.spec.js",
      "tests/protocol-fixture-ui.spec.js",
      "tests/protocol-host-label.spec.js",
      "tests/protocol-stale-approval.spec.js",
      "tests/protocol-ux-fixes.spec.js",
      "tests/protocol-workspace.spec.js",
      "tests/sidebar-and-layout.spec.js",
    ],
  },
});
