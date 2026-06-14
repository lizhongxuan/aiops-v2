package terminal

import (
	"context"
	"testing"
	"time"
)

type fakeRemoteBackend struct {
	openReq RemoteTerminalOpenRequest
	handle  *fakeRemoteHandle
}

func (b *fakeRemoteBackend) OpenTerminal(_ context.Context, req RemoteTerminalOpenRequest) (RemoteTerminalHandle, error) {
	b.openReq = req
	b.handle = &fakeRemoteHandle{}
	return b.handle, nil
}

type fakeRemoteHandle struct {
	inputs  []string
	resizes [][2]int
	signals []string
	closed  bool
}

func (h *fakeRemoteHandle) SendInput(data string) error {
	h.inputs = append(h.inputs, data)
	return nil
}

func (h *fakeRemoteHandle) Resize(cols, rows int) error {
	h.resizes = append(h.resizes, [2]int{cols, rows})
	return nil
}

func (h *fakeRemoteHandle) Signal(signal string) error {
	h.signals = append(h.signals, signal)
	return nil
}

func (h *fakeRemoteHandle) Close() error {
	h.closed = true
	return nil
}

func TestManager_RemoteGRPCTerminalSessionBridgesEventsAndControls(t *testing.T) {
	backend := &fakeRemoteBackend{}
	mgr := NewManager(WithRemoteBackend(backend))

	meta, err := mgr.CreateSession(context.Background(), CreateSessionRequest{
		HostID: "host-grpc",
		Cwd:    "/srv",
		Shell:  "/bin/bash",
		Cols:   100,
		Rows:   40,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if backend.openReq.SessionID != meta.SessionID || backend.openReq.HostID != "host-grpc" {
		t.Fatalf("open request = %+v, want session %s on host-grpc", backend.openReq, meta.SessionID)
	}
	if meta.Status != SessionStatusRunning || meta.PID != 0 {
		t.Fatalf("metadata = %+v, want running remote metadata without local pid", meta)
	}

	session := mgr.GetSession(meta.SessionID)
	if session == nil {
		t.Fatal("GetSession() returned nil")
	}
	events, release := session.Subscribe()
	defer release()
	<-events // ready

	backend.openReq.Emit(Event{Type: EventTypeOutput, Data: "remote output\n"})
	select {
	case event := <-events:
		if event.Type != EventTypeOutput || event.Data != "remote output\n" || event.SessionID != meta.SessionID || event.HostID != "host-grpc" {
			t.Fatalf("output event = %+v, want normalized remote output", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for remote output event")
	}

	if err := session.SendInput("uptime\n"); err != nil {
		t.Fatalf("SendInput() error = %v", err)
	}
	session.Resize(120, 48)
	if err := session.Signal("SIGINT"); err != nil {
		t.Fatalf("Signal() error = %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if got := backend.handle.inputs; len(got) != 1 || got[0] != "uptime\n" {
		t.Fatalf("inputs = %#v, want uptime input", got)
	}
	if got := backend.handle.resizes; len(got) != 1 || got[0] != [2]int{120, 48} {
		t.Fatalf("resizes = %#v, want 120x48", got)
	}
	if got := backend.handle.signals; len(got) != 1 || got[0] != "SIGINT" {
		t.Fatalf("signals = %#v, want SIGINT", got)
	}
	if !backend.handle.closed {
		t.Fatal("remote handle was not closed")
	}
	if got := mgr.GetSession(meta.SessionID).Metadata().Status; got != SessionStatusExited {
		t.Fatalf("status = %q, want exited after close", got)
	}
}
