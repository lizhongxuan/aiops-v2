package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
	stdcontext "context"
)

type mcpAPITestRuntime struct{}

func (mcpAPITestRuntime) RunTurn(stdcontext.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (mcpAPITestRuntime) ResumeTurn(stdcontext.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (mcpAPITestRuntime) CancelTurn(stdcontext.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

type mcpAPITestRepo struct {
	items []store.MCPServerRecord
}

func (r *mcpAPITestRepo) GetMCPServers() ([]store.MCPServerRecord, error) {
	out := make([]store.MCPServerRecord, 0, len(r.items))
	for _, item := range r.items {
		cp := item
		cp.Args = append([]string(nil), cp.Args...)
		cp.Env = cloneStringMap(cp.Env)
		out = append(out, cp)
	}
	return out, nil
}

func (r *mcpAPITestRepo) SaveMCPServers(items []store.MCPServerRecord) error {
	r.items = make([]store.MCPServerRecord, 0, len(items))
	for _, item := range items {
		cp := item
		cp.Args = append([]string(nil), cp.Args...)
		cp.Env = cloneStringMap(cp.Env)
		r.items = append(r.items, cp)
	}
	return nil
}

func TestMCPServersAPI(t *testing.T) {
	repo := &mcpAPITestRepo{
		items: []store.MCPServerRecord{
			{
				Name:      "docs",
				Transport: "stdio",
				Command:   "docs-mcp",
				Args:      []string{"--flag"},
				Disabled:  true,
				Status:    "closed",
			},
		},
	}
	registry := mcp.NewRegistry()
	if err := registry.RegisterServer(mcp.ServerConfig{
		ID:        "generator",
		Name:      "generator",
		Transport: "local",
		Command:   []string{"generator"},
		Source:    "builtin",
	}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	if err := registry.OnServerConnected("generator", []tooling.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{Name: "generator.generate", Description: "generate"},
			ExecuteFunc: func(stdcontext.Context, json.RawMessage) (tooling.ToolResult, error) {
				return tooling.ToolResult{}, nil
			},
		},
	}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}
	registry.SetServerHealthSnapshot(mcp.HealthSnapshot{
		ServerID:      "generator",
		Status:        mcp.HealthUnavailable,
		LastCheckedAt: time.Unix(100, 0),
		LastError:     "502 bad gateway token=secret",
		TTLSeconds:    30,
		Capabilities:  []string{"tools"},
	})

	srv := NewHTTPServer(appui.NewServices(mcpAPITestRuntime{}, nil, appui.WithMCPRepository(repo), appui.WithMCPRegistry(registry)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var listed struct {
		ConfigPath string                `json:"configPath"`
		Items      []appui.MCPServerView `json:"items"`
	}
	if err := getJSON(ts.URL+"/api/v1/mcp/servers", &listed); err != nil {
		t.Fatalf("GET /api/v1/mcp/servers error = %v", err)
	}
	if listed.ConfigPath != "mcp-servers.json" {
		t.Fatalf("configPath = %q, want mcp-servers.json", listed.ConfigPath)
	}
	if len(listed.Items) != 2 {
		t.Fatalf("items = %+v, want 2 servers", listed.Items)
	}
	if generator := findMCPAPIItem(listed.Items, "generator"); generator == nil || generator.Health.Status != mcp.HealthUnavailable {
		t.Fatalf("generator health = %+v, want unavailable", generator)
	} else if strings.Contains(generator.Health.LastError, "secret") {
		t.Fatalf("generator health leaked secret: %+v", generator.Health)
	}

	createBody, _ := json.Marshal(appui.MCPServerUpsert{
		Name:      "search",
		Transport: "http",
		URL:       "http://127.0.0.1:9000/mcp",
	})
	createResp, err := http.Post(ts.URL+"/api/v1/mcp/servers", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/v1/mcp/servers error = %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/v1/mcp/servers status = %d, body = %s", createResp.StatusCode, bodyString(createResp))
	}
	if _, ok := registry.GetServer("search"); !ok {
		t.Fatal("search should be registered after create")
	}

	closeResp, err := http.Post(ts.URL+"/api/v1/mcp/servers/search/close", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/v1/mcp/servers/search/close error = %v", err)
	}
	defer closeResp.Body.Close()
	if closeResp.StatusCode != http.StatusOK {
		t.Fatalf("close status = %d, body = %s", closeResp.StatusCode, bodyString(closeResp))
	}
	if !registry.IsServerDisabled("search") {
		t.Fatal("search should be disabled after close")
	}

	openResp, err := http.Post(ts.URL+"/api/v1/mcp/servers/search/open", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/v1/mcp/servers/search/open error = %v", err)
	}
	defer openResp.Body.Close()
	if openResp.StatusCode != http.StatusOK {
		t.Fatalf("open status = %d, body = %s", openResp.StatusCode, bodyString(openResp))
	}
	if _, ok := registry.GetServer("search"); !ok {
		t.Fatal("search should be registered after open")
	}
	if registry.IsServerDisabled("search") {
		t.Fatal("search should be re-enabled after open")
	}

	refreshResp, err := http.Post(ts.URL+"/api/v1/mcp/servers/refresh", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/v1/mcp/servers/refresh error = %v", err)
	}
	defer refreshResp.Body.Close()
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", refreshResp.StatusCode, bodyString(refreshResp))
	}
}

func findMCPAPIItem(items []appui.MCPServerView, name string) *appui.MCPServerView {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func getJSON(url string, target any) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func bodyString(resp *http.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return buf.String()
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
