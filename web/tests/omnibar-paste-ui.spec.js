// @ts-check
import { test, expect } from "@playwright/test";
import {
  createChatFixtureSessions,
  createChatFixtureState,
  createProtocolFixtureSessions,
  createProtocolFixtureState,
  openFixturePage,
} from "./helpers/uiFixtureHarness";

async function pasteText(locator, text) {
  await locator.evaluate((element, payload) => {
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", {
      configurable: true,
      value: {
        getData: () => payload,
        files: [],
      },
    });
    element.dispatchEvent(pasteEvent);
  }, text);
}

async function pasteTransferOnly(locator, text) {
  await locator.evaluate((element, payload) => {
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", {
      configurable: true,
      value: {
        getData: () => payload,
        files: [],
      },
    });
    element.dispatchEvent(pasteEvent);
  }, text);
}

async function dropImage(locator, filename) {
  await locator.evaluate((element, name) => {
    const file = new File(["img"], name, { type: "image/png" });
    const dropEvent = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(dropEvent, "dataTransfer", {
      configurable: true,
      value: {
        files: [file],
        getData: () => "",
      },
    });
    element.dispatchEvent(dropEvent);
  }, filename);
}

const LARGE_PASTE = [
  "journalctl -u nginx --since '-10m'",
  "upstream timeout for service-a",
  "upstream timeout for service-b",
  "upstream timeout for service-c",
  "connection reset by peer",
  "error summary: 15 entries",
].join("\n");

const PATH_LIST = [
  "/Users/demo/logs/nginx.log",
  "/Users/demo/conf/nginx.conf",
].join("\n");

test.describe("Omnibar paste assist", () => {
  test("chat page shows a paste buffer hint before send becomes available", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: createChatFixtureState({
        runtime: {
          turn: { active: false, phase: "completed", hostId: "web-01" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createChatFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");
    const primaryAction = page.getByTestId("omnibar-primary-action");

    await pasteText(input, LARGE_PASTE);

    await expect(input).toHaveValue(LARGE_PASTE);
    await expect(page.getByTestId("omnibar-paste-indicator")).toContainText("正在整理");
    await expect(primaryAction).toBeDisabled();

    await page.waitForTimeout(500);

    await expect(page.getByTestId("omnibar-paste-indicator")).toContainText("可继续检查后发送");
    await expect(primaryAction).toBeEnabled();
  });

  test("protocol page shows the same paste guard in the docked omnibar", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
        approvals: [],
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");
    const primaryAction = page.getByTestId("omnibar-primary-action");

    await pasteText(input, LARGE_PASTE);

    await expect(input).toHaveValue(LARGE_PASTE);
    await expect(page.getByTestId("omnibar-paste-indicator")).toContainText("正在整理");
    await expect(primaryAction).toBeDisabled();

    await page.waitForTimeout(500);

    await expect(page.getByTestId("omnibar-paste-indicator")).toContainText("可继续检查后发送");
    await expect(primaryAction).toBeEnabled();
  });

  test("chat page recognizes pasted paths without dumping the raw list into the textarea", async ({ page }) => {
    await openFixturePage(page, "/", {
      state: createChatFixtureState({
        runtime: {
          turn: { active: false, phase: "completed", hostId: "web-01" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
      }),
      sessions: createChatFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");

    await pasteTransferOnly(input, PATH_LIST);

    await expect(page.getByTestId("omnibar-attachment-indicator")).toContainText("2 个路径");
    await expect(page.getByTestId("omnibar-artifact-pill")).toContainText("路径 2");
    await expect(input).toHaveValue("");

    await input.blur();
    await input.focus();

    await expect(page.getByTestId("omnibar-focus-hint")).toContainText("已恢复输入焦点");
  });

  test("protocol page recognizes dropped images and keeps a subtle recovery hint on refocus", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
        approvals: [],
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");

    await dropImage(input, "incident.png");

    await expect(page.getByTestId("omnibar-attachment-indicator")).toContainText("1 张图片");
    await expect(page.getByTestId("omnibar-artifact-pill")).toContainText("图片 1");

    await input.blur();
    await input.focus();

    await expect(page.getByTestId("omnibar-focus-hint")).toContainText("已恢复输入焦点");
  });

  test("protocol page pastes short plain text directly into the docked omnibar", async ({ page }) => {
    await openFixturePage(page, "/protocol", {
      state: createProtocolFixtureState({
        runtime: {
          turn: { active: false, phase: "completed", hostId: "server-local" },
          codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
          activity: {},
        },
        approvals: [],
      }),
      sessions: createProtocolFixtureSessions(),
    });

    const input = page.getByTestId("omnibar-input");

    await pasteText(input, "请检查 host-2 的磁盘状态");

    await expect(input).toHaveValue("请检查 host-2 的磁盘状态");
    await expect(page.getByTestId("omnibar-hint")).toContainText("发送");
  });
});
