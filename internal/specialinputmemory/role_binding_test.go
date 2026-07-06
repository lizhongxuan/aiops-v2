package specialinputmemory

import "testing"

func TestRoleBindingHashIncludesEnvironmentClusterRoleAndRuntimeName(t *testing.T) {
	left := NewMentionRoleBinding(RoleBindingInput{
		ResourceKind:   ResourceKindHost,
		ResourceID:     "host-c",
		RoleKey:        "monitor",
		RuntimeName:    "pg_mon",
		EnvironmentKey: "prod",
		ClusterKey:     "orders",
		SourceTurnID:   "turn-1",
	})
	right := NewMentionRoleBinding(RoleBindingInput{
		ResourceKind:   ResourceKindHost,
		ResourceID:     "host-c",
		RoleKey:        "monitor",
		RuntimeName:    "node_exporter",
		EnvironmentKey: "prod",
		ClusterKey:     "orders",
		SourceTurnID:   "turn-1",
	})
	if left.BindingHash == "" {
		t.Fatalf("missing binding hash")
	}
	if left.BindingHash == right.BindingHash {
		t.Fatalf("runtimeName should affect hash: %q", left.BindingHash)
	}
}

func TestDetectRoleBindingConflictsIsolatedByEnvironmentAndCluster(t *testing.T) {
	bindings := []MentionRoleBinding{
		NewMentionRoleBinding(RoleBindingInput{ResourceKind: ResourceKindHost, ResourceID: "host-a", RoleKey: "pg_primary", EnvironmentKey: "prod", ClusterKey: "orders"}),
		NewMentionRoleBinding(RoleBindingInput{ResourceKind: ResourceKindHost, ResourceID: "host-d", RoleKey: "pg_primary", EnvironmentKey: "test", ClusterKey: "orders"}),
	}
	if conflicts := DetectRoleBindingConflicts(bindings); len(conflicts) != 0 {
		t.Fatalf("prod/test role bindings should not conflict: %#v", conflicts)
	}

	bindings = append(bindings, NewMentionRoleBinding(RoleBindingInput{ResourceKind: ResourceKindHost, ResourceID: "host-x", RoleKey: "pg_primary", EnvironmentKey: "prod", ClusterKey: "orders"}))
	conflicts := DetectRoleBindingConflicts(bindings)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts len = %d, want 1: %#v", len(conflicts), conflicts)
	}
	if conflicts[0].EnvironmentKey != "prod" || conflicts[0].ClusterKey != "orders" || conflicts[0].RoleKey != "pg_primary" {
		t.Fatalf("conflict lost scope: %#v", conflicts[0])
	}
}
