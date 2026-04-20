package runtimekernel

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionType_IsValid(t *testing.T) {
	tests := []struct {
		input SessionType
		want  bool
	}{
		{SessionTypeHost, true},
		{SessionTypeWorkspace, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.input.IsValid(); got != tt.want {
			t.Errorf("SessionType(%q).IsValid() = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAllSessionTypes(t *testing.T) {
	types := AllSessionTypes()
	if len(types) != 2 {
		t.Fatalf("AllSessionTypes() returned %d items, want 2", len(types))
	}
	for _, st := range types {
		if !st.IsValid() {
			t.Errorf("AllSessionTypes() contains invalid type %q", st)
		}
	}
}

func TestMode_IsValid(t *testing.T) {
	tests := []struct {
		input Mode
		want  bool
	}{
		{ModeChat, true},
		{ModeInspect, true},
		{ModePlan, true},
		{ModeExecute, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.input.IsValid(); got != tt.want {
			t.Errorf("Mode(%q).IsValid() = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestAllModes(t *testing.T) {
	modes := AllModes()
	if len(modes) != 4 {
		t.Fatalf("AllModes() returned %d items, want 4", len(modes))
	}
	for _, m := range modes {
		if !m.IsValid() {
			t.Errorf("AllModes() contains invalid mode %q", m)
		}
	}
}

func TestTurnRequest_Validate(t *testing.T) {
	valid := TurnRequest{SessionType: SessionTypeHost, Mode: ModeChat}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid TurnRequest.Validate() = %v", err)
	}

	invalidSession := TurnRequest{SessionType: "bad", Mode: ModeChat}
	if err := invalidSession.Validate(); err == nil {
		t.Error("TurnRequest with invalid session type should fail validation")
	}

	invalidMode := TurnRequest{SessionType: SessionTypeHost, Mode: "bad"}
	if err := invalidMode.Validate(); err == nil {
		t.Error("TurnRequest with invalid mode should fail validation")
	}
}

func TestTurnResult_Validate(t *testing.T) {
	valid := TurnResult{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		SessionID:   "sess-1",
		TurnID:      "turn-1",
		Status:      "completed",
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid TurnResult.Validate() = %v", err)
	}

	noSession := TurnResult{SessionType: SessionTypeHost, Mode: ModeChat, TurnID: "t1"}
	if err := noSession.Validate(); err == nil {
		t.Error("TurnResult without session id should fail validation")
	}

	noTurn := TurnResult{SessionType: SessionTypeHost, Mode: ModeChat, SessionID: "s1"}
	if err := noTurn.Validate(); err == nil {
		t.Error("TurnResult without turn id should fail validation")
	}
}

func TestResumeRequest_Validate(t *testing.T) {
	valid := ResumeRequest{SessionID: "s1", TurnID: "t1"}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid ResumeRequest.Validate() = %v", err)
	}

	if err := (ResumeRequest{TurnID: "t1"}).Validate(); err == nil {
		t.Error("ResumeRequest without session id should fail")
	}
	if err := (ResumeRequest{SessionID: "s1"}).Validate(); err == nil {
		t.Error("ResumeRequest without turn id should fail")
	}
}

func TestCancelRequest_Validate(t *testing.T) {
	valid := CancelRequest{SessionID: "s1", TurnID: "t1"}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid CancelRequest.Validate() = %v", err)
	}

	if err := (CancelRequest{TurnID: "t1"}).Validate(); err == nil {
		t.Error("CancelRequest without session id should fail")
	}
	if err := (CancelRequest{SessionID: "s1"}).Validate(); err == nil {
		t.Error("CancelRequest without turn id should fail")
	}
}

func TestEventType_IsValid(t *testing.T) {
	for _, et := range AllEventTypes() {
		if !et.IsValid() {
			t.Errorf("AllEventTypes() contains invalid type %q", et)
		}
	}
	if EventType("unknown").IsValid() {
		t.Error("unknown EventType should not be valid")
	}
}

func TestAllEventTypes(t *testing.T) {
	types := AllEventTypes()
	if len(types) != 10 {
		t.Fatalf("AllEventTypes() returned %d items, want 10", len(types))
	}
}

func TestLifecycleEvent_Validate(t *testing.T) {
	valid := LifecycleEvent{
		Type:      EventToolStarted,
		SessionID: "s1",
		TurnID:    "t1",
		Timestamp: time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid LifecycleEvent.Validate() = %v", err)
	}

	badType := valid
	badType.Type = "bad"
	if err := badType.Validate(); err == nil {
		t.Error("LifecycleEvent with invalid type should fail")
	}

	noSession := valid
	noSession.SessionID = ""
	if err := noSession.Validate(); err == nil {
		t.Error("LifecycleEvent without session id should fail")
	}

	noTurn := valid
	noTurn.TurnID = ""
	if err := noTurn.Validate(); err == nil {
		t.Error("LifecycleEvent without turn id should fail")
	}

	noTime := valid
	noTime.Timestamp = time.Time{}
	if err := noTime.Validate(); err == nil {
		t.Error("LifecycleEvent with zero timestamp should fail")
	}
}

func TestLifecycleEvent_JSONRoundTrip(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"tool": "disk_usage"})
	original := LifecycleEvent{
		Type:      EventToolCompleted,
		SessionID: "sess-abc",
		TurnID:    "turn-123",
		Timestamp: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
		Payload:   payload,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded LifecycleEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, original.Type)
	}
	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID mismatch: got %q, want %q", decoded.SessionID, original.SessionID)
	}
	if decoded.TurnID != original.TurnID {
		t.Errorf("TurnID mismatch: got %q, want %q", decoded.TurnID, original.TurnID)
	}
	if !decoded.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp mismatch: got %v, want %v", decoded.Timestamp, original.Timestamp)
	}
}

func TestRuntimeContext_Validate(t *testing.T) {
	valid := RuntimeContext{SessionType: SessionTypeHost, Mode: ModeInspect}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid RuntimeContext.Validate() = %v", err)
	}

	badSession := RuntimeContext{SessionType: "x", Mode: ModeChat}
	if err := badSession.Validate(); err == nil {
		t.Error("RuntimeContext with invalid session type should fail")
	}

	badMode := RuntimeContext{SessionType: SessionTypeWorkspace, Mode: "x"}
	if err := badMode.Validate(); err == nil {
		t.Error("RuntimeContext with invalid mode should fail")
	}
}


func TestSessionState_Validate(t *testing.T) {
	valid := SessionState{
		ID:        "sess-1",
		Type:      SessionTypeHost,
		Mode:      ModeChat,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid SessionState.Validate() = %v", err)
	}

	noID := SessionState{Type: SessionTypeHost, Mode: ModeChat}
	if err := noID.Validate(); err == nil {
		t.Error("SessionState without id should fail validation")
	}

	badType := SessionState{ID: "s1", Type: "bad", Mode: ModeChat}
	if err := badType.Validate(); err == nil {
		t.Error("SessionState with invalid type should fail validation")
	}

	badMode := SessionState{ID: "s1", Type: SessionTypeWorkspace, Mode: "bad"}
	if err := badMode.Validate(); err == nil {
		t.Error("SessionState with invalid mode should fail validation")
	}
}

func TestSessionState_JSONRoundTrip(t *testing.T) {
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	original := SessionState{
		ID:     "sess-abc",
		Type:   SessionTypeWorkspace,
		Mode:   ModeExecute,
		HostID: "host-1",
		Messages: []Message{
			{ID: "msg-1", Role: "user", Content: "hello", Timestamp: now},
		},
		Context: ContextWindow{
			MaxTokens:  4096,
			UsedTokens: 100,
			Messages:   1,
		},
		Activity: ActivityStats{
			SearchCount:  2,
			CommandCount: 1,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded SessionState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, original.Type)
	}
	if decoded.Mode != original.Mode {
		t.Errorf("Mode mismatch: got %q, want %q", decoded.Mode, original.Mode)
	}
	if decoded.HostID != original.HostID {
		t.Errorf("HostID mismatch: got %q, want %q", decoded.HostID, original.HostID)
	}
	if len(decoded.Messages) != 1 {
		t.Fatalf("Messages length mismatch: got %d, want 1", len(decoded.Messages))
	}
	if decoded.Messages[0].Content != "hello" {
		t.Errorf("Message content mismatch: got %q, want %q", decoded.Messages[0].Content, "hello")
	}
	if decoded.Context.MaxTokens != 4096 {
		t.Errorf("MaxTokens mismatch: got %d, want 4096", decoded.Context.MaxTokens)
	}
	if decoded.Activity.SearchCount != 2 {
		t.Errorf("SearchCount mismatch: got %d, want 2", decoded.Activity.SearchCount)
	}
}

func TestMessage_JSONRoundTrip(t *testing.T) {
	args := json.RawMessage(`{"path":"/tmp"}`)
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	original := Message{
		ID:   "msg-1",
		Role: "assistant",
		ToolCalls: []ToolCall{
			{ID: "tc-1", Name: "disk_usage", Arguments: args},
		},
		Timestamp: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Role != original.Role {
		t.Errorf("Role mismatch: got %q, want %q", decoded.Role, original.Role)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length mismatch: got %d, want 1", len(decoded.ToolCalls))
	}
	if decoded.ToolCalls[0].Name != "disk_usage" {
		t.Errorf("ToolCall name mismatch: got %q, want %q", decoded.ToolCalls[0].Name, "disk_usage")
	}
}

func TestToolResult_JSONRoundTrip(t *testing.T) {
	displayData := json.RawMessage(`{"usage":"85%"}`)
	original := ToolResult{
		ToolCallID: "tc-1",
		Content:    "disk usage is 85%",
		Display: &ToolDisplayPayload{
			Type:  "metric",
			Title: "Disk Usage",
			Data:  displayData,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded ToolResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ToolCallID != original.ToolCallID {
		t.Errorf("ToolCallID mismatch: got %q, want %q", decoded.ToolCallID, original.ToolCallID)
	}
	if decoded.Content != original.Content {
		t.Errorf("Content mismatch: got %q, want %q", decoded.Content, original.Content)
	}
	if decoded.Display == nil {
		t.Fatal("Display should not be nil")
	}
	if decoded.Display.Type != "metric" {
		t.Errorf("Display.Type mismatch: got %q, want %q", decoded.Display.Type, "metric")
	}
	if decoded.Display.Title != "Disk Usage" {
		t.Errorf("Display.Title mismatch: got %q, want %q", decoded.Display.Title, "Disk Usage")
	}
}

func TestApprovalRecord_Validate(t *testing.T) {
	valid := ApprovalRecord{
		ID:        "apr-1",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		ToolName:  "host.exec",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid ApprovalRecord.Validate() = %v", err)
	}

	noID := valid
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Error("ApprovalRecord without id should fail")
	}

	noSession := valid
	noSession.SessionID = ""
	if err := noSession.Validate(); err == nil {
		t.Error("ApprovalRecord without session id should fail")
	}

	noTurn := valid
	noTurn.TurnID = ""
	if err := noTurn.Validate(); err == nil {
		t.Error("ApprovalRecord without turn id should fail")
	}

	noTool := valid
	noTool.ToolName = ""
	if err := noTool.Validate(); err == nil {
		t.Error("ApprovalRecord without tool name should fail")
	}
}

func TestWorkspaceTask_Validate(t *testing.T) {
	valid := WorkspaceTask{
		ID:          "task-1",
		Type:        "host_exec",
		Status:      "pending",
		Description: "check disk",
		StartTime:   time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid WorkspaceTask.Validate() = %v", err)
	}

	noID := valid
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Error("WorkspaceTask without id should fail")
	}

	noType := valid
	noType.Type = ""
	if err := noType.Validate(); err == nil {
		t.Error("WorkspaceTask without type should fail")
	}

	noStatus := valid
	noStatus.Status = ""
	if err := noStatus.Validate(); err == nil {
		t.Error("WorkspaceTask without status should fail")
	}
}

func TestWorkspaceTask_JSONRoundTrip(t *testing.T) {
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	endTime := now.Add(5 * time.Minute)
	original := WorkspaceTask{
		ID:          "task-abc",
		Type:        "multi_host",
		Status:      "completed",
		Description: "check all disks",
		HostIDs:     []string{"host-1", "host-2"},
		StartTime:   now,
		EndTime:     &endTime,
		Output:      "all disks healthy",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded WorkspaceTask
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: got %q, want %q", decoded.Type, original.Type)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status mismatch: got %q, want %q", decoded.Status, original.Status)
	}
	if len(decoded.HostIDs) != 2 {
		t.Fatalf("HostIDs length mismatch: got %d, want 2", len(decoded.HostIDs))
	}
	if decoded.Output != original.Output {
		t.Errorf("Output mismatch: got %q, want %q", decoded.Output, original.Output)
	}
	if decoded.EndTime == nil {
		t.Fatal("EndTime should not be nil")
	}
	if !decoded.EndTime.Equal(endTime) {
		t.Errorf("EndTime mismatch: got %v, want %v", decoded.EndTime, endTime)
	}
}

func TestApprovalRecord_JSONRoundTrip(t *testing.T) {
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	decidedAt := now.Add(2 * time.Minute)
	original := ApprovalRecord{
		ID:        "apr-1",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		ToolName:  "host.exec",
		Command:   "rm -rf /tmp/old",
		HostID:    "host-1",
		Status:    "approved",
		Operator:  "admin",
		Decision:  "approved",
		CreatedAt: now,
		DecidedAt: &decidedAt,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded ApprovalRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.ToolName != original.ToolName {
		t.Errorf("ToolName mismatch: got %q, want %q", decoded.ToolName, original.ToolName)
	}
	if decoded.Command != original.Command {
		t.Errorf("Command mismatch: got %q, want %q", decoded.Command, original.Command)
	}
	if decoded.DecidedAt == nil {
		t.Fatal("DecidedAt should not be nil")
	}
	if !decoded.DecidedAt.Equal(decidedAt) {
		t.Errorf("DecidedAt mismatch: got %v, want %v", decoded.DecidedAt, decidedAt)
	}
}
