package hostops

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/opssemantic"
)

func TestHostAgentFullRuntimeEvalMultiHostReadAndWriteTaskScheduling(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	scheduler := NewHostSubTaskScheduler(store)
	executor := &evalHostCommandExecutor{stdout: "generic status ok"}
	tool := NewHostCommandTool(executor, NewCommandPolicy(CommandPolicyConfig{
		GlobalWhitelist: []CommandPolicyRule{{
			ID:      "generic-read",
			Pattern: "inspect-resource *",
			MaxRisk: opssemantic.RiskReadOnly,
		}},
	}))

	for _, hostID := range []string{"host-alpha", "host-beta"} {
		result, err := tool.Run(ctx, HostCommandToolRequest{
			ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: hostID},
			MissionID:    "mission-eval",
			ChildAgentID: "child-" + strings.TrimPrefix(hostID, "host-"),
			PlanStepID:   "step-read-" + strings.TrimPrefix(hostID, "host-"),
			HostID:       hostID,
			HostAddress:  hostID + ".agent.local",
			Command:      "inspect-resource current-state",
			RiskLevel:    opssemantic.RiskReadOnly,
		})
		if err != nil {
			t.Fatalf("read command for %s returned error = %v", hostID, err)
		}
		if !result.Executed || result.ApprovalRequired {
			t.Fatalf("read command result for %s = %#v, want executed without approval", hostID, result)
		}
	}
	if len(executor.requests) != 2 || executor.requests[0].HostID != "host-alpha" || executor.requests[1].HostID != "host-beta" {
		t.Fatalf("executor requests = %#v, want one read execution per bound host", executor.requests)
	}

	alphaFirst, err := scheduler.Schedule(ctx, HostSubTask{
		ID:          "subtask-alpha-write-1",
		MissionID:   "mission-eval",
		PlanStepID:  "step-write-alpha-1",
		HostAgentID: "child-alpha",
		HostID:      "host-alpha",
		Goal:        "apply approved generic change",
		ActionType:  opssemantic.ActionWrite,
		RiskLevel:   opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Schedule(alphaFirst) error = %v", err)
	}
	alphaSecond, err := scheduler.Schedule(ctx, HostSubTask{
		ID:          "subtask-alpha-write-2",
		MissionID:   "mission-eval",
		PlanStepID:  "step-write-alpha-2",
		HostAgentID: "child-alpha",
		HostID:      "host-alpha",
		Goal:        "apply follow-up generic change",
		ActionType:  opssemantic.ActionWrite,
		RiskLevel:   opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Schedule(alphaSecond) error = %v", err)
	}
	betaWrite, err := scheduler.Schedule(ctx, HostSubTask{
		ID:          "subtask-beta-write-1",
		MissionID:   "mission-eval",
		PlanStepID:  "step-write-beta-1",
		HostAgentID: "child-beta",
		HostID:      "host-beta",
		Goal:        "apply independent generic change",
		ActionType:  opssemantic.ActionWrite,
		RiskLevel:   opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Schedule(betaWrite) error = %v", err)
	}
	if alphaFirst.Status != HostSubTaskStatusRunning {
		t.Fatalf("alphaFirst = %#v, want running", alphaFirst)
	}
	if alphaSecond.Status != HostSubTaskStatusQueued || alphaSecond.ActiveSubTaskID != alphaFirst.SubTaskID {
		t.Fatalf("alphaSecond = %#v, want queued behind active alpha write", alphaSecond)
	}
	if betaWrite.Status != HostSubTaskStatusRunning {
		t.Fatalf("betaWrite = %#v, want independent host write running", betaWrite)
	}
}

func TestHostAgentFullRuntimeEvalApprovalDeniedBlocksChildWithoutExecution(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := newHostAgentFullRuntimeEvalFixture(t)
	executor := &evalHostCommandExecutor{stdout: "write completed"}
	approvalStore := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvalStore,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	tool := NewHostCommandToolWithApprovals(executor, NewCommandPolicy(CommandPolicyConfig{}), controller)

	result, err := tool.Run(ctx, HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-alpha"},
		MissionID:    "mission-eval",
		ChildAgentID: "child-alpha",
		PlanStepID:   "step-write-alpha",
		HostID:       "host-alpha",
		HostAddress:  "host-alpha.agent.local",
		Command:      "apply-change --scope current",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.ApprovalRequired || result.ApprovalID == "" || result.Executed {
		t.Fatalf("result = %#v, want pending approval without execution", result)
	}

	approval, _, err := controller.Decide(ctx, result.ApprovalID, "denied")
	if err != nil {
		t.Fatalf("Decide(denied) error = %v", err)
	}
	if approval.Status != CommandApprovalStatusDenied || approval.Decision != "denied" {
		t.Fatalf("approval = %#v, want denied", approval)
	}
	if len(executor.requests) != 0 {
		t.Fatalf("executor requests = %#v, want no command execution after denial", executor.requests)
	}
	child, err := missions.GetChildAgent(ctx, "child-alpha")
	if err != nil {
		t.Fatalf("GetChildAgent() error = %v", err)
	}
	if child.Status != HostChildAgentStatusBlocked || child.Error == "" {
		t.Fatalf("child = %#v, want blocked child after denied approval", child)
	}
	items, err := transcripts.List(ctx, "child-alpha")
	if err != nil {
		t.Fatalf("List transcript error = %v", err)
	}
	if len(items) < 2 || items[len(items)-1].Type != TranscriptItemApproval || items[len(items)-1].Status != "denied" {
		t.Fatalf("transcript = %#v, want denied approval event", items)
	}
}

func TestHostAgentFullRuntimeEvalRejectsSensitiveAndCrossHostContamination(t *testing.T) {
	rawCredential := "eval-sensitive-value"
	longContext := strings.Repeat("generic diagnostic line\n", 80) + "password=" + rawCredential
	planStep := PlanStep{
		ID:               "step-read-alpha",
		Title:            "Inspect resource token=" + rawCredential,
		Summary:          "Use only bound host context",
		HostIDs:          []string{"host-alpha"},
		ActionType:       opssemantic.ActionReadOnly,
		RiskLevel:        opssemantic.RiskReadOnly,
		EvidenceRequired: []string{"command evidence ref"},
	}

	runtimeContext, trace, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
		MissionID:       "mission-eval",
		ParentAgentID:   "manager-eval",
		HostAgentID:     "child-alpha",
		SessionID:       "session-child-alpha",
		HostID:          "host-alpha",
		HostAddress:     "host-alpha.agent.local",
		HostDisplayName: "Host Alpha",
		PlanStep:        planStep,
		Goal:            "Inspect status with token=" + rawCredential,
		Constraints:     []string{"Never persist Bearer " + rawCredential},
		ContextRefs: []ContextRef{{
			ID:          "ctx-alpha-small",
			Kind:        "transcript",
			ScopeHostID: "host-alpha",
			Content:     "bounded context for alpha",
		}, {
			ID:          "ctx-beta-private",
			Kind:        "transcript",
			ScopeHostID: "host-beta",
			Content:     "private beta context",
		}, {
			ID:          "ctx-alpha-long",
			Kind:        "tool-output",
			ScopeHostID: "host-alpha",
			Content:     longContext,
		}},
	})
	if err != nil {
		t.Fatalf("BuildHostAgentRuntimeContext() error = %v", err)
	}
	if containsRaw(runtimeContext, rawCredential) {
		t.Fatalf("runtime context leaked raw credential: %#v", runtimeContext)
	}
	if ref := findContextRef(runtimeContext.ContextRefs, "ctx-beta-private"); ref != nil {
		t.Fatalf("runtime context included cross-host ref: %#v", ref)
	}
	longRef := findContextRef(runtimeContext.ContextRefs, "ctx-alpha-long")
	if longRef == nil || longRef.Content != "" || longRef.ArtifactRef == "" || longRef.Digest == "" {
		t.Fatalf("long context ref = %#v, want externalized ref with digest", longRef)
	}
	if !traceHasExcluded(trace, "ctx-beta-private", ContextDecisionScopeViolation) {
		t.Fatalf("trace = %#v, want cross-host exclusion decision", trace)
	}
	if !traceHasExternalized(trace, "ctx-alpha-long") {
		t.Fatalf("trace = %#v, want long context externalization decision", trace)
	}

	validator := NewHostTaskReportValidator(HostTaskReportValidationContext{
		MissionID:   "mission-eval",
		PlanStepID:  "step-read-alpha",
		HostAgentID: "child-alpha",
		HostID:      "host-alpha",
	})
	sanitized, err := validator.Sanitized(HostTaskReport{
		MissionID:   "mission-eval",
		PlanStepID:  "step-read-alpha",
		HostAgentID: "child-alpha",
		HostID:      "host-alpha",
		Status:      string(HostTaskReportStatusCompleted),
		Summary:     "Completed without persisting token=" + rawCredential,
		Commands: []HostTaskCommandRecord{{
			Command: "inspect-resource --token " + rawCredential,
			Status:  "success",
			Summary: "Bearer " + rawCredential,
		}},
		Evidence: []HostTaskEvidence{{
			ID:              "evidence-alpha",
			HostID:          "host-alpha",
			Source:          EvidenceSourceHostCommandTool,
			RedactionStatus: RedactionStatusApplied,
		}},
	})
	if err != nil {
		t.Fatalf("Sanitized() error = %v", err)
	}
	if containsRaw(sanitized, rawCredential) {
		t.Fatalf("sanitized report leaked raw credential: %#v", sanitized)
	}

	err = validator.Validate(HostTaskReport{
		MissionID:   "mission-eval",
		PlanStepID:  "step-read-alpha",
		HostAgentID: "child-alpha",
		HostID:      "host-alpha",
		Status:      string(HostTaskReportStatusCompleted),
		Evidence: []HostTaskEvidence{{
			ID:              "evidence-beta",
			HostID:          "host-beta",
			Source:          EvidenceSourceHostCommandTool,
			RedactionStatus: RedactionStatusNotRequired,
		}},
	})
	if !errors.Is(err, ErrInvalidHostTaskReport) {
		t.Fatalf("Validate(cross-host evidence) error = %v, want ErrInvalidHostTaskReport", err)
	}
}

func TestHostAgentFullRuntimeEvalCrossHostCommandIsSecurityRefused(t *testing.T) {
	ctx := context.Background()
	transcripts := NewInMemoryTranscriptStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       NewInMemoryCommandApprovalStore(),
		Transcripts: transcripts,
	})
	executor := &evalHostCommandExecutor{stdout: "generic status ok"}
	tool := NewHostCommandToolWithApprovals(executor, NewCommandPolicy(CommandPolicyConfig{
		GlobalWhitelist: []CommandPolicyRule{{
			ID:      "generic-read",
			Pattern: "inspect-resource *",
			MaxRisk: opssemantic.RiskReadOnly,
		}},
	}), controller)

	_, err := tool.Run(ctx, HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-alpha"},
		MissionID:    "mission-eval",
		ChildAgentID: "child-alpha",
		PlanStepID:   "step-read-alpha",
		HostID:       "host-beta",
		HostAddress:  "host-beta.agent.local",
		Command:      "inspect-resource current-state",
		RiskLevel:    opssemantic.RiskReadOnly,
	})
	if !errors.Is(err, ErrCrossHostDenied) {
		t.Fatalf("Run(cross-host) error = %v, want ErrCrossHostDenied", err)
	}
	if len(executor.requests) != 0 {
		t.Fatalf("executor requests = %#v, want no execution for cross-host command", executor.requests)
	}
	items, listErr := transcripts.List(ctx, "child-alpha")
	if listErr != nil {
		t.Fatalf("List transcript error = %v", listErr)
	}
	if len(items) != 1 || items[0].Type != TranscriptItemError || items[0].Status != "security_refused" {
		t.Fatalf("transcript = %#v, want security refusal event", items)
	}
}

func newHostAgentFullRuntimeEvalFixture(t *testing.T) (*InMemoryMissionStore, *InMemoryTranscriptStore) {
	t.Helper()
	ctx := context.Background()
	missions := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	mission := HostOperationMission{
		ID:             "mission-eval",
		ThreadID:       "thread-eval",
		UserTurnID:     "turn-eval",
		ManagerAgentID: "manager-eval",
		Status:         HostMissionStatusRunning,
		PlanRequired:   true,
		PlanAccepted:   true,
		Plan: HostOperationPlan{
			ID:      "plan-eval",
			Version: 1,
			Status:  PlanStatusRunning,
			Steps: []PlanStep{{
				ID:         "step-write-alpha",
				Index:      1,
				Title:      "Apply generic host change",
				Status:     PlanStepStatusRunning,
				HostIDs:    []string{"host-alpha"},
				RiskLevel:  opssemantic.RiskLowWrite,
				ActionType: opssemantic.ActionWrite,
			}},
		},
		Mentions: []HostMention{{
			Raw:         "@host-alpha",
			HostID:      "host-alpha",
			DisplayName: "Host Alpha",
			Resolved:    true,
			Source:      HostMentionSourceInventory,
		}},
	}
	if err := missions.SaveMission(ctx, mission); err != nil {
		t.Fatalf("SaveMission error = %v", err)
	}
	if err := missions.SaveChildAgent(ctx, HostChildAgent{
		ID:              "child-alpha",
		MissionID:       "mission-eval",
		ParentAgentID:   "manager-eval",
		SessionID:       "session-child-alpha",
		HostID:          "host-alpha",
		HostAddress:     "host-alpha.agent.local",
		HostDisplayName: "Host Alpha",
		Status:          HostChildAgentStatusRunning,
		PlanStepIDs:     []string{"step-write-alpha"},
	}); err != nil {
		t.Fatalf("SaveChildAgent error = %v", err)
	}
	return missions, transcripts
}

type evalHostCommandExecutor struct {
	stdout   string
	requests []HostCommandRequest
}

func (e *evalHostCommandExecutor) RunShell(_ context.Context, _ ToolContext, req HostCommandRequest) (HostCommandResult, error) {
	e.requests = append(e.requests, req)
	return HostCommandResult{Status: "success", Stdout: e.stdout, ExitCode: 0}, nil
}

func findContextRef(refs []ContextRef, id string) *ContextRef {
	for i := range refs {
		if refs[i].ID == id {
			return &refs[i]
		}
	}
	return nil
}

func traceHasExcluded(trace ContextDecisionTrace, id, reason string) bool {
	for _, item := range trace.Excluded {
		if item.ID == id && item.Reason == reason {
			return true
		}
	}
	return false
}

func traceHasExternalized(trace ContextDecisionTrace, id string) bool {
	for _, item := range trace.Externalized {
		if item.ID == id && item.Ref != "" && item.Digest != "" {
			return true
		}
	}
	return false
}

func containsRaw(value any, raw string) bool {
	data, _ := json.Marshal(value)
	return strings.Contains(string(data), raw)
}
