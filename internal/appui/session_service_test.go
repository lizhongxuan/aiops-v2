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
	if host.Snapshot.SelectedHostID != "" {
		t.Fatalf("host snapshot selectedHostId = %q, want empty until explicit host selection", host.Snapshot.SelectedHostID)
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

	activatedHost, err := services.SessionService().ActivateSession(context.Background(), host.ActiveSessionID)
	if err != nil {
		t.Fatalf("ActivateSession(host) error = %v", err)
	}
	if activatedHost.Snapshot.SelectedHostID != "" {
		t.Fatalf("activated host selectedHostId = %q, want empty until explicit host selection", activatedHost.Snapshot.SelectedHostID)
	}
}

func TestSessionService_IgnoresHostChildSessionsForListAndDefaultState(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	services := NewServices(runtimeStub{}, sessions)

	main := sessions.GetOrCreate("sess-main-hostops", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	main.HostID = "remote-linux-01"
	sessions.Update(main)
	child := sessions.GetOrCreate("host-child:hostops:turn-1:remote-linux-01", runtimekernel.SessionTypeHost, runtimekernel.ModeExecute)
	child.HostID = "remote-linux-01"
	sessions.Update(child)

	list, err := services.SessionService().ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if list.ActiveSessionID != "sess-main-hostops" {
		t.Fatalf("ActiveSessionID = %q, want main user session", list.ActiveSessionID)
	}
	if len(list.Sessions) != 1 || list.Sessions[0].ID != "sess-main-hostops" {
		t.Fatalf("Sessions = %+v, want only main user session", list.Sessions)
	}
	state, err := services.StateService().GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.SessionID != "sess-main-hostops" {
		t.Fatalf("state.SessionID = %q, want main user session", state.SessionID)
	}
}
