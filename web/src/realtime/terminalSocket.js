function resolveTerminalSocketUrl(location, sessionId) {
  const protocol = location?.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${location?.host || ""}/api/v1/terminal/ws?sessionId=${encodeURIComponent(sessionId)}`;
}

export function createTerminalSocketClient(options = {}) {
  const {
    WebSocketCtor = WebSocket,
    location = window.location,
    onOpen,
    onReady,
    onOutput,
    onStatus,
    onExit,
    onErrorMessage,
    onRawMessage,
    onClose,
    onSocketError,
  } = options;

  let socket = null;

  function connect(sessionId) {
    socket = new WebSocketCtor(resolveTerminalSocketUrl(location, sessionId));
    socket.onopen = () => onOpen?.();
    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        switch (message?.type) {
          case "ready":
            onReady?.(message);
            break;
          case "output":
            onOutput?.(message);
            break;
          case "status":
            onStatus?.(message);
            break;
          case "exit":
            onExit?.(message);
            break;
          case "error":
            onErrorMessage?.(message);
            break;
          default:
            onRawMessage?.(event.data);
            break;
        }
      } catch {
        onRawMessage?.(event.data);
      }
    };
    socket.onclose = () => onClose?.();
    socket.onerror = (event) => onSocketError?.(event);
    return socket;
  }

  function send(payload) {
    if (socket?.readyState === (WebSocketCtor.OPEN ?? 1)) {
      socket.send(JSON.stringify(payload));
    }
  }

  function sendInput(data) {
    send({ type: "input", data });
  }

  function sendResize({ cols, rows }) {
    send({ type: "resize", cols, rows });
  }

  function sendSignal(signal) {
    send({ type: "signal", signal });
  }

  function disconnect() {
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
    sendInput,
    sendResize,
    sendSignal,
  };
}
