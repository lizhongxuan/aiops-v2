import { beforeEach, describe, expect, it, vi } from "vitest";
import { createTerminalSocketClient } from "../src/realtime/terminalSocket";

class FakeWebSocket {
  static instances = [];
  static OPEN = 1;

  constructor(url) {
    this.url = url;
    this.readyState = 0;
    this.sent = [];
    FakeWebSocket.instances.push(this);
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  message(data) {
    this.onmessage?.({ data });
  }

  close() {
    this.readyState = 3;
    this.onclose?.();
  }

  error(event = new Event("error")) {
    this.onerror?.(event);
  }

  send(payload) {
    this.sent.push(payload);
  }
}

describe("terminalSocket", () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
  });

  it("connects to the terminal websocket and routes typed messages", () => {
    const events = [];
    const client = createTerminalSocketClient({
      WebSocketCtor: FakeWebSocket,
      location: { protocol: "https:", host: "example.test" },
      onOpen: () => events.push("open"),
      onReady: (payload) => events.push(["ready", payload]),
      onOutput: (payload) => events.push(["output", payload]),
      onStatus: (payload) => events.push(["status", payload]),
      onExit: (payload) => events.push(["exit", payload]),
      onErrorMessage: (payload) => events.push(["error", payload]),
      onRawMessage: (payload) => events.push(["raw", payload]),
      onClose: () => events.push("close"),
      onSocketError: () => events.push("socket-error"),
    });

    client.connect("term-1");
    expect(FakeWebSocket.instances[0].url).toBe("wss://example.test/api/v1/terminal/ws?sessionId=term-1");

    FakeWebSocket.instances[0].open();
    FakeWebSocket.instances[0].message(JSON.stringify({ type: "ready", cwd: "~" }));
    FakeWebSocket.instances[0].message(JSON.stringify({ type: "output", data: "hello" }));
    FakeWebSocket.instances[0].message(JSON.stringify({ type: "status", status: "connected" }));
    FakeWebSocket.instances[0].message(JSON.stringify({ type: "exit", code: 0 }));
    FakeWebSocket.instances[0].message(JSON.stringify({ type: "error", message: "boom" }));
    FakeWebSocket.instances[0].message("plain text");
    FakeWebSocket.instances[0].error();
    FakeWebSocket.instances[0].close();

    expect(events).toEqual([
      "open",
      ["ready", { type: "ready", cwd: "~" }],
      ["output", { type: "output", data: "hello" }],
      ["status", { type: "status", status: "connected" }],
      ["exit", { type: "exit", code: 0 }],
      ["error", { type: "error", message: "boom" }],
      ["raw", "plain text"],
      "socket-error",
      "close",
    ]);
  });

  it("sends input, resize, and signal frames through the active socket", () => {
    const client = createTerminalSocketClient({
      WebSocketCtor: FakeWebSocket,
      location: { protocol: "http:", host: "localhost:5173" },
    });

    client.connect("term-2");
    FakeWebSocket.instances[0].open();
    client.sendInput("ls\n");
    client.sendResize({ cols: 120, rows: 36 });
    client.sendSignal("SIGINT");

    expect(FakeWebSocket.instances[0].sent).toEqual([
      JSON.stringify({ type: "input", data: "ls\n" }),
      JSON.stringify({ type: "resize", cols: 120, rows: 36 }),
      JSON.stringify({ type: "signal", signal: "SIGINT" }),
    ]);
  });
});
