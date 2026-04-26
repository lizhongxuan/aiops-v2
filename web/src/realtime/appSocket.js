function resolveSocketUrl(location, path = "/ws") {
  const protocol = location?.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${location?.host || ""}${path}`;
}

export function createAppSocketClient(options = {}) {
  const {
    WebSocketCtor = WebSocket,
    location = window.location,
    pingIntervalMs = 10000,
    heartbeatMs = 45000,
    reconnectDelayMs = 1000,
    maxRetries = 5,
    shouldRestoreState = () => false,
    onStatusChange,
    onHeartbeat,
    onSnapshot,
    onAgentEvent,
    onProtocolError,
    onError,
    onRestoreRequired,
    onStopped,
  } = options;

  const timers = options.timers || window;
  let socket = null;
  let retryCount = 0;
  let manualClose = false;
  let heartbeatTimer = null;
  let pingTimer = null;
  let reconnectTimer = null;

  function setStatus(status) {
    onStatusChange?.(status, { retryCount });
  }

  function clearTimers() {
    if (heartbeatTimer) {
      timers.clearTimeout(heartbeatTimer);
      heartbeatTimer = null;
    }
    if (pingTimer) {
      timers.clearInterval(pingTimer);
      pingTimer = null;
    }
    if (reconnectTimer) {
      timers.clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  }

  function refreshHeartbeat() {
    if (heartbeatTimer) {
      timers.clearTimeout(heartbeatTimer);
    }
    heartbeatTimer = timers.setTimeout(() => {
      if (socket && socket.readyState === (WebSocketCtor.OPEN ?? 1)) {
        socket.close();
      }
    }, heartbeatMs);
  }

  function connect() {
    manualClose = false;
    clearTimers();
    setStatus("connecting");
    socket = new WebSocketCtor(resolveSocketUrl(location));

    socket.onopen = () => {
      const needsRestore = retryCount > 0 || shouldRestoreState();
      clearTimers();
      refreshHeartbeat();
      pingTimer = timers.setInterval(() => {
        if (socket?.readyState === (WebSocketCtor.OPEN ?? 1)) {
          socket.send(JSON.stringify({ type: "ping" }));
        }
      }, pingIntervalMs);
      setStatus("connected");
      if (needsRestore) {
        onRestoreRequired?.();
      }
      retryCount = 0;
    };

    socket.onmessage = (event) => {
      refreshHeartbeat();
      try {
        const payload = JSON.parse(event.data);
        if (payload?.type === "heartbeat") {
          onHeartbeat?.(payload);
          return;
        }
        if (payload?.type === "agent_event" && payload?.event) {
          onAgentEvent?.(payload.event);
          return;
        }
        if (payload?.type === "snapshot") {
          onSnapshot?.(payload.snapshot || payload.state || payload);
          return;
        }
        if (payload?.type) {
          onProtocolError?.(payload);
          return;
        }
        onSnapshot?.(payload);
      } catch (error) {
        onError?.(error, event.data);
      }
    };

    socket.onerror = (event) => {
      onError?.(event);
    };

    socket.onclose = () => {
      clearTimers();
      setStatus("disconnected");
      if (manualClose) {
        socket = null;
        return;
      }
      retryCount += 1;
      if (retryCount > maxRetries) {
        socket = null;
        onStopped?.({ retryCount });
        return;
      }
      setStatus("reconnecting");
      reconnectTimer = timers.setTimeout(() => {
        reconnectTimer = null;
        if (manualClose) {
          return;
        }
        connect();
      }, reconnectDelayMs);
    };
  }

  function disconnect() {
    manualClose = true;
    clearTimers();
    if (socket) {
      const activeSocket = socket;
      socket = null;
      activeSocket.close();
    }
  }

  function getSocket() {
    return socket;
  }

  return {
    connect,
    disconnect,
    getSocket,
  };
}
