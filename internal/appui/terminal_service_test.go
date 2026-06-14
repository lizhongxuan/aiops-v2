package appui

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"aiops-v2/internal/store"
	"aiops-v2/internal/terminal"
)

func TestTerminalService_CreateAndListSessions(t *testing.T) {
	mgr := terminal.NewManager(terminal.WithCommandFactory(func(req terminal.CreateSessionRequest) (*exec.Cmd, error) {
		return exec.Command("/bin/cat"), nil
	}))
	svc := NewTerminalService(mgr, nil)

	created, err := svc.CreateSession(context.Background(), TerminalCreateSessionCommand{
		HostID: "host-a",
		Cwd:    "/tmp",
		Shell:  "/bin/cat",
		Cols:   120,
		Rows:   36,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.SessionID == "" {
		t.Fatal("CreateSession() returned empty sessionId")
	}
	if created.HostID != "host-a" {
		t.Fatalf("HostID = %q, want host-a", created.HostID)
	}

	list, err := svc.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(list.Sessions) != 1 {
		t.Fatalf("len(Sessions) = %d, want 1", len(list.Sessions))
	}
	if list.Sessions[0].SessionID != created.SessionID {
		t.Fatalf("Sessions[0].SessionID = %q, want %q", list.Sessions[0].SessionID, created.SessionID)
	}
	if list.Sessions[0].Status != terminal.SessionStatusRunning {
		t.Fatalf("Sessions[0].Status = %q, want running", list.Sessions[0].Status)
	}
}

func TestTerminalService_EnforcesHostTerminalPermission(t *testing.T) {
	mgr := terminal.NewManager(terminal.WithCommandFactory(func(req terminal.CreateSessionRequest) (*exec.Cmd, error) {
		return exec.Command("/bin/cat"), nil
	}))
	repo := newHostRepoStub(
		store.HostRecord{
			ID:              "terminal-only",
			Status:          "online",
			TerminalCapable: true,
		},
		store.HostRecord{
			ID:         "exec-only",
			Status:     "online",
			Executable: true,
		},
		store.HostRecord{
			ID:              "offline",
			Status:          "offline",
			TerminalCapable: true,
			Executable:      true,
		},
		store.HostRecord{
			ID:     "readonly",
			Status: "online",
		},
	)
	svc := NewTerminalService(mgr, repo)

	for _, hostID := range []string{"terminal-only", "exec-only", "server-local"} {
		t.Run("allows "+hostID, func(t *testing.T) {
			created, err := svc.CreateSession(context.Background(), TerminalCreateSessionCommand{HostID: hostID})
			if err != nil {
				t.Fatalf("CreateSession(%q) error = %v", hostID, err)
			}
			if created.HostID != hostID {
				t.Fatalf("created.HostID = %q, want %q", created.HostID, hostID)
			}
		})
	}

	for _, tc := range []struct {
		hostID string
		want   string
	}{
		{hostID: "offline", want: "offline"},
		{hostID: "readonly", want: "terminal is not enabled"},
		{hostID: "missing", want: "host not found"},
	} {
		t.Run("rejects "+tc.hostID, func(t *testing.T) {
			_, err := svc.CreateSession(context.Background(), TerminalCreateSessionCommand{HostID: tc.hostID})
			if err == nil {
				t.Fatalf("CreateSession(%q) error = nil, want failure", tc.hostID)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CreateSession(%q) error = %q, want substring %q", tc.hostID, err.Error(), tc.want)
			}
		})
	}
}
