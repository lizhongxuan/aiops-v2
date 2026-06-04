package agentmgr

import (
	"context"
	"testing"

	"aiops-v2/internal/hostops"
)

func TestKernelAdapterSpawnHostChildRegistersBoundWorker(t *testing.T) {
	factory, registry := newTestFactory(t)
	registerTestTools(t, registry)
	manager := NewAgentManager(factory, &testRunner{}, nil)
	adapter := NewKernelAdapter(manager, factory)

	child, err := adapter.SpawnHostChild(context.Background(), hostops.SpawnHostChildRequest{
		ChildAgentID:  "child-1",
		MissionID:     "mission-1",
		ParentAgentID: "manager-1",
		SessionID:     "session-child-1",
		HostID:        "host-a",
		Task:          "prepare pg primary",
	})
	if err != nil {
		t.Fatalf("SpawnHostChild() error = %v", err)
	}
	if child.ID != "child-1" || child.HostID != "host-a" || child.MissionID != "mission-1" {
		t.Fatalf("child = %+v, want registered host-a child", child)
	}
	inst := manager.GetInstance("child-1")
	if inst == nil {
		t.Fatal("child instance was not registered")
	}
	if inst.Kind != AgentKindWorker || inst.HostID != "host-a" || inst.ParentID != "manager-1" {
		t.Fatalf("instance = %+v, want host-bound worker child", inst)
	}
}
