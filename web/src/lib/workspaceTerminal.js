import { createTerminalSession } from "../api/terminal";
import { createTerminalSocketClient } from "../realtime/terminalSocket";

export function normalizeWorkspaceTerminalOutput(source) {
  if (source === null || source === undefined) {
    return "";
  }
  if (Array.isArray(source)) {
    return source.map((item) => String(item ?? "").replace(/\r\n/g, "\n")).join("\n");
  }
  if (typeof source === "object") {
    const fields = ["output", "stdout", "text", "summary", "value", "data"];
    for (const field of fields) {
      const value = source[field];
      if (typeof value === "string" && value.trim()) {
        return value;
      }
      if (Array.isArray(value) && value.length) {
        return value.map((item) => String(item ?? "")).join("\n");
      }
    }
    if (Array.isArray(source.lines) && source.lines.length) {
      return source.lines.map((item) => String(item ?? "")).join("\n");
    }
    if (Array.isArray(source.messages) && source.messages.length) {
      return source.messages
        .map((item) => {
          if (typeof item === "string") {
            return item;
          }
          if (item && typeof item === "object") {
            return String(item.text || item.message || item.summary || "").trim();
          }
          return "";
        })
        .filter(Boolean)
        .join("\n");
    }
  }
  return String(source).replace(/\r\n/g, "\n");
}

export function normalizeWorkspaceTerminalLines(source) {
  const output = normalizeWorkspaceTerminalOutput(source);
  if (!output) {
    return [];
  }
  return output.split("\n");
}

export async function createWorkspaceTerminalSession({ hostId, cwd = "~", shell = "/bin/zsh", cols = 120, rows = 36 }) {
  return createTerminalSession({ hostId, cwd, shell, cols, rows });
}

export function openWorkspaceTerminalSocket(sessionId, handlers = {}) {
  const client = createTerminalSocketClient({
    onOpen: handlers.onOpen,
    onReady: handlers.onReady,
    onOutput: handlers.onOutput,
    onExit: handlers.onExit,
    onStatus: handlers.onStatus,
    onErrorMessage: handlers.onError,
    onRawMessage: handlers.onRawMessage,
    onClose: handlers.onClose,
    onSocketError: handlers.onSocketError,
  });
  client.connect(sessionId);
  return client;
}
