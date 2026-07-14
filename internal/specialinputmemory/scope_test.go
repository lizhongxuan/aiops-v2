package specialinputmemory

import "testing"

func TestScopeKeyNormalizesAndMatchesEnvironmentCluster(t *testing.T) {
	left := NewScopeKey("sess-1", "task-1", "", "Prod", "Orders")
	right := NewScopeKey("sess-1", "task-1", ScopeCurrentTask, "prod", "orders")
	if left.Scope != ScopeCurrentTask || left.EnvironmentKey != "prod" || left.ClusterKey != "orders" {
		t.Fatalf("scope not normalized: %#v", left)
	}
	if !left.Matches(right) {
		t.Fatalf("expected matching scope keys: %#v %#v", left, right)
	}
}

func TestScopeKeySeparatesTasksAndEnvironments(t *testing.T) {
	current := NewScopeKey("sess-1", "task-1", ScopeCurrentTask, "prod", "orders")
	if current.Matches(NewScopeKey("sess-1", "task-2", ScopeCurrentTask, "prod", "orders")) {
		t.Fatalf("current task scope should not match another task")
	}
	if current.Matches(NewScopeKey("sess-1", "task-1", ScopeCurrentTask, "test", "orders")) {
		t.Fatalf("prod scope should not match test scope")
	}
}
