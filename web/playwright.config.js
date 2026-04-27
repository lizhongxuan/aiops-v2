import { defineConfig } from "@playwright/test";

const workerCount = Number.parseInt(process.env.PLAYWRIGHT_WORKERS || "1", 10);

export default defineConfig({
  testDir: "./tests",
  testMatch: ["e2e/**/*.spec.js", "protocol-chat-ui.spec.js", "chat-ui-visual.spec.js", "chat-ui-snapshot.spec.js"],
  timeout: 30000,
  workers: Number.isFinite(workerCount) && workerCount > 0 ? workerCount : 1,
  snapshotPathTemplate: "{testDir}/__screenshots__/{testFilePath}/{arg}{ext}",
  expect: {
    toHaveScreenshot: {
      maxDiffPixelRatio: 0.01,
      threshold: 0.2,
    },
  },
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://127.0.0.1:8080",
    viewport: { width: 1440, height: 900 },
    ignoreHTTPSErrors: true,
  },
  projects: [
    {
      name: "chromium",
      use: { browserName: "chromium" },
    },
  ],
});
