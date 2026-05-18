package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/store"
)

func TestHostAgentServiceRegisterMarksHostManagedOnline(t *testing.T) {
	token := "expected-agent-token"
	repo := newHostRepoStub(store.HostRecord{
		ID:            "host-a",
		Name:          "host-a",
		Address:       "10.0.0.11",
		Status:        "installing",
		InstallState:  "running",
		AgentTokenRef: hostAgentTokenHashRef(token),
		Labels:        map[string]string{"env": "prod"},
	})
	service := NewHostAgentService(repo)

	resp, err := service.Register(context.Background(), HostAgentRegisterRequest{
		HostID:        "host-a",
		Hostname:      "prod-web-01",
		OS:            "linux",
		Arch:          "amd64",
		AgentVersion:  "v0.1.0",
		ListenAddress: ":7072",
		Labels:        map[string]string{"role": "web"},
		Capabilities:  []string{"script.shell", "terminal"},
	}, token)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if resp.Status != "online" || resp.HostID != "host-a" {
		t.Fatalf("Register() response = %+v", resp)
	}
	saved, err := repo.GetHost("host-a")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "online" || saved.InstallState != "installed" || saved.ControlMode != "managed" {
		t.Fatalf("registered host state = %+v", saved)
	}
	if saved.Transport != "agent_http" || !saved.TerminalCapable || !saved.Executable {
		t.Fatalf("registered host capabilities = %+v", saved)
	}
	if saved.OS != "linux" || saved.Arch != "amd64" || saved.AgentVersion != "v0.1.0" {
		t.Fatalf("registered host platform = %+v", saved)
	}
	if saved.AgentURL != "http://10.0.0.11:7072" {
		t.Fatalf("AgentURL = %q, want http://10.0.0.11:7072", saved.AgentURL)
	}
	if saved.LastHeartbeat == "" {
		t.Fatalf("LastHeartbeat is empty")
	}
	if saved.Labels["env"] != "prod" || saved.Labels["role"] != "web" {
		t.Fatalf("labels = %+v, want merged labels", saved.Labels)
	}
}

func TestHostAgentServiceHeartbeatUpdatesLastHeartbeat(t *testing.T) {
	token := "expected-agent-token"
	repo := newHostRepoStub(store.HostRecord{
		ID:            "host-a",
		Name:          "host-a",
		Address:       "10.0.0.11",
		Status:        "online",
		InstallState:  "installed",
		AgentVersion:  "v0.1.0",
		LastHeartbeat: "2026-05-18T00:00:00Z",
		AgentTokenRef: hostAgentTokenHashRef(token),
	})
	service := NewHostAgentService(repo)

	resp, err := service.Heartbeat(context.Background(), HostAgentHeartbeatRequest{
		HostID:       "host-a",
		AgentVersion: "v0.1.1",
		Timestamp:    "2026-05-18T10:00:00+08:00",
	}, token)
	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if resp.Status != "online" || resp.LastHeartbeat == "" {
		t.Fatalf("Heartbeat() response = %+v", resp)
	}
	saved, err := repo.GetHost("host-a")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.LastHeartbeat == "" || saved.LastHeartbeat == "2026-05-18T00:00:00Z" {
		t.Fatalf("LastHeartbeat = %q, want refreshed timestamp", saved.LastHeartbeat)
	}
	if saved.AgentVersion != "v0.1.1" {
		t.Fatalf("AgentVersion = %q, want v0.1.1", saved.AgentVersion)
	}
}

func TestHostAgentServiceRegisterBindsFirstInstallToken(t *testing.T) {
	token := "generated-on-target"
	repo := newHostRepoStub(store.HostRecord{
		ID:           "host-a",
		Name:         "host-a",
		Status:       "installing",
		InstallState: "running",
	})
	service := NewHostAgentService(repo)

	if _, err := service.Register(context.Background(), HostAgentRegisterRequest{
		HostID:       "host-a",
		OS:           "linux",
		Arch:         "amd64",
		AgentVersion: "v0.1.0",
	}, token); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	saved, err := repo.GetHost("host-a")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.AgentTokenRef != hostAgentTokenHashRef(token) {
		t.Fatalf("AgentTokenRef = %q, want first token hash", saved.AgentTokenRef)
	}
	if _, err := service.Heartbeat(context.Background(), HostAgentHeartbeatRequest{HostID: "host-a"}, "wrong-token"); err == nil {
		t.Fatalf("Heartbeat() error = nil, want bound token rejection")
	}
}

func TestHostAgentServiceRejectsWrongToken(t *testing.T) {
	token := "expected-agent-token"
	repo := newHostRepoStub(store.HostRecord{
		ID:            "host-a",
		Name:          "host-a",
		Status:        "installing",
		InstallState:  "running",
		AgentTokenRef: hostAgentTokenHashRef(token),
	})
	service := NewHostAgentService(repo)

	if _, err := service.Register(context.Background(), HostAgentRegisterRequest{HostID: "host-a"}, "wrong-token"); err == nil {
		t.Fatalf("Register() error = nil, want token rejection")
	}
	saved, err := repo.GetHost("host-a")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "installing" || saved.InstallState != "running" {
		t.Fatalf("host changed after rejected token: %+v", saved)
	}
}
