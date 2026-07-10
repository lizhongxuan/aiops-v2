package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
	"aiops-v2/internal/terminal"
	"golang.org/x/net/websocket"
)

type terminalHostRepoStub struct {
	items map[string]store.HostRecord
}

func newTerminalHostRepoStub(records ...store.HostRecord) *terminalHostRepoStub {
	repo := &terminalHostRepoStub{items: map[string]store.HostRecord{}}
	for _, record := range records {
		repo.items[record.ID] = record
	}
	return repo
}

func (r *terminalHostRepoStub) GetHost(id string) (*store.HostRecord, error) {
	record, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("host not found")
	}
	cp := record
	return &cp, nil
}

func (r *terminalHostRepoStub) ListHosts() ([]store.HostRecord, error) {
	items := make([]store.HostRecord, 0, len(r.items))
	for _, record := range r.items {
		items = append(items, record)
	}
	return items, nil
}

func (r *terminalHostRepoStub) SaveHost(host *store.HostRecord) error {
	r.items[host.ID] = *host
	return nil
}

func (r *terminalHostRepoStub) DeleteHost(id string) error {
	delete(r.items, id)
	return nil
}

func TestTerminalAPI_CreateListAndLifecycle(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	terminalMgr := terminal.NewManager(terminal.WithCommandFactory(func(req terminal.CreateSessionRequest) (*exec.Cmd, error) {
		return exec.Command("/bin/cat"), nil
	}))
	srv := NewHTTPServer(
		appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithTerminalManager(terminalMgr)),
		WithTerminalManager(terminalMgr),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody, _ := json.Marshal(map[string]any{
		"hostId": "host-a",
		"cwd":    "/tmp",
		"shell":  "/bin/cat",
		"cols":   80,
		"rows":   24,
	})
	createResp, err := http.Post(ts.URL+"/api/v1/terminal/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/v1/terminal/sessions error = %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200", createResp.StatusCode)
	}

	var created terminal.SessionMetadata
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.SessionID == "" || created.HostID != "host-a" || created.Shell != "/bin/cat" {
		t.Fatalf("created session = %+v", created)
	}

	listResp, err := http.Get(ts.URL + "/api/v1/terminal/sessions")
	if err != nil {
		t.Fatalf("GET /api/v1/terminal/sessions error = %v", err)
	}
	defer listResp.Body.Close()
	var listed struct {
		Sessions []terminal.SessionMetadata `json:"sessions"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed.Sessions) != 1 || listed.Sessions[0].SessionID != created.SessionID {
		t.Fatalf("listed sessions = %+v, want created session", listed.Sessions)
	}

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/terminal/ws?sessionId=" + created.SessionID
	conn, err := websocket.Dial(wsURL, "", "http://example.test/")
	if err != nil {
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	var ready map[string]any
	if err := websocket.JSON.Receive(conn, &ready); err != nil {
		t.Fatalf("receive ready: %v", err)
	}
	if ready["type"] != "ready" {
		t.Fatalf("ready.type = %v, want ready", ready["type"])
	}
	if ready["sessionId"] != created.SessionID {
		t.Fatalf("ready.sessionId = %v, want %s", ready["sessionId"], created.SessionID)
	}

	if err := websocket.JSON.Send(conn, map[string]any{"type": "input", "data": "hello terminal\n"}); err != nil {
		t.Fatalf("send input: %v", err)
	}

	var output map[string]any
	if err := websocket.JSON.Receive(conn, &output); err != nil {
		t.Fatalf("receive output: %v", err)
	}
	if output["type"] != "output" {
		t.Fatalf("output.type = %v, want output", output["type"])
	}
	if text, _ := output["data"].(string); text == "" || !strings.Contains(text, "hello terminal") {
		t.Fatalf("output.data = %v, want echoed input", output["data"])
	}

	if err := websocket.JSON.Send(conn, map[string]any{"type": "resize", "cols": 100, "rows": 40}); err != nil {
		t.Fatalf("send resize: %v", err)
	}

	if err := websocket.JSON.Send(conn, map[string]any{"type": "close"}); err != nil {
		t.Fatalf("send close: %v", err)
	}

	var exit map[string]any
	if err := websocket.JSON.Receive(conn, &exit); err != nil {
		t.Fatalf("receive exit: %v", err)
	}
	if exit["type"] != "exit" {
		t.Fatalf("exit.type = %v, want exit", exit["type"])
	}

	updated, err := http.Get(ts.URL + "/api/v1/terminal/sessions")
	if err != nil {
		t.Fatalf("GET updated sessions error = %v", err)
	}
	defer updated.Body.Close()
	var afterClose struct {
		Sessions []terminal.SessionMetadata `json:"sessions"`
	}
	if err := json.NewDecoder(updated.Body).Decode(&afterClose); err != nil {
		t.Fatalf("decode updated sessions: %v", err)
	}
	if len(afterClose.Sessions) != 1 || afterClose.Sessions[0].Status != terminal.SessionStatusExited {
		t.Fatalf("after close sessions = %+v, want exited", afterClose.Sessions)
	}
}

func TestTerminalAPI_CreateRejectsHostWithoutTerminalPermission(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	terminalMgr := terminal.NewManager(terminal.WithCommandFactory(func(req terminal.CreateSessionRequest) (*exec.Cmd, error) {
		return exec.Command("/bin/cat"), nil
	}))
	hostRepo := newTerminalHostRepoStub(store.HostRecord{
		ID:     "readonly",
		Status: "online",
	})
	srv := NewHTTPServer(
		appui.NewServices(
			sessionAPITestRuntime{},
			sessionMgr,
			appui.WithTerminalManager(terminalMgr),
			appui.WithHostRepository(hostRepo),
		),
		WithTerminalManager(terminalMgr),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody, _ := json.Marshal(map[string]any{"hostId": "readonly"})
	createResp, err := http.Post(ts.URL+"/api/v1/terminal/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/v1/terminal/sessions error = %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400", createResp.StatusCode)
	}
	var payload struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Code != "terminal_not_enabled" {
		t.Fatalf("code = %q, want terminal_not_enabled", payload.Code)
	}
	if !strings.Contains(payload.Error, "terminal is not enabled") {
		t.Fatalf("error = %q, want terminal permission message", payload.Error)
	}
}

func TestTerminalAPI_CreateReturnsActionableGuidanceForMissingSSHSecret(t *testing.T) {
	sessionMgr := runtimekernel.NewSessionManager()
	terminalMgr := terminal.NewManager(terminal.WithCommandFactory(func(req terminal.CreateSessionRequest) (*exec.Cmd, error) {
		return nil, fmt.Errorf("read ssh credential secret://hosts/remote/ssh-password: open .data/secrets/hosts/remote/ssh-password: no such file or directory")
	}))
	srv := NewHTTPServer(
		appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithTerminalManager(terminalMgr)),
		WithTerminalManager(terminalMgr),
	)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody, _ := json.Marshal(map[string]any{"hostId": "remote"})
	createResp, err := http.Post(ts.URL+"/api/v1/terminal/sessions", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/v1/terminal/sessions error = %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400", createResp.StatusCode)
	}
	var payload struct {
		Code        string   `json:"code"`
		Error       string   `json:"error"`
		Message     string   `json:"message"`
		Detail      string   `json:"detail"`
		Diagnostics []string `json:"diagnostics"`
		NextSteps   []string `json:"nextSteps"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Code != "ssh_credential_secret_missing" {
		t.Fatalf("code = %q, want ssh_credential_secret_missing; payload = %+v", payload.Code, payload)
	}
	if !strings.Contains(payload.Message, "SSH 凭证文件缺失") {
		t.Fatalf("message = %q, want user-facing credential message", payload.Message)
	}
	if !strings.Contains(payload.Detail, "read ssh credential") {
		t.Fatalf("detail = %q, want raw diagnostic detail", payload.Detail)
	}
	if strings.Join(payload.Diagnostics, "\n") == "" || !strings.Contains(strings.Join(payload.Diagnostics, "\n"), "AIOPS_DATA_DIR") {
		t.Fatalf("diagnostics = %+v, want AIOPS_DATA_DIR diagnostic hint", payload.Diagnostics)
	}
	if strings.Join(payload.NextSteps, "\n") == "" || !strings.Contains(strings.Join(payload.NextSteps, "\n"), "重新输入 SSH 密码") {
		t.Fatalf("nextSteps = %+v, want re-enter password guidance", payload.NextSteps)
	}
	if payload.Error == "" {
		t.Fatalf("error must remain populated for old clients")
	}
}
