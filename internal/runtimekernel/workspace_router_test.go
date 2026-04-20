package runtimekernel

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// Mock ProjectionReader for testing
// ---------------------------------------------------------------------------

type mockProjectionReader struct {
	state string
	err   error
}

func (m *mockProjectionReader) ReadState(_ string) (string, error) {
	return m.state, m.err
}

// ---------------------------------------------------------------------------
// Unit Tests: WorkspaceRouter.ClassifyRequest
// ---------------------------------------------------------------------------

func TestClassifyRequest_StateQuery(t *testing.T) {
	router := NewWorkspaceRouter(&mockProjectionReader{state: "ok"})

	tests := []struct {
		name  string
		input string
	}{
		{"chinese status", "当前服务器状态"},
		{"english status", "show me the current status"},
		{"list hosts", "列出所有在线主机"},
		{"how many running", "how many tasks are running"},
		{"what are online", "what are the online hosts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TurnRequest{
				SessionType: SessionTypeWorkspace,
				Mode:        ModeInspect,
				Input:       tt.input,
			}
			decision := router.ClassifyRequest(req)
			if decision.Category != CategoryStateQuery {
				t.Errorf("expected CategoryStateQuery, got %q (reason: %s)", decision.Category, decision.Reason)
			}
		})
	}
}

func TestClassifyRequest_StateQueryWithAction_NotStateQuery(t *testing.T) {
	router := NewWorkspaceRouter(&mockProjectionReader{state: "ok"})

	tests := []struct {
		name  string
		input string
	}{
		{"status then execute", "查看状态然后执行清理"},
		{"show and restart", "show status and restart the service"},
		{"list and delete", "list files and delete old ones"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TurnRequest{
				SessionType: SessionTypeWorkspace,
				Mode:        ModeExecute,
				Input:       tt.input,
			}
			decision := router.ClassifyRequest(req)
			if decision.Category == CategoryStateQuery {
				t.Errorf("should NOT be CategoryStateQuery when action keywords present")
			}
		})
	}
}

func TestClassifyRequest_SingleHostReadonly(t *testing.T) {
	router := NewWorkspaceRouter(&mockProjectionReader{state: "ok"})

	tests := []struct {
		name   string
		input  string
		hostID string
		mode   Mode
	}{
		{"check logs", "check the logs on this host", "host-1", ModeInspect},
		{"read file", "read /var/log/syslog", "host-2", ModeChat},
		{"inspect disk", "inspect disk usage", "host-3", ModeInspect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TurnRequest{
				SessionType: SessionTypeWorkspace,
				Mode:        tt.mode,
				Input:       tt.input,
				HostID:      tt.hostID,
			}
			decision := router.ClassifyRequest(req)
			if decision.Category != CategorySingleHostReadonly {
				t.Errorf("expected CategorySingleHostReadonly, got %q (reason: %s)", decision.Category, decision.Reason)
			}
			if len(decision.TargetHosts) != 1 || decision.TargetHosts[0] != tt.hostID {
				t.Errorf("expected target host %q, got %v", tt.hostID, decision.TargetHosts)
			}
		})
	}
}

func TestClassifyRequest_ComplexTask(t *testing.T) {
	router := NewWorkspaceRouter(&mockProjectionReader{state: "ok"})

	tests := []struct {
		name   string
		input  string
		hostID string
		mode   Mode
	}{
		{"multi-host cleanup", "检查所有服务器磁盘并清理", "", ModeExecute},
		{"deploy all", "deploy the new version to all servers", "", ModeExecute},
		{"no host specified execute", "restart the nginx service", "", ModeExecute},
		{"mutation on host in execute mode", "restart nginx", "host-1", ModeExecute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := TurnRequest{
				SessionType: SessionTypeWorkspace,
				Mode:        tt.mode,
				Input:       tt.input,
				HostID:      tt.hostID,
			}
			decision := router.ClassifyRequest(req)
			if decision.Category != CategoryComplexTask {
				t.Errorf("expected CategoryComplexTask, got %q (reason: %s)", decision.Category, decision.Reason)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unit Tests: WorkspaceRouter.RouteRequest
// ---------------------------------------------------------------------------

func TestRouteRequest_StateQuery(t *testing.T) {
	projector := &mockProjectionReader{state: "all systems operational"}
	router := NewWorkspaceRouter(projector)
	kernel := newTestKernel(nil)

	req := TurnRequest{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeInspect,
		Input:       "当前状态",
		SessionID:   "sess-1",
		TurnID:      "turn-1",
	}

	result, err := router.RouteRequest(context.Background(), req, kernel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", result.Status)
	}
	if result.Output != "all systems operational" {
		t.Errorf("expected output from projection, got %q", result.Output)
	}
}

func TestRouteRequest_SingleHostReadonly(t *testing.T) {
	projector := &mockProjectionReader{state: "ok"}
	router := NewWorkspaceRouter(projector)
	kernel := newTestKernel(nil)

	req := TurnRequest{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeInspect,
		Input:       "check the logs",
		HostID:      "host-1",
	}

	result, err := router.RouteRequest(context.Background(), req, kernel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single-host readonly delegates to RunTurn, which completes normally
	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", result.Status)
	}
}

func TestRouteRequest_ComplexTask(t *testing.T) {
	projector := &mockProjectionReader{state: "ok"}
	router := NewWorkspaceRouter(projector)
	kernel := newTestKernel(nil)

	req := TurnRequest{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		Input:       "检查所有服务器磁盘并清理",
		SessionID:   "sess-ws",
		TurnID:      "turn-ws",
	}

	result, err := router.RouteRequest(context.Background(), req, kernel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "running" {
		t.Errorf("expected status 'running', got %q", result.Status)
	}
}

// ---------------------------------------------------------------------------
// Unit Tests: RequestCategory validation
// ---------------------------------------------------------------------------

func TestRequestCategory_IsValid(t *testing.T) {
	for _, cat := range AllRequestCategories() {
		if !cat.IsValid() {
			t.Errorf("expected %q to be valid", cat)
		}
	}

	invalid := RequestCategory("unknown")
	if invalid.IsValid() {
		t.Error("expected 'unknown' to be invalid")
	}
}

// ---------------------------------------------------------------------------
// Unit Tests: Classification helpers
// ---------------------------------------------------------------------------

func TestIsStateQuery(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"当前状态", true},
		{"show status", true},
		{"list running tasks", true},
		{"execute cleanup", false},
		{"restart service", false},
		{"hello world", false},
	}

	for _, tt := range tests {
		got := isStateQuery(tt.input)
		if got != tt.expected {
			t.Errorf("isStateQuery(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestIsReadonlyIntent(t *testing.T) {
	tests := []struct {
		input    string
		mode     Mode
		expected bool
	}{
		{"anything", ModeChat, true},
		{"anything", ModeInspect, true},
		{"check logs", ModeExecute, true},
		{"read file", ModePlan, true},
		{"do something", ModeExecute, false},
		{"do something", ModePlan, false},
	}

	for _, tt := range tests {
		got := isReadonlyIntent(tt.input, tt.mode)
		if got != tt.expected {
			t.Errorf("isReadonlyIntent(%q, %q) = %v, want %v", tt.input, tt.mode, got, tt.expected)
		}
	}
}

func TestClassifyTaskType(t *testing.T) {
	tests := []struct {
		name     string
		decision RoutingDecision
		expected string
	}{
		{"multi host", RoutingDecision{TargetHosts: []string{"a", "b"}}, "multi_host"},
		{"single host", RoutingDecision{TargetHosts: []string{"a"}}, "host_exec"},
		{"no host", RoutingDecision{TargetHosts: nil}, "plan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTaskType(tt.decision)
			if got != tt.expected {
				t.Errorf("classifyTaskType() = %q, want %q", got, tt.expected)
			}
		})
	}
}
