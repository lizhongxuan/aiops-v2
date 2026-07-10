package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/appui"
	"golang.org/x/net/websocket"
)

type terminalServiceProvider interface {
	appui.HTTPServices
	TerminalService() appui.TerminalService
}

func terminalServiceFromHTTP(ui appui.HTTPServices) (appui.TerminalService, bool) {
	if provider, ok := ui.(terminalServiceProvider); ok {
		return provider.TerminalService(), true
	}
	return nil, false
}

func (s *HTTPServer) handleTerminalSessions(w http.ResponseWriter, r *http.Request) {
	terminalSvc, _ := terminalServiceFromHTTP(s.ui)
	if terminalSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal service is not configured"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		resp, err := terminalSvc.ListSessions(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req appui.TerminalCreateSessionCommand
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		meta, err := terminalSvc.CreateSession(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, terminalCreateSessionErrorPayload(err))
			return
		}
		writeJSON(w, http.StatusOK, meta)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func terminalCreateSessionErrorPayload(err error) map[string]any {
	raw := ""
	if err != nil {
		raw = strings.TrimSpace(err.Error())
	}
	if raw == "" {
		raw = "创建终端会话失败"
	}
	payload := map[string]any{"error": raw}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "read ssh credential") && (strings.Contains(lower, "no such file") || strings.Contains(lower, "cannot find")):
		payload["code"] = "ssh_credential_secret_missing"
		payload["message"] = "SSH 凭证文件缺失，无法创建远程终端"
		payload["detail"] = raw
		payload["diagnostics"] = []string{
			"主机记录里保存了 SSH 凭证引用，但本机数据目录下找不到对应 secret 文件。",
			"常见原因是切换了 AIOPS_DATA_DIR、清理了 .data/secrets、换机器部署时没有迁移 secrets，或主机配置从旧环境复制过来。",
			"先确认当前 ai-server 使用的数据目录是否正确，再检查该目录下的 secrets/hosts 子目录是否完整。",
		}
		payload["nextSteps"] = []string{
			"进入主机列表，编辑该主机，重新输入 SSH 密码并点击 SSH 测试，让系统重新生成本地 secret。",
			"如果这是迁移后的环境，确认旧环境的 .data/secrets 已随 .data/hosts 一起迁移。",
			"修复后重新打开终端页面，状态应从 error 变为 connected/ready。",
		}
	case strings.Contains(lower, "ssh credential ref is required"):
		payload["code"] = "ssh_credential_not_configured"
		payload["message"] = "该主机未配置 SSH 凭证，无法创建远程终端"
		payload["detail"] = raw
		payload["diagnostics"] = []string{
			"主机可执行或启用了终端，但缺少 SSHCredentialRef 或对应 SSH 密码/密钥。",
		}
		payload["nextSteps"] = []string{
			"进入主机列表，编辑该主机，填写 SSH 用户、端口和 SSH 密码，然后点击 SSH 测试。",
		}
	case strings.Contains(lower, "terminal is not enabled"):
		payload["code"] = "terminal_not_enabled"
		payload["message"] = "该主机未启用终端能力"
		payload["detail"] = raw
		payload["diagnostics"] = []string{
			"主机当前没有 terminalCapable 或 executable 能力，系统不会为它创建交互式终端。",
		}
		payload["nextSteps"] = []string{
			"进入主机列表检查该主机的连接方式、Agent 状态和终端能力配置。",
		}
	case strings.Contains(lower, "host ") && strings.Contains(lower, " offline"):
		payload["code"] = "host_offline"
		payload["message"] = "主机不在线，无法创建远程终端"
		payload["detail"] = raw
		payload["diagnostics"] = []string{
			"主机状态不是 online，可能是 Host Agent 心跳过期、网络不可达，或 SSH/Agent 连接配置失效。",
		}
		payload["nextSteps"] = []string{
			"进入主机列表查看 Agent 状态和最后心跳时间，必要时重新执行 SSH 测试或重装 Host Agent。",
		}
	}
	return payload
}

func (s *HTTPServer) handleTerminalWebSocket() websocket.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		if s.terminalManager == nil {
			return
		}
		req := conn.Request()
		sessionID := ""
		if req != nil {
			sessionID = strings.TrimSpace(req.URL.Query().Get("sessionId"))
		}
		session, events, release, err := s.terminalManager.Subscribe(sessionID)
		if err != nil {
			_ = websocket.JSON.Send(conn, map[string]any{
				"type":    "error",
				"message": err.Error(),
			})
			return
		}
		defer release()

		writeMu := &sync.Mutex{}
		sendJSON := func(payload any) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return websocket.JSON.Send(conn, payload)
		}

		ctx := conn.Request().Context()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				var msg map[string]any
				if err := websocket.JSON.Receive(conn, &msg); err != nil {
					return
				}
				switch strings.TrimSpace(asString(msg["type"])) {
				case "input":
					_ = session.SendInput(asString(msg["data"]))
				case "resize":
					session.Resize(asInt(msg["cols"]), asInt(msg["rows"]))
				case "signal":
					_ = session.Signal(asString(msg["signal"]))
				case "close":
					_ = session.Close()
				case "ping":
					_ = sendJSON(map[string]any{"type": "heartbeat", "time": time.Now().UTC().Format(time.RFC3339Nano)})
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case evt, ok := <-events:
				if !ok {
					return
				}
				_ = sendJSON(evt)
			}
		}
	})
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
