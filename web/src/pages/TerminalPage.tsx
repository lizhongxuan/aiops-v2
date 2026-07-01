import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { ArrowLeft, Eraser, Maximize2, OctagonX } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useRegisterAppShellPageChrome } from "@/app/AppShellChromeContext";
import { ToneBadge } from "@/pages/settingsComponents";

type TerminalSession = {
  sessionId?: string;
  id?: string;
  hostId?: string;
  cwd?: string;
  shell?: string;
  startedAt?: string;
};

async function requestJson<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(new URL(path, window.location.origin).toString(), { credentials: "include", ...init, headers: { "Content-Type": "application/json", ...(init.headers || {}) } });
  const payload = (await response.json().catch(() => ({}))) as T;
  if (!response.ok) throw new Error(`Request failed: ${response.status}`);
  return payload;
}

function socketUrl(sessionId: string) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/api/v1/terminal/ws?sessionId=${encodeURIComponent(sessionId)}`;
}

export function TerminalPage() {
  const params = useParams();
  const hostId = String(params.hostId || "");
  const terminalElementRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const [session, setSession] = useState<TerminalSession | null>(null);
  const [status, setStatus] = useState("initializing");
  const [error, setError] = useState("");

  useEffect(() => {
    const terminal = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontFamily: '"SF Mono", "Fira Code", "Menlo", "Monaco", "Consolas", monospace',
      fontSize: 13,
      theme: { background: "#0f172a", foreground: "#e2e8f0" },
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;
    if (terminalElementRef.current) {
      terminal.open(terminalElementRef.current);
      fitAddon.fit();
      terminal.write("gRPC host client terminal initializing...\r\n");
    }
    const disposable = terminal.onData((data) => {
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "input", data }));
    });
    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "resize", cols: terminal.cols, rows: terminal.rows }));
    });
    if (terminalElementRef.current) resizeObserver.observe(terminalElementRef.current);
    return () => {
      disposable.dispose();
      resizeObserver.disconnect();
      socketRef.current?.close();
      terminal.dispose();
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function openSession() {
      if (!hostId) return;
      setStatus("connecting");
      setError("");
      try {
        const payload = await requestJson<TerminalSession>("/api/v1/terminal/sessions", {
          method: "POST",
          body: JSON.stringify({ hostId, cols: terminalRef.current?.cols || 120, rows: terminalRef.current?.rows || 36 }),
        });
        if (cancelled) return;
        const sessionId = payload.sessionId || payload.id || "";
        setSession({ ...payload, sessionId, hostId });
        terminalRef.current?.write(`Connected session ${sessionId || "-"} for ${hostId}\r\n`);
        if (sessionId) {
          socketRef.current?.close();
          const socket = new WebSocket(socketUrl(sessionId));
          socketRef.current = socket;
          socket.onopen = () => setStatus("connected");
          socket.onmessage = (event) => {
            try {
              const message = JSON.parse(event.data);
              if (message.type === "output") terminalRef.current?.write(String(message.data || message.output || ""));
              if (message.type === "ready") setStatus("ready");
              if (message.type === "status") setStatus(String(message.status || "connected"));
              if (message.type === "exit") setStatus("exited");
              if (message.type === "error") setError(String(message.message || "terminal error"));
            } catch {
              terminalRef.current?.write(String(event.data));
            }
          };
          socket.onclose = () => setStatus((current) => current === "exited" ? current : "closed");
          socket.onerror = () => setError("WebSocket 连接失败");
        }
      } catch (cause) {
        if (!cancelled) {
          setStatus("error");
          setError((cause as Error).message || "创建终端会话失败");
        }
      }
    }
    void openSession();
    return () => {
      cancelled = true;
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [hostId]);

  function sendSignal(signal: string) {
    const socket = socketRef.current;
    if (socket?.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "signal", signal }));
  }

  const chromeActions = useMemo(
    () => (
      <>
        <Button asChild type="button" size="sm" variant="outline">
          <Link to="/settings/hosts" title="返回主机管理列表">
            <ArrowLeft />
            退出
          </Link>
        </Button>
        <ToneBadge tone={status === "ready" || status === "connected" ? "success" : status === "error" ? "danger" : "warning"}>{status}</ToneBadge>
        <Button size="sm" variant="outline" onClick={() => terminalRef.current?.clear()}>
          <Eraser />
          清屏
        </Button>
        <Button size="sm" variant="outline" onClick={() => sendSignal("SIGINT")}>
          <OctagonX />
          Ctrl-C
        </Button>
        <Button size="sm" variant="outline" onClick={() => fitAddonRef.current?.fit()}>
          <Maximize2 />
          Fit
        </Button>
      </>
    ),
    [status],
  );

  useRegisterAppShellPageChrome({
    title: `Terminal · ${hostId || "host"}`,
    description: `gRPC 主机客户端 · terminal session + websocket · ${session?.shell || "shell"} · ${session?.cwd || "~"}`,
    actions: chromeActions,
  });

  return (
    <section data-testid="terminal-page" className="flex h-full min-h-0 flex-col bg-slate-950 text-slate-100">
      {error ? <div className="shrink-0 border-b border-red-500/40 bg-red-950/80 px-4 py-2 text-sm text-red-100">{error}</div> : null}
      <div
        ref={terminalElementRef}
        className="min-h-0 flex-1 overflow-hidden bg-slate-950 p-2 [&_.xterm]:h-full"
        data-testid="terminal-xterm"
      />
    </section>
  );
}
