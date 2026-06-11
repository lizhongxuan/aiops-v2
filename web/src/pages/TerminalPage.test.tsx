import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
          <Routes>
            <Route path="/terminal/:hostId" element={<TerminalPage />} />
          </Routes>
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
});
