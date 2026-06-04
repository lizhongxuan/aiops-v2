package appui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aiops-v2/internal/store"
	"runner/workflow/visual"
)

type fakeHostBootstrapRunner struct {
	run          HostInstallRun
	err          error
	graph        visual.Graph
	vars         map[string]any
	idempotency  string
	submitCalled bool
}

func (f *fakeHostBootstrapRunner) SubmitHostInstallGraph(_ context.Context, graph visual.Graph, vars map[string]any, idempotencyKey string) (HostInstallRun, error) {
	f.submitCalled = true
	f.graph = graph
	f.vars = vars
	f.idempotency = idempotencyKey
	if f.err != nil {
		return HostInstallRun{}, f.err
	}
	return f.run, nil
}

func (f *fakeHostBootstrapRunner) GetHostInstallRun(context.Context, string) (HostInstallRun, error) {
	return f.run, f.err
}

func TestHostBootstrapServiceSubmitsBuiltinWorkflowWithRedactedVars(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://ops/prod-web-01-ssh-key",
		AgentVersion:     "v0.1.0",
	})
	runner := &fakeHostBootstrapRunner{run: HostInstallRun{RunID: "run-1", WorkflowID: BuiltinHostAgentInstallWorkflowID, Status: "queued"}}
	service := NewHostBootstrapService(repo, runner)

	run, err := service.Install(context.Background(), "prod-web-01", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if run.RunID != "run-1" {
		t.Fatalf("RunID = %q", run.RunID)
	}
	if !runner.submitCalled {
		t.Fatal("runner was not called")
	}
	if got := runner.vars["ssh_credential_ref"]; got != "secret://ops/prod-web-01-ssh-key" {
		t.Fatalf("ssh_credential_ref = %v", got)
	}
	for key, value := range runner.vars {
		text := strings.ToLower(key + "=" + fmt.Sprint(value))
		if strings.Contains(strings.ToLower(key), "password") {
			t.Fatalf("vars leaked password key: %s=%v", key, value)
		}
		if strings.Contains(text, "begin openssh private key") || strings.Contains(text, "password=") {
			t.Fatalf("vars leaked private material: %s=%v", key, value)
		}
	}
	if runner.idempotency != "host-agent-install:prod-web-01:v0.1.0" {
		t.Fatalf("idempotency = %q", runner.idempotency)
	}
	saved, err := repo.GetHost("prod-web-01")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.InstallRunID != "run-1" || saved.InstallWorkflowID != BuiltinHostAgentInstallWorkflowID || saved.InstallState != "running" {
		t.Fatalf("saved host = %+v", saved)
	}
}

func TestHostBootstrapServiceSubmitsWorkflowWithoutSSHCredentialRef(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:           "prod-web-01",
		Address:      "10.0.0.11",
		SSHUser:      "ubuntu",
		SSHPort:      22,
		AgentVersion: "v0.1.0",
	})
	runner := &fakeHostBootstrapRunner{run: HostInstallRun{RunID: "run-1", WorkflowID: BuiltinHostAgentInstallWorkflowID, Status: "queued"}}
	service := NewHostBootstrapService(repo, runner)

	run, err := service.Install(context.Background(), "prod-web-01", HostInstallRequest{AgentVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if run.RunID != "run-1" {
		t.Fatalf("RunID = %q", run.RunID)
	}
	if got := runner.vars["ssh_credential_ref"]; got != "" {
		t.Fatalf("ssh_credential_ref = %v, want empty", got)
	}
}

func TestHostBootstrapServiceMapsSubmitFailureToInstallFailed(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://ops/prod-web-01-ssh-key",
		AgentVersion:     "v0.1.0",
	})
	runner := &fakeHostBootstrapRunner{err: errors.New("runner unavailable")}
	service := NewHostBootstrapService(repo, runner)

	if _, err := service.Install(context.Background(), "prod-web-01", HostInstallRequest{AgentVersion: "v0.1.0"}); err == nil {
		t.Fatal("Install() error = nil, want runner failure")
	}
	saved, err := repo.GetHost("prod-web-01")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "install_failed" || saved.InstallState != "failed" || !strings.Contains(saved.LastError, "runner unavailable") {
		t.Fatalf("saved host = %+v", saved)
	}
}

func TestHostBootstrapServiceMapsDetectPlatformFailureToUnsupportedPlatform(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:                "prod-web-01",
		Status:            "installing",
		InstallState:      "running",
		InstallRunID:      "run-1",
		InstallWorkflowID: BuiltinHostAgentInstallWorkflowID,
	})
	runner := &fakeHostBootstrapRunner{run: HostInstallRun{
		RunID:        "run-1",
		WorkflowID:   BuiltinHostAgentInstallWorkflowID,
		Status:       "failed",
		CurrentStep:  "detect-platform",
		LastError:    "script.shell failed: exit status 65",
		AgentVersion: "v0.1.0",
	}}
	service := NewHostBootstrapService(repo, runner)

	service.updateHostInstallProgress("prod-web-01", runner.run)

	saved, err := repo.GetHost("prod-web-01")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if saved.Status != "install_failed" || saved.InstallState != "unsupported_platform" || saved.InstallStep != "detect-platform" {
		t.Fatalf("saved host = %+v", saved)
	}
	if strings.TrimSpace(saved.LastError) == "" {
		t.Fatalf("LastError is empty")
	}
}

func TestHostBootstrapServiceUsesStableIdempotencyKey(t *testing.T) {
	repo := newHostRepoStub(store.HostRecord{
		ID:               "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://ops/prod-web-01-ssh-key",
	})
	runner := &fakeHostBootstrapRunner{run: HostInstallRun{RunID: "run-1", WorkflowID: BuiltinHostAgentInstallWorkflowID, Status: "queued"}}
	service := NewHostBootstrapService(repo, runner)

	if _, err := service.Install(context.Background(), "prod-web-01", HostInstallRequest{AgentVersion: "v9.9.9"}); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if runner.idempotency != "host-agent-install:prod-web-01:v9.9.9" {
		t.Fatalf("idempotency = %q", runner.idempotency)
	}
}
