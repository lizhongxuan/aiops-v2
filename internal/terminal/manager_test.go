package terminal

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestManager_CreateListAndBridgeLifecycle(t *testing.T) {
	mgr := NewManager(WithCommandFactory(func(req CreateSessionRequest) (*exec.Cmd, error) {
		return exec.Command("/bin/cat"), nil
	}))

	meta, err := mgr.CreateSession(context.Background(), CreateSessionRequest{
		HostID: "host-a",
		Cwd:    "/tmp",
		Shell:  "/bin/cat",
		Cols:   120,
		Rows:   36,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if meta.SessionID == "" {
		t.Fatal("CreateSession() returned empty sessionId")
	}
	if meta.HostID != "host-a" {
		t.Fatalf("HostID = %q, want host-a", meta.HostID)
	}
	if meta.Shell != "/bin/cat" {
		t.Fatalf("Shell = %q, want /bin/cat", meta.Shell)
	}
	if meta.Status != SessionStatusRunning {
		t.Fatalf("Status = %q, want running", meta.Status)
	}

	list := mgr.ListSessions()
	if len(list) != 1 {
		t.Fatalf("ListSessions() len = %d, want 1", len(list))
	}
	if list[0].SessionID != meta.SessionID {
		t.Fatalf("ListSessions()[0].SessionID = %q, want %q", list[0].SessionID, meta.SessionID)
	}

	session := mgr.GetSession(meta.SessionID)
	if session == nil {
		t.Fatal("GetSession() returned nil")
	}

	events, release := session.Subscribe()
	defer release()

	select {
	case ready := <-events:
		if ready.Type != EventTypeReady {
			t.Fatalf("first event type = %q, want ready", ready.Type)
		}
		if ready.SessionID != meta.SessionID {
			t.Fatalf("ready.SessionID = %q, want %q", ready.SessionID, meta.SessionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ready event")
	}

	if err := session.SendInput("hello terminal\n"); err != nil {
		t.Fatalf("SendInput() error = %v", err)
	}

	var output Event
	select {
	case output = <-events:
		if output.Type != EventTypeOutput {
			t.Fatalf("output event type = %q, want output", output.Type)
		}
		if !strings.Contains(output.Data, "hello terminal") {
			t.Fatalf("output.Data = %q, want echoed input", output.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for output event")
	}

	session.Resize(200, 50)
	resized := mgr.GetSession(meta.SessionID)
	if resized == nil {
		t.Fatalf("resize metadata = %+v, want existing session", resized)
	}
	resizedMeta := resized.Metadata()
	if resizedMeta.Cols != 200 || resizedMeta.Rows != 50 {
		t.Fatalf("resize metadata = %+v, want cols=200 rows=50", resized)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case event := <-events:
		if event.Type != EventTypeExit {
			t.Fatalf("exit event type = %q, want exit", event.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for exit event")
	}

	if got := mgr.GetSession(meta.SessionID); got == nil || got.Metadata().Status != SessionStatusExited {
		t.Fatalf("final session status = %+v, want exited", got)
	}
}
