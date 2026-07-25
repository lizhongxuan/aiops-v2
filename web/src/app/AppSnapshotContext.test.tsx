import { act, useEffect } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  AppSnapshotProvider,
  mainAppWebSocketURL,
  parseAppSnapshot,
  useAppSnapshot,
} from "./AppSnapshotContext";

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  readonly url: string;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  closed = false;

  constructor(url: string | URL) {
    this.url = String(url);
    FakeWebSocket.instances.push(this);
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    this.onclose?.();
  }
}

function Probe({ onSnapshot }: { onSnapshot: (value: ReturnType<typeof useAppSnapshot>) => void }) {
  const value = useAppSnapshot();
  useEffect(() => onSnapshot(value), [onSnapshot, value]);
  return <output>{value.connection}:{value.snapshot?.config?.workflowWorkspace?.lastSequence ?? "none"}</output>;
}

describe("AppSnapshotProvider", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.useRealTimers();
    vi.unstubAllGlobals();
    FakeWebSocket.instances = [];
  });

  it("owns one main websocket and publishes only canonical snapshots", () => {
    vi.stubGlobal("WebSocket", FakeWebSocket);
    const seen = vi.fn();
    act(() => root.render(<AppSnapshotProvider><Probe onSnapshot={seen} /></AppSnapshotProvider>));
    expect(FakeWebSocket.instances).toHaveLength(1);
    const socket = FakeWebSocket.instances[0];
    expect(socket.url).toMatch(/\/ws$/);

    act(() => socket.onopen?.());
    act(() => socket.onmessage?.({ data: JSON.stringify({ type: "heartbeat" }) }));
    expect(container.textContent).toBe("online:none");

    act(() => socket.onmessage?.({ data: JSON.stringify({
      kind: "app_snapshot",
      config: { workflowWorkspace: { schemaVersion: "aiops.workflow.workspace.v1", lastSequence: 9 } },
    }) }));
    expect(container.textContent).toBe("online:9");
    expect(seen).toHaveBeenCalled();

    act(() => root.unmount());
    expect(socket.closed).toBe(true);
    root = createRoot(container);
  });

  it("reconnects the single application stream without polling", () => {
    vi.useFakeTimers();
    vi.stubGlobal("WebSocket", FakeWebSocket);
    act(() => root.render(<AppSnapshotProvider><Probe onSnapshot={() => undefined} /></AppSnapshotProvider>));
    const first = FakeWebSocket.instances[0];
    act(() => first.onclose?.());
    expect(container.textContent).toBe("reconnecting:none");
    act(() => vi.advanceTimersByTime(250));
    expect(FakeWebSocket.instances).toHaveLength(2);
  });
});

describe("AppSnapshot helpers", () => {
  it("uses the main same-origin websocket URL", () => {
    expect(mainAppWebSocketURL({ protocol: "https:", host: "aiops.example" } as Location)).toBe("wss://aiops.example/ws");
  });

  it("rejects heartbeats, agent events, malformed JSON, and non-snapshots", () => {
    expect(parseAppSnapshot("{" )).toBeUndefined();
    expect(parseAppSnapshot({ type: "heartbeat" })).toBeUndefined();
    expect(parseAppSnapshot({ type: "agent_event", event: {} })).toBeUndefined();
    expect(parseAppSnapshot({ arbitrary: true })).toBeUndefined();
    expect(parseAppSnapshot({ kind: "app_snapshot", config: {} })).toMatchObject({ kind: "app_snapshot" });
  });
});
