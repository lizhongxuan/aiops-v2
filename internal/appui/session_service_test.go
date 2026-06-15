package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/runtimekernel"
)

func TestSessionService_CreateSessionReturnsActiveListAndSnapshot(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	services := NewServices(runtimeStub{}, sessions)

	workspace, err := services.SessionService().CreateSession(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CreateSession(workspace) error = %v", err)
	}
	if workspace.ActiveSessionID == "" {
		t.Fatal("CreateSession(workspace) returned empty activeSessionId")
	}
	if workspace.Snapshot.Kind != "workspace" {
		t.Fatalf("workspace snapshot kind = %q, want workspace", workspace.Snapshot.Kind)
	}
	if len(workspace.Sessions) != 1 || workspace.Sessions[0].Kind != "workspace" {
		t.Fatalf("workspace sessions = %+v, want one workspace summary", workspace.Sessions)
	}

	host, err := services.SessionService().CreateSession(context.Background(), "single_host")
	if err != nil {
		t.Fatalf("CreateSession(single_host) error = %v", err)
	}
	if host.Snapshot.Kind != "single_host" {
		t.Fatalf("host snapshot kind = %q, want single_host", host.Snapshot.Kind)
	}
	if host.Snapshot.SelectedHostID != "server-local" {
		t.Fatalf("host snapshot selectedHostId = %q, want server-local", host.Snapshot.SelectedHostID)
	}
	if host.Snapshot.CurrentMode != "execute" || host.Snapshot.CurrentLane != "execute" {
		t.Fatalf("host snapshot mode/lane = %q/%q, want execute/execute", host.Snapshot.CurrentMode, host.Snapshot.CurrentLane)
	}
	if len(host.Sessions) != 2 {
		t.Fatalf("len(host sessions) = %d, want 2", len(host.Sessions))
	}
}

func TestSessionService_CreateSingleHostSessionBindsRequestedHost(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	services := NewServices(runtimeStub{}, sessions)

	host, err := services.SessionService().CreateSession(context.Background(), "single_host", "remote-linux-01")
	if err != nil {
		t.Fatalf("CreateSession(single_host, remote-linux-01) error = %v", err)
	}

	if host.Snapshot.SelectedHostID != "remote-linux-01" {
		t.Fatalf("snapshot selectedHostId = %q, want remote-linux-01", host.Snapshot.SelectedHostID)
	}
	if len(host.Sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(host.Sessions))
	}
	if host.Sessions[0].SelectedHostID != "remote-linux-01" {
		t.Fatalf("session selectedHostId = %q, want remote-linux-01", host.Sessions[0].SelectedHostID)
	}
}

func TestSessionService_ActivateSessionPromotesExistingSession(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	services := NewServices(runtimeStub{}, sessions)

	workspace, err := services.SessionService().CreateSession(context.Background(), "workspace")
	if err != nil {
		t.Fatalf("CreateSession(workspace) error = %v", err)
	}
	host, err := services.SessionService().CreateSession(context.Background(), "single_host")
	if err != nil {
		t.Fatalf("CreateSession(single_host) error = %v", err)
	}

	activated, err := services.SessionService().ActivateSession(context.Background(), workspace.ActiveSessionID)
	if err != nil {
		t.Fatalf("ActivateSession(workspace) error = %v", err)
	}
	if activated.ActiveSessionID != workspace.ActiveSessionID {
		t.Fatalf("activeSessionId = %q, want %q", activated.ActiveSessionID, workspace.ActiveSessionID)
	}
	if activated.Snapshot.SessionID != workspace.ActiveSessionID {
		t.Fatalf("snapshot.sessionId = %q, want %q", activated.Snapshot.SessionID, workspace.ActiveSessionID)
	}
	if len(activated.Sessions) < 2 || activated.Sessions[0].ID != workspace.ActiveSessionID {
		t.Fatalf("sessions ordering = %+v, want workspace first after activation", activated.Sessions)
	}
	if host.ActiveSessionID == activated.ActiveSessionID {
		t.Fatalf("expected activation to move away from host session %q", host.ActiveSessionID)
	}
}
