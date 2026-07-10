import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AppShellChromeProvider, useAppShellChrome } from "@/app/AppShellChromeContext";

import { TerminalPage } from "./TerminalPage";

const terminalWrite = vi.fn();

vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    cols = 120;
    rows = 36;
    loadAddon() {}
    open() {}
    write(value: string) {
      terminalWrite(value);
    }
    onData() {
      return { dispose() {} };
    }
    clear() {}
    dispose() {}
  },
}));

vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class {
    fit() {}
  },
}));

class MockResizeObserver {
  observe() {}
  disconnect() {}
}

class MockWebSocket {
  static OPEN = 1;
  readyState = MockWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: string[] = [];

  constructor(public url: string) {
    setTimeout(() => this.onopen?.(), 0);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.onclose?.();
  }
}

function ChromeActionsProbe() {
  const { headerActions, headerDescription, headerTitle } = useAppShellChrome();
  return (
    <div data-testid="chrome-actions">
      <span>{headerTitle}</span>
      <span>{headerDescription}</span>
      {headerActions}
    </div>
  );
}

describe("TerminalPage", () => {
  let root: Root;
  let container: HTMLDivElement;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    terminalWrite.mockClear();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ sessionId: "term-grpc-1", hostId: "host-prod-07", shell: "/bin/bash", cwd: "/srv" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("ResizeObserver", MockResizeObserver);
    vi.stubGlobal("WebSocket", MockWebSocket);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.unstubAllGlobals();
  });

  it("describes a gRPC host client terminal without Agent, Mission, or Plan semantics", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/terminal/host-prod-07"]}>
          <AppShellChromeProvider>
            <Routes>
              <Route path="/terminal/:hostId" element={<TerminalPage />} />
            </Routes>
            <ChromeActionsProbe />
          </AppShellChromeProvider>
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const text = container.textContent || "";
    expect(text).toContain("gRPC 主机客户端");
    expect(text).toContain("terminal session");
    expect(text).toContain("websocket");
    expect(text).not.toMatch(/Agent|Mission|Plan|任务计划|子任务/);

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/api/v1/terminal/sessions"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ hostId: "host-prod-07", cols: 120, rows: 36 }),
      }),
    );
    expect(terminalWrite).toHaveBeenCalledWith(expect.stringContaining("gRPC host client terminal initializing"));
  });

  it("uses the app top bar for terminal controls and lets the terminal fill the page", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/terminal/host-prod-07"]}>
          <AppShellChromeProvider>
            <Routes>
              <Route path="/terminal/:hostId" element={<TerminalPage />} />
            </Routes>
            <ChromeActionsProbe />
          </AppShellChromeProvider>
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const chromeActionsText = container.querySelector('[data-testid="chrome-actions"]')?.textContent || "";
    expect(chromeActionsText).toContain("退出");
    expect(chromeActionsText).toMatch(/connected|ready/);
    expect(chromeActionsText).toContain("清屏");
    expect(chromeActionsText).toContain("Ctrl-C");
    expect(chromeActionsText).toContain("Fit");
    expect(container.querySelector('[data-testid="terminal-card-header"]')).toBeNull();

    expect(container.querySelector('[data-testid="terminal-page"]')?.className).toContain("h-full");
    const terminalShell = container.querySelector('[data-testid="terminal-xterm"]');
    expect(terminalShell?.className).toContain("flex-1");
    expect(terminalShell?.className).toContain("min-h-0");
    expect(terminalShell?.className).not.toContain("h-[620px]");
  });

  it("reserves bottom inset inside xterm so the last terminal row is not clipped", async () => {
    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/terminal/host-prod-07"]}>
          <AppShellChromeProvider>
            <Routes>
              <Route path="/terminal/:hostId" element={<TerminalPage />} />
            </Routes>
            <ChromeActionsProbe />
          </AppShellChromeProvider>
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const terminalShell = container.querySelector('[data-testid="terminal-xterm"]');
    expect(terminalShell?.className).toContain("[&_.xterm]:box-border");
    expect(terminalShell?.className).toContain("[&_.xterm]:pb-3");
  });

  it("shows backend terminal session errors instead of a generic 400", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: "ssh_credential_secret_missing",
          error: "SSH 凭证文件缺失，无法创建远程终端",
          message: "SSH 凭证文件缺失，无法创建远程终端",
          detail: "read ssh credential secret://hosts/remote/ssh-password: no such file or directory",
          diagnostics: ["检查当前 AIOPS_DATA_DIR 是否指向原来的数据目录。"],
          nextSteps: ["进入主机列表，编辑该主机，重新输入 SSH 密码并点击 SSH 测试。"],
        }),
        {
          status: 400,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await act(async () => {
      root.render(
        <MemoryRouter initialEntries={["/terminal/host-prod-07"]}>
          <AppShellChromeProvider>
            <Routes>
              <Route path="/terminal/:hostId" element={<TerminalPage />} />
            </Routes>
            <ChromeActionsProbe />
          </AppShellChromeProvider>
        </MemoryRouter>,
      );
    });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    const text = container.textContent || "";
    expect(text).toContain("SSH 凭证文件缺失");
    expect(text).toContain("诊断建议");
    expect(text).toContain("AIOPS_DATA_DIR");
    expect(text).toContain("下一步");
    expect(text).toContain("重新输入 SSH 密码");
    expect(text).toContain("read ssh credential");
    expect(text).not.toContain("Request failed: 400");
  });
});
