package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

func TestHostAgentAPIRegisterAcceptsBearerToken(t *testing.T) {
	dataDir := t.TempDir()
	dataStore, err := store.NewJSONFileStore(dataDir, 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()
	token := "expected-agent-token"
	if err := dataStore.SaveHost(&store.HostRecord{
		ID:            "host-a",
		Name:          "host-a",
		Address:       "10.0.0.11",
		Status:        "installing",
		InstallState:  "running",
		AgentTokenRef: hostAgentTokenHashRefForTest(token),
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	sessionMgr := runtimekernel.NewSessionManager(dataStore)
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"hostId":        "host-a",
		"hostname":      "prod-web-01",
		"os":            "linux",
		"arch":          "amd64",
		"osRelease":     "Ubuntu 24.04 LTS",
		"kernelVersion": "6.8.0-31-generic",
		"cpuCores":      8,
		"memoryBytes":   34359738368,
		"agentVersion":  "v0.1.0",
		"listenAddress": ":7072",
		"capabilities":  []string{"script.shell", "terminal"},
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/host-agents/register", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST register error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST register status = %d, want 200", resp.StatusCode)
	}
	var payload appui.HostAgentRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if payload.Status != "online" || payload.Host.Status != "online" {
		t.Fatalf("register response = %+v", payload)
	}
	if payload.Host.OSRelease != "Ubuntu 24.04 LTS" || payload.Host.KernelVersion != "6.8.0-31-generic" || payload.Host.CPUCores != 8 || payload.Host.MemoryBytes != 34359738368 {
		t.Fatalf("register host system basics = %+v", payload.Host)
	}
}

func TestHostAgentAPIHeartbeatAcceptsHeaderToken(t *testing.T) {
	dataDir := t.TempDir()
	dataStore, err := store.NewJSONFileStore(dataDir, 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()
	token := "expected-agent-token"
	if err := dataStore.SaveHost(&store.HostRecord{
		ID:            "host-a",
		Name:          "host-a",
		Address:       "10.0.0.11",
		Status:        "online",
		InstallState:  "installed",
		AgentTokenRef: hostAgentTokenHashRefForTest(token),
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	sessionMgr := runtimekernel.NewSessionManager(dataStore)
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"hostId":        "host-a",
		"agentVersion":  "v0.1.0",
		"osRelease":     "Debian GNU/Linux 12",
		"kernelVersion": "6.1.0-21-amd64",
		"cpuCores":      4,
		"memoryBytes":   8589934592,
	})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/host-agents/heartbeat", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Host-Agent-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST heartbeat error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST heartbeat status = %d, want 200", resp.StatusCode)
	}
	var payload appui.HostAgentHeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode heartbeat response: %v", err)
	}
	if payload.Status != "online" || payload.LastHeartbeat == "" {
		t.Fatalf("heartbeat response = %+v", payload)
	}
	if payload.Host.OSRelease != "Debian GNU/Linux 12" || payload.Host.KernelVersion != "6.1.0-21-amd64" || payload.Host.CPUCores != 4 || payload.Host.MemoryBytes != 8589934592 {
		t.Fatalf("heartbeat host system basics = %+v", payload.Host)
	}
}

func TestHostAgentAPIRejectsWrongToken(t *testing.T) {
	dataDir := t.TempDir()
	dataStore, err := store.NewJSONFileStore(dataDir, 10)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer dataStore.Close()
	if err := dataStore.SaveHost(&store.HostRecord{
		ID:            "host-a",
		Name:          "host-a",
		Status:        "online",
		AgentTokenRef: hostAgentTokenHashRefForTest("expected-agent-token"),
	}); err != nil {
		t.Fatalf("SaveHost() error = %v", err)
	}

	sessionMgr := runtimekernel.NewSessionManager(dataStore)
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{"hostId": "host-a", "agentVersion": "v0.1.0"})
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/host-agents/heartbeat", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST heartbeat error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST heartbeat status = %d, want 401", resp.StatusCode)
	}
}

func hostAgentTokenHashRefForTest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("sha256:%x", sum[:])
}
