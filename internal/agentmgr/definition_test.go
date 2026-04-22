package agentmgr

import (
	"testing"
	"time"

	"aiops-v2/internal/agents"
)

func TestAgentDefinition_ToRegistryDefinition(t *testing.T) {
	def := AgentDefinition{
		Kind:           AgentKindWorker,
		Name:           "worker-v1",
		Description:    "worker agent",
		Prompt:         "worker prompt",
		PromptTemplate: "worker_template",
		Tools:          []string{"read_file", "exec_command"},
		Model:         "gpt-4o-mini",
		Hooks:         []string{"pre_tool_use"},
		MCPServers:    []string{"filesystem"},
		MaxIterations: 20,
	}

	got := def.ToRegistryDefinition()

	if got.Kind != string(def.Kind) {
		t.Fatalf("Kind = %q, want %q", got.Kind, def.Kind)
	}
	if got.Name != def.Name {
		t.Fatalf("Name = %q, want %q", got.Name, def.Name)
	}
	if got.Source != string(agents.SourceBuiltin) {
		t.Fatalf("Source = %q, want %q", got.Source, agents.SourceBuiltin)
	}
	if got.Description != def.Description {
		t.Fatalf("Description = %q, want %q", got.Description, def.Description)
	}
	if got.Prompt != def.Prompt {
		t.Fatalf("Prompt = %q, want %q", got.Prompt, def.Prompt)
	}
	if got.Model != def.Model {
		t.Fatalf("Model = %q, want %q", got.Model, def.Model)
	}
	if got.MaxIterations != def.MaxIterations {
		t.Fatalf("MaxIterations = %d, want %d", got.MaxIterations, def.MaxIterations)
	}
	if len(got.Tools) != len(def.Tools) || got.Tools[0] != def.Tools[0] {
		t.Fatalf("Tools = %#v, want %#v", got.Tools, def.Tools)
	}
	if len(got.Hooks) != len(def.Hooks) || got.Hooks[0] != def.Hooks[0] {
		t.Fatalf("Hooks = %#v, want %#v", got.Hooks, def.Hooks)
	}
	if len(got.MCPServers) != len(def.MCPServers) || got.MCPServers[0] != def.MCPServers[0] {
		t.Fatalf("MCPServers = %#v, want %#v", got.MCPServers, def.MCPServers)
	}
}

func TestAgentDefinition_ToAgentToolUsesRegistryView(t *testing.T) {
	def := AgentDefinition{
		Kind:           AgentKindWorker,
		Name:           "worker-v1",
		Description:    "worker agent",
		PromptTemplate: "worker_template",
		Tools:          []string{"read_file", "exec_command"},
		Model:          "gpt-4o-mini",
		Hooks:          []string{"pre_tool_use"},
		MCPServers:     []string{"filesystem"},
		MaxIterations:  20,
	}

	got := def.ToAgentTool()

	if got.Kind != string(def.Kind) {
		t.Fatalf("AgentTool.Kind = %q, want %q", got.Kind, def.Kind)
	}
	if got.Name != def.Name {
		t.Fatalf("AgentTool.Name = %q, want %q", got.Name, def.Name)
	}
	if got.Description != def.Description {
		t.Fatalf("AgentTool.Description = %q, want %q", got.Description, def.Description)
	}
	if got.Prompt != def.PromptTemplate {
		t.Fatalf("AgentTool.Prompt = %q, want %q", got.Prompt, def.PromptTemplate)
	}
	if got.Model != def.Model {
		t.Fatalf("AgentTool.Model = %q, want %q", got.Model, def.Model)
	}
	if len(got.Tools) != len(def.Tools) || got.Tools[1] != def.Tools[1] {
		t.Fatalf("AgentTool.Tools = %#v, want %#v", got.Tools, def.Tools)
	}
	if len(got.Hooks) != len(def.Hooks) || got.Hooks[0] != def.Hooks[0] {
		t.Fatalf("AgentTool.Hooks = %#v, want %#v", got.Hooks, def.Hooks)
	}
	if len(got.MCPServers) != len(def.MCPServers) || got.MCPServers[0] != def.MCPServers[0] {
		t.Fatalf("AgentTool.MCPServers = %#v, want %#v", got.MCPServers, def.MCPServers)
	}
}

func TestRegistryDefinition_ToAgentDefinition(t *testing.T) {
	def := agents.Definition{
		Kind:            "planner",
		Name:            "planner-v1",
		Description:     "planner agent",
		Prompt:          "planner prompt",
		Tools:           []string{"read_file"},
		Model:           "gpt-4.1",
		Hooks:           []string{"post_tool_use"},
		MCPServers:      []string{"filesystem"},
		MaxIterations:   8,
	}

	got := FromRegistryDefinition(def)

	if got.Kind != AgentKind(def.Kind) {
		t.Fatalf("Kind = %q, want %q", got.Kind, def.Kind)
	}
	if got.Name != def.Name {
		t.Fatalf("Name = %q, want %q", got.Name, def.Name)
	}
	if got.Description != def.Description {
		t.Fatalf("Description = %q, want %q", got.Description, def.Description)
	}
	if got.Prompt != def.Prompt {
		t.Fatalf("Prompt = %q, want %q", got.Prompt, def.Prompt)
	}
	if got.PromptTemplate != def.Prompt {
		t.Fatalf("PromptTemplate = %q, want %q", got.PromptTemplate, def.Prompt)
	}
	if got.Model != def.Model {
		t.Fatalf("Model = %q, want %q", got.Model, def.Model)
	}
	if got.MaxIterations != def.MaxIterations {
		t.Fatalf("MaxIterations = %d, want %d", got.MaxIterations, def.MaxIterations)
	}
}

func TestAgentDefinition_Validate(t *testing.T) {
	valid := AgentDefinition{
		Kind:           AgentKindWorker,
		Name:           "host-worker",
		PromptTemplate: "worker_v1",
		MaxIterations: 10,
		Model:         "gpt-4o-mini",
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("valid AgentDefinition.Validate() returned error: %v", err)
	}

	invalid := valid
	invalid.Kind = "bad"
	if err := invalid.Validate(); err == nil {
		t.Error("AgentDefinition with invalid kind should fail validation")
	}

	invalid = valid
	invalid.Name = ""
	if err := invalid.Validate(); err == nil {
		t.Error("AgentDefinition with empty name should fail validation")
	}

	invalid = valid
	invalid.MaxIterations = -1
	if err := invalid.Validate(); err == nil {
		t.Error("AgentDefinition with negative MaxIterations should fail validation")
	}
}

func TestAgentInstance_Validate(t *testing.T) {
	now := time.Now()
	valid := AgentInstance{
		ID:        "agent-001",
		Kind:      AgentKindWorker,
		MissionID: "mission-001",
		SessionID: "session-001",
		Status:    AgentStatusRunning,
		HostID:    "host-1",
		Task:      "check disk usage",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("valid AgentInstance.Validate() returned error: %v", err)
	}

	invalid := valid
	invalid.ID = ""
	if err := invalid.Validate(); err == nil {
		t.Error("AgentInstance with empty ID should fail validation")
	}

	invalid = valid
	invalid.Kind = "bad"
	if err := invalid.Validate(); err == nil {
		t.Error("AgentInstance with invalid kind should fail validation")
	}

	invalid = valid
	invalid.MissionID = ""
	if err := invalid.Validate(); err == nil {
		t.Error("AgentInstance with empty MissionID should fail validation")
	}

	invalid = valid
	invalid.SessionID = ""
	if err := invalid.Validate(); err == nil {
		t.Error("AgentInstance with empty SessionID should fail validation")
	}

	invalid = valid
	invalid.Status = "bad"
	if err := invalid.Validate(); err == nil {
		t.Error("AgentInstance with invalid status should fail validation")
	}
}

func TestAgentResult_Fields(t *testing.T) {
	result := AgentResult{
		AgentID:  "agent-001",
		HostID:   "host-1",
		Status:   AgentStatusCompleted,
		Output:   "disk usage: 45%",
		Error:    "",
		Duration: 5 * time.Second,
	}

	if result.AgentID != "agent-001" {
		t.Errorf("AgentResult.AgentID = %q, want %q", result.AgentID, "agent-001")
	}
	if result.Status != AgentStatusCompleted {
		t.Errorf("AgentResult.Status = %q, want %q", result.Status, AgentStatusCompleted)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("AgentResult.Duration = %v, want %v", result.Duration, 5*time.Second)
	}
}
