package appui

import (
	"context"
	"strings"
	"testing"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
)

func TestHostOpsRouteMetadataOnlyRunsRuntime(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	runtime := &chatRuntimeCapture{}
	hosts := newHostRepoStub(
		store.HostRecord{ID: "guard-host-a", Name: "guard-a", Address: "10.20.1.11", Status: "online", Executable: true, AgentURL: "http://guard-a:7072"},
		store.HostRecord{ID: "guard-host-b", Name: "guard-b", Address: "10.20.1.12", Status: "online", Executable: true, AgentURL: "http://guard-b:7072"},
	)
	hostOps := &chatHostOpsServiceCapture{}
	services := NewServices(runtime, sessions, WithHostRepository(hosts), WithHostOpsService(hostOps))

	result, err := services.ChatService().SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-hostops-guard",
		Content:   "@guard-a @guard-b 对比 Redis 延迟，走多主机 manager。",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "accepted" {
		t.Fatalf("Status = %q, want accepted", result.Status)
	}
	runReq := waitForRunTurn(t, runtime)
	if hostOps.created {
		t.Fatalf("HostOpsService.CreateMission was called from appui legacy route: %+v", hostOps.command)
	}
	if runReq.Metadata["aiops.hostops.routeKind"] != string(hostops.RouteKindHostOps) {
		t.Fatalf("hostops route metadata missing: %#v", runReq.Metadata)
	}
	if runReq.Metadata["enableToolPack"] == "" || !strings.Contains(runReq.Metadata["enableToolPack"], hostops.ToolPackHostOps) {
		t.Fatalf("enableToolPack metadata = %q, want hostops pack", runReq.Metadata["enableToolPack"])
	}
	if session := sessions.Get(result.SessionID); session != nil && session.CurrentTurn != nil && session.CurrentTurn.Lifecycle == runtimekernel.TurnLifecycleCompleted {
		t.Fatalf("appui wrote completed hostops turn: %+v", session.CurrentTurn)
	}
}
