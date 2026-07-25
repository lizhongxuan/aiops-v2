import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from "react";

import type { WorkflowAuthoritativeProjection } from "@/runner/workspace/WorkflowWorkspaceProvider";

export type AppSnapshot = {
  kind?: string;
  config?: {
    workflowWorkspace?: WorkflowAuthoritativeProjection;
    [key: string]: unknown;
  };
  [key: string]: unknown;
};

export type AppSnapshotConnection = "connecting" | "online" | "reconnecting" | "offline";

type AppSnapshotContextValue = {
  snapshot?: AppSnapshot;
  connection: AppSnapshotConnection;
};

const AppSnapshotContext = createContext<AppSnapshotContextValue>({ connection: "connecting" });

export function AppSnapshotProvider({ children }: PropsWithChildren) {
  const [snapshot, setSnapshot] = useState<AppSnapshot>();
  const [connection, setConnection] = useState<AppSnapshotConnection>("connecting");

  useEffect(() => {
    let disposed = false;
    let socket: WebSocket | undefined;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    let reconnectAttempt = 0;

    const connect = () => {
      if (disposed || typeof WebSocket === "undefined") {
        setConnection("offline");
        return;
      }
      setConnection(reconnectAttempt === 0 ? "connecting" : "reconnecting");
      socket = new WebSocket(mainAppWebSocketURL());
      socket.onopen = () => {
        reconnectAttempt = 0;
        setConnection("online");
      };
      socket.onmessage = (event) => {
        const next = parseAppSnapshot(event.data);
        if (next) setSnapshot(next);
      };
      socket.onerror = () => socket?.close();
      socket.onclose = () => {
        if (disposed) return;
        setConnection("reconnecting");
        reconnectAttempt += 1;
        const delay = Math.min(5_000, 250 * 2 ** Math.min(reconnectAttempt - 1, 4));
        reconnectTimer = setTimeout(connect, delay);
      };
    };

    connect();
    return () => {
      disposed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      socket?.close();
    };
  }, []);

  const value = useMemo(() => ({ snapshot, connection }), [connection, snapshot]);
  return <AppSnapshotContext.Provider value={value}>{children}</AppSnapshotContext.Provider>;
}

export function useAppSnapshot() {
  return useContext(AppSnapshotContext);
}

export function mainAppWebSocketURL(location: Pick<Location, "protocol" | "host"> = window.location) {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${location.host}/ws`;
}

export function parseAppSnapshot(value: unknown): AppSnapshot | undefined {
  let candidate: unknown = value;
  if (typeof candidate === "string") {
    try {
      candidate = JSON.parse(candidate);
    } catch {
      return undefined;
    }
  }
  if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) return undefined;
  const record = candidate as Record<string, unknown>;
  if (record.type === "heartbeat" || record.type === "agent_event") return undefined;
  if (typeof record.kind !== "string" && !record.config) return undefined;
  return record as AppSnapshot;
}
