package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

func TestRuntimeModelInputCompatibilityAdapters(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "old"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "old-call", Name: "read_file"}}},
		{Role: "tool", Content: "old result", ToolResult: &ToolResult{ToolCallID: "old-call", Content: "old result"}},
		{Role: "assistant", Content: "stable answer"},
		{Role: "user", Content: "current"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "current-call", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}}},
		{Role: "tool", Content: "current result", ToolResult: &ToolResult{ToolCallID: "current-call", Content: "current result"}},
	}

	filtered := messagesForCurrentTurnModelInput(history)
	if len(filtered) != 5 {
		t.Fatalf("filtered len = %d, want stable old answer plus current turn", len(filtered))
	}
	joined := strings.Builder{}
	for _, msg := range filtered {
		joined.WriteString(msg.Content)
	}
	if strings.Contains(joined.String(), "old result") || !strings.Contains(joined.String(), "stable answer") {
		t.Fatalf("filtered messages = %#v, want prior tool noise dropped and stable answer kept", filtered)
	}

	roundTrip := runtimeMessagesFromPromptInput([]promptinput.Message{{
		Role:      "assistant",
		ToolCalls: []promptinput.ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x"}`)}},
	}, {
		Role:       "tool",
		Content:    "ok",
		ToolResult: &promptinput.ToolResult{ToolCallID: "call-1", Content: "ok"},
	}})
	if len(roundTrip) != 2 || roundTrip[0].ToolCalls[0].Name != "read_file" || roundTrip[1].ToolResult.ToolCallID != "call-1" {
		t.Fatalf("runtimeMessagesFromPromptInput() = %#v, want tool call/result preserved", roundTrip)
	}
}

type aiChatHarnessGoldenCase struct {
	Name                   string                       `json:"name"`
	UserInput              string                       `json:"userInput"`
	ModelToolCalls         []aiChatGoldenToolCall       `json:"modelToolCalls,omitempty"`
	ModelFinalOutput       string                       `json:"modelFinalOutput,omitempty"`
	AvailableTools         []aiChatGoldenTool           `json:"availableTools,omitempty"`
	ExpectedStatus         string                       `json:"expectedStatus"`
	ExpectedVisibleStates  []string                     `json:"expectedVisibleStates,omitempty"`
	ForbiddenVisibleStates []string                     `json:"forbiddenVisibleStates,omitempty"`
	ExpectedFailureKind    string                       `json:"expectedFailureKind,omitempty"`
	ExpectedAttempts       []aiChatExpectedAttempt      `json:"expectedAttempts,omitempty"`
	HostID                 string                       `json:"hostId,omitempty"`
	Mode                   string                       `json:"mode,omitempty"`
	IntentFrame            *runtimecontract.IntentFrame `json:"intentFrame,omitempty"`
	ExpectedProviderCalls  *int                         `json:"expectedProviderCalls,omitempty"`
	ExpectedHarness        aiChatExpectedHarness        `json:"expectedHarness,omitempty"`
}

type aiChatGoldenToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type aiChatGoldenTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Result      string          `json:"result,omitempty"`
	RiskLevel   string          `json:"riskLevel,omitempty"`
	Mutating    bool            `json:"mutating,omitempty"`
	Approval    *struct {
		Reason         string `json:"reason,omitempty"`
		Risk           string `json:"risk,omitempty"`
		Source         string `json:"source,omitempty"`
		ExpectedEffect string `json:"expectedEffect,omitempty"`
		Rollback       string `json:"rollback,omitempty"`
		Validation     string `json:"validation,omitempty"`
	} `json:"approval,omitempty"`
}

type aiChatExpectedAttempt struct {
	ToolCallID         string `json:"toolCallId"`
	Action             string `json:"action"`
	Outcome            string `json:"outcome"`
	TriggerFailureKind string `json:"triggerFailureKind,omitempty"`
}

type aiChatExpectedHarness struct {
	SchemaVersion            string                     `json:"schemaVersion,omitempty"`
	RouteMode                string                     `json:"routeMode,omitempty"`
	TargetBinding            string                     `json:"targetBinding,omitempty"`
	ModelVisibleTools        []string                   `json:"modelVisibleTools,omitempty"`
	HiddenTools              []aiChatExpectedHiddenTool `json:"hiddenTools,omitempty"`
	ToolCalls                []string                   `json:"toolCalls,omitempty"`
	ApprovalRequested        *bool                      `json:"approvalRequested,omitempty"`
	ApprovalDecided          *bool                      `json:"approvalDecided,omitempty"`
	EvidenceRefs             []string                   `json:"evidenceRefs,omitempty"`
	FinalStatus              string                     `json:"finalStatus,omitempty"`
	RollbackContractRequired *bool                      `json:"rollbackContractRequired,omitempty"`
	TimelineOrder            []string                   `json:"timelineOrder,omitempty"`
	NoRawDSMLFinal           bool                       `json:"noRawDsmlFinal,omitempty"`
	Extra                    map[string]json.RawMessage `json:"extra,omitempty"`
}

type aiChatExpectedHiddenTool struct {
	Name   string `json:"name"`
	Reason string `json:"reason,omitempty"`
}

func loadAIChatHarnessGoldenCases(t *testing.T, dir string) []aiChatHarnessGoldenCase {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read golden cases from %s: %v", dir, err)
	}
	cases := make([]aiChatHarnessGoldenCase, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden case %s: %v", path, err)
		}
		var tc aiChatHarnessGoldenCase
		if err := json.Unmarshal(data, &tc); err != nil {
			t.Fatalf("decode golden case %s: %v", path, err)
		}
		if strings.TrimSpace(tc.Name) == "" {
			t.Fatalf("golden case %s missing name", path)
		}
		if strings.TrimSpace(tc.ExpectedStatus) == "" {
			t.Fatalf("golden case %s missing expectedStatus", path)
		}
		cases = append(cases, tc)
	}
	if len(cases) == 0 {
		t.Fatalf("no golden cases found in %s", dir)
	}
	return cases
}

func loadGoldenCaseByName(t *testing.T, dir, name string) aiChatHarnessGoldenCase {
	t.Helper()
	for _, tc := range loadAIChatHarnessGoldenCases(t, dir) {
		if tc.Name == name {
			return tc
		}
	}
	t.Fatalf("golden case %q not found in %s", name, dir)
	return aiChatHarnessGoldenCase{}
}

func runGoldenTurn(t *testing.T, tc aiChatHarnessGoldenCase) (TurnResult, *TurnSnapshot, []LifecycleEvent) {
	t.Helper()

	responses := make([]*schema.Message, 0, 2)
	if len(tc.ModelToolCalls) > 0 {
		responses = append(responses, schema.AssistantMessage("", schemaToolCallsFromGolden(tc.ModelToolCalls)))
	}
	if strings.TrimSpace(tc.ModelFinalOutput) != "" || len(tc.ModelToolCalls) == 0 {
		responses = append(responses, schema.AssistantMessage(tc.ModelFinalOutput, nil))
	}
	model := &sequentialLoopModel{responses: responses}
	registry := tooling.NewRegistry()
	for _, toolSpec := range tc.AvailableTools {
		toolDef := staticToolFromGolden(t, toolSpec)
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("register golden tool %q: %v", toolSpec.Name, err)
		}
	}
	kernel, emitter := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, &testMockCompiler{}, model)
	mode := ModeInspect
	if strings.TrimSpace(tc.Mode) != "" {
		mode = Mode(strings.TrimSpace(tc.Mode))
	}
	metadata := map[string]string{
		"taskDepth":                     "simple_read",
		"aiops.target.binding":          "none",
		"aiops.tool.execCommandAllowed": "false",
	}
	if hostID := strings.TrimSpace(tc.HostID); hostID != "" {
		metadata["aiops.target.binding"] = "host"
		metadata["aiops.target.hostId"] = hostID
		metadata["aiops.target.refs"] = "host:" + hostID
		metadata["aiops.tool.execCommandAllowed"] = "true"
		metadata["aiops.route.allowsExecCommand"] = "true"
	}

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-golden-" + tc.Name,
		SessionType: SessionTypeHost,
		Mode:        mode,
		TurnID:      "turn-golden-" + tc.Name,
		HostID:      strings.TrimSpace(tc.HostID),
		IntentFrame: tc.IntentFrame,
		Input:       tc.UserInput,
		Metadata:    metadata,
	})
	if err != nil && tc.ExpectedStatus != "error" {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if tc.ExpectedProviderCalls != nil && len(model.inputs) != *tc.ExpectedProviderCalls {
		t.Fatalf("provider calls = %d, want %d", len(model.inputs), *tc.ExpectedProviderCalls)
	}
	session := kernel.sessions.Get("sess-golden-" + tc.Name)
	if session == nil || session.CurrentTurn == nil {
		t.Fatalf("missing current turn snapshot for golden case %s", tc.Name)
	}
	return result, session.CurrentTurn, append([]LifecycleEvent(nil), emitter.events...)
}

func schemaToolCallsFromGolden(calls []aiChatGoldenToolCall) []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, schema.ToolCall{
			ID:   call.ID,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      call.Name,
				Arguments: string(call.Arguments),
			},
		})
	}
	return out
}

func staticToolFromGolden(t *testing.T, spec aiChatGoldenTool) *tooling.StaticTool {
	t.Helper()
	meta := tooling.ToolMetadata{
		Name:        spec.Name,
		Description: firstNonEmpty(spec.Description, spec.Name),
		Origin:      tooling.ToolOriginBuiltin,
		Mutating:    spec.Mutating,
	}
	if spec.Mutating {
		meta.RequiresApproval = true
		meta.RiskLevel = tooling.ToolRiskHigh
		meta.ResourceLocks = []tooling.ToolResourceLockKey{{
			ResourceType:  "host",
			ResourceID:    "golden-target",
			OperationKind: "mutation",
		}}
		meta.Idempotency = tooling.ToolIdempotencyMetadata{
			Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
			PostCheckRefs: []string{"golden-post-check"},
		}
	}
	if spec.Approval != nil {
		meta.Discovery.PermissionScope = "argument_scoped"
	}
	if spec.RiskLevel != "" {
		meta.RiskLevel = tooling.ToolRiskLevel(spec.RiskLevel)
	}
	inputSchema := spec.InputSchema
	if len(inputSchema) == 0 {
		inputSchema = json.RawMessage(`{"type":"object"}`)
	}
	result := firstNonEmpty(spec.Result, fmt.Sprintf("%s result", spec.Name))
	return &tooling.StaticTool{
		Meta:            meta,
		InputSchemaData: inputSchema,
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect), string(ModeExecute)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return !spec.Mutating },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			if spec.Approval == nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
			}
			return tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: firstNonEmpty(spec.Approval.Reason, "approval required"),
				Approval: &tooling.PermissionApprovalPayload{
					Reason:         firstNonEmpty(spec.Approval.Reason, "approval required"),
					Risk:           firstNonEmpty(spec.Approval.Risk, string(meta.RiskLevel.Normalize())),
					Source:         firstNonEmpty(spec.Approval.Source, "golden"),
					ExpectedEffect: spec.Approval.ExpectedEffect,
					Rollback:       spec.Approval.Rollback,
					Validation:     spec.Approval.Validation,
				},
			}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: result}, nil
		},
	}
}

func assertGoldenTurnContract(t *testing.T, tc aiChatHarnessGoldenCase, result TurnResult, snapshot *TurnSnapshot, events []LifecycleEvent) {
	t.Helper()

	if result.Status != tc.ExpectedStatus {
		t.Fatalf("result status = %q, want %q; result=%#v", result.Status, tc.ExpectedStatus, result)
	}
	assertNoNewTransportStates(t, events, tc.ForbiddenVisibleStates)
	for _, state := range tc.ExpectedVisibleStates {
		if !goldenStateObserved(state, snapshot, events) {
			t.Fatalf("expected visible state %q was not observed; snapshot lifecycle=%q events=%v", state, snapshot.Lifecycle, eventTypes(events))
		}
	}
	assertFailureKindIfExpected(t, snapshot, tc.ExpectedFailureKind)
	assertToolInvocationsRecorded(t, tc, snapshot)
	assertCheckpointSequenceMonotonic(t, snapshot)
	assertExpectedHarnessContract(t, tc, snapshot, events)
	providerFreeAdmissionFailure := tc.ExpectedProviderCalls != nil && *tc.ExpectedProviderCalls == 0
	if providerFreeAdmissionFailure {
		if snapshot.StableToolFingerprint != "" || len(snapshot.Iterations) != 0 {
			t.Fatalf("provider-free admission failure compiled model state: fingerprint=%q iterations=%d", snapshot.StableToolFingerprint, len(snapshot.Iterations))
		}
	} else {
		if strings.TrimSpace(snapshot.StableToolFingerprint) == "" {
			t.Fatal("expected stable tool fingerprint to be recorded")
		}
		if len(snapshot.Iterations) > 0 && strings.TrimSpace(snapshot.Iterations[len(snapshot.Iterations)-1].ToolSurfaceFingerprint) == "" {
			t.Fatal("expected latest iteration tool surface fingerprint to be recorded")
		}
	}
}

func assertExpectedHarnessContract(t *testing.T, tc aiChatHarnessGoldenCase, snapshot *TurnSnapshot, events []LifecycleEvent) {
	t.Helper()
	expected := tc.ExpectedHarness
	if strings.TrimSpace(expected.SchemaVersion) == "" && strings.TrimSpace(expected.RouteMode) == "" && strings.TrimSpace(expected.FinalStatus) == "" {
		return
	}
	if expected.RouteMode != "" && string(snapshot.Mode) != expected.RouteMode {
		t.Fatalf("harness routeMode = %q, want %q", snapshot.Mode, expected.RouteMode)
	}
	if expected.TargetBinding != "" {
		if got := goldenTargetBinding(snapshot); got != expected.TargetBinding {
			t.Fatalf("harness targetBinding = %q, want %q", got, expected.TargetBinding)
		}
	}
	for _, name := range expected.ModelVisibleTools {
		if !goldenToolSurfaceHasTool(snapshot, name) {
			t.Fatalf("modelVisibleTools missing %q; snapshot=%#v", name, snapshot.ToolSurfaceSnapshot)
		}
	}
	for _, hidden := range expected.HiddenTools {
		if !goldenHiddenToolObserved(snapshot, hidden) {
			t.Fatalf("hidden tool not observed: %#v; snapshot=%#v", hidden, snapshot.ToolSurfaceSnapshot)
		}
	}
	for _, callName := range expected.ToolCalls {
		if !goldenToolCallObserved(snapshot, callName) {
			t.Fatalf("tool call %q not observed", callName)
		}
	}
	if expected.ApprovalRequested != nil {
		got := len(snapshot.PendingApprovals) > 0 || hasEventType(events, EventApprovalNeeded)
		if got != *expected.ApprovalRequested {
			t.Fatalf("approvalRequested = %v, want %v", got, *expected.ApprovalRequested)
		}
	}
	if expected.ApprovalDecided != nil {
		got := hasEventType(events, EventApprovalDecided)
		if got != *expected.ApprovalDecided {
			t.Fatalf("approvalDecided = %v, want %v", got, *expected.ApprovalDecided)
		}
	}
	for _, ref := range expected.EvidenceRefs {
		if !containsString(assistantMessageEvidenceRefsFromSnapshot(snapshot), ref) {
			t.Fatalf("evidence refs = %#v, want %q", assistantMessageEvidenceRefsFromSnapshot(snapshot), ref)
		}
	}
	if expected.FinalStatus != "" {
		if got := goldenFinalContractStatus(snapshot); got != expected.FinalStatus {
			t.Fatalf("final contract status = %q, want %q; final=%q", got, expected.FinalStatus, snapshot.FinalOutput)
		}
	}
	if expected.RollbackContractRequired != nil {
		got := goldenPendingApprovalHasRollbackContract(snapshot)
		if got != *expected.RollbackContractRequired {
			t.Fatalf("rollbackContractRequired = %v, want %v; approvals=%#v", got, *expected.RollbackContractRequired, snapshot.PendingApprovals)
		}
	}
	assertGoldenTimelineOrder(t, snapshot, expected.TimelineOrder)
	if expected.NoRawDSMLFinal && strings.Contains(snapshot.FinalOutput, "DSML") {
		t.Fatalf("final output leaked raw DSML markup: %q", snapshot.FinalOutput)
	}
}

func assertNoNewTransportStates(t *testing.T, events []LifecycleEvent, forbidden []string) {
	t.Helper()
	if len(forbidden) == 0 {
		forbidden = []string{"fallback_planned", "retry_scheduled", "manual_reconcile"}
	}
	eventTypes := eventTypes(events)
	for _, blocked := range forbidden {
		for _, eventType := range eventTypes {
			if eventType == blocked {
				t.Fatalf("forbidden visible state/event %q observed in %v", blocked, eventTypes)
			}
		}
	}
}

func assertFailureKindIfExpected(t *testing.T, snapshot *TurnSnapshot, expected string) {
	t.Helper()
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return
	}
	for _, iter := range snapshot.Iterations {
		for _, result := range iter.ToolResults {
			if failureKindFromToolResult(result) == expected {
				return
			}
		}
	}
	t.Fatalf("expected failure kind %q not found in tool results", expected)
}

func assertToolInvocationsRecorded(t *testing.T, tc aiChatHarnessGoldenCase, snapshot *TurnSnapshot) {
	t.Helper()
	if len(tc.ModelToolCalls) == 0 {
		return
	}
	invocations := make([]ToolInvocationState, 0, len(tc.ModelToolCalls))
	for _, iter := range snapshot.Iterations {
		invocations = append(invocations, iter.ToolInvocations...)
	}
	if len(invocations) < len(tc.ModelToolCalls) {
		t.Fatalf("tool invocations = %#v, want at least %d", invocations, len(tc.ModelToolCalls))
	}
	for _, call := range tc.ModelToolCalls {
		inv, ok := findGoldenToolInvocation(invocations, call.ID)
		if !ok {
			t.Fatalf("missing tool invocation state for call %q; invocations=%#v", call.ID, invocations)
		}
		if inv.ToolName != call.Name {
			t.Fatalf("invocation tool name = %q, want %q", inv.ToolName, call.Name)
		}
		if strings.TrimSpace(inv.ArgumentsHash) == "" {
			t.Fatalf("invocation %q missing arguments hash", call.ID)
		}
		if strings.TrimSpace(inv.ToolSurfaceFingerprint) == "" {
			t.Fatalf("invocation %q missing tool surface fingerprint", call.ID)
		}
		switch {
		case tc.ExpectedFailureKind != "":
			if inv.Status != ToolInvocationFailed {
				t.Fatalf("invocation %q status = %q, want failed", call.ID, inv.Status)
			}
			if inv.FailureKind != tc.ExpectedFailureKind {
				t.Fatalf("invocation %q failure kind = %q, want %q", call.ID, inv.FailureKind, tc.ExpectedFailureKind)
			}
		case tc.ExpectedStatus == "blocked":
			if inv.Status != ToolInvocationBlocked {
				t.Fatalf("invocation %q status = %q, want blocked", call.ID, inv.Status)
			}
		default:
			if inv.Status != ToolInvocationCompleted {
				t.Fatalf("invocation %q status = %q, want completed", call.ID, inv.Status)
			}
			if inv.CompletedAt == nil {
				t.Fatalf("invocation %q missing completedAt", call.ID)
			}
		}
	}
	assertGoldenExpectedAttempts(t, tc, invocations)
}

func assertGoldenExpectedAttempts(t *testing.T, tc aiChatHarnessGoldenCase, invocations []ToolInvocationState) {
	t.Helper()
	for _, expected := range tc.ExpectedAttempts {
		inv, ok := findGoldenToolInvocation(invocations, expected.ToolCallID)
		if !ok {
			t.Fatalf("expected attempt for missing invocation %q", expected.ToolCallID)
		}
		found := false
		for _, attempt := range inv.Attempts {
			if string(attempt.Action) != expected.Action || string(attempt.Outcome) != expected.Outcome {
				continue
			}
			if expected.TriggerFailureKind != "" && attempt.TriggerFailureKind != expected.TriggerFailureKind {
				continue
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("invocation %q attempts = %#v, want action=%q outcome=%q failure=%q", expected.ToolCallID, inv.Attempts, expected.Action, expected.Outcome, expected.TriggerFailureKind)
		}
	}
}

func findGoldenToolInvocation(invocations []ToolInvocationState, toolCallID string) (ToolInvocationState, bool) {
	for _, invocation := range invocations {
		if invocation.ToolCallID == toolCallID {
			return invocation, true
		}
	}
	return ToolInvocationState{}, false
}

func failureKindFromToolResult(result ToolResult) string {
	if strings.TrimSpace(result.Content) == "" {
		return ""
	}
	var payload struct {
		Type        string `json:"type"`
		FailureKind string `json:"failureKind"`
	}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		return ""
	}
	if payload.Type != "tool_error" {
		return ""
	}
	return payload.FailureKind
}

func assertCheckpointSequenceMonotonic(t *testing.T, snapshot *TurnSnapshot) {
	t.Helper()
	lastSeq := 0
	for _, iter := range snapshot.Iterations {
		if iter.Checkpoint == nil {
			continue
		}
		if iter.Checkpoint.Sequence < lastSeq {
			t.Fatalf("checkpoint sequence regressed from %d to %d", lastSeq, iter.Checkpoint.Sequence)
		}
		lastSeq = iter.Checkpoint.Sequence
	}
	if snapshot.LatestCheckpoint != nil && snapshot.LatestCheckpoint.Sequence < lastSeq {
		t.Fatalf("latest checkpoint sequence = %d, want >= %d", snapshot.LatestCheckpoint.Sequence, lastSeq)
	}
}

func goldenStateObserved(state string, snapshot *TurnSnapshot, events []LifecycleEvent) bool {
	state = strings.TrimSpace(state)
	switch state {
	case "completed":
		return snapshot.Lifecycle == TurnLifecycleCompleted || hasEventType(events, EventToolCompleted)
	case "failed":
		if snapshot.Lifecycle == TurnLifecycleFailed || hasEventType(events, EventToolFailed) {
			return true
		}
		for _, iter := range snapshot.Iterations {
			for _, result := range iter.ToolResults {
				if result.Error != "" {
					return true
				}
			}
		}
	case "blocked":
		return snapshot.Lifecycle == TurnLifecycleSuspended || hasEventType(events, EventApprovalNeeded)
	case "running":
		return hasEventType(events, EventToolStarted) || len(snapshot.Iterations) > 0
	}
	return false
}

func goldenTargetBinding(snapshot *TurnSnapshot) string {
	if snapshot == nil {
		return "none"
	}
	if binding := strings.TrimSpace(snapshot.Metadata["aiops.target.binding"]); binding != "" {
		return binding
	}
	if strings.TrimSpace(snapshot.Metadata["aiops.target.hostId"]) != "" {
		return "host"
	}
	if strings.TrimSpace(snapshot.Metadata["hostId"]) != "" {
		return "host"
	}
	return "none"
}

func goldenToolSurfaceHasTool(snapshot *TurnSnapshot, name string) bool {
	name = strings.TrimSpace(name)
	if snapshot == nil || snapshot.ToolSurfaceSnapshot == nil || name == "" {
		return false
	}
	return containsString(snapshot.ToolSurfaceSnapshot.ToolNames, name)
}

func goldenHiddenToolObserved(snapshot *TurnSnapshot, expected aiChatExpectedHiddenTool) bool {
	if snapshot == nil || snapshot.ToolSurfaceSnapshot == nil || snapshot.ToolSurfaceSnapshot.PolicySnapshot == nil {
		return false
	}
	for _, hidden := range snapshot.ToolSurfaceSnapshot.PolicySnapshot.HiddenTools {
		if strings.TrimSpace(hidden.Name) != strings.TrimSpace(expected.Name) {
			continue
		}
		if strings.TrimSpace(expected.Reason) == "" || strings.Contains(strings.TrimSpace(hidden.Reason), strings.TrimSpace(expected.Reason)) {
			return true
		}
	}
	return false
}

func goldenToolCallObserved(snapshot *TurnSnapshot, name string) bool {
	name = strings.TrimSpace(name)
	if snapshot == nil || name == "" {
		return false
	}
	for _, iter := range snapshot.Iterations {
		for _, call := range iter.ToolCalls {
			if strings.TrimSpace(call.Name) == name || strings.TrimSpace(call.ID) == name {
				return true
			}
		}
	}
	return false
}

func goldenFinalContractStatus(snapshot *TurnSnapshot) string {
	if snapshot == nil {
		return ""
	}
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		if item.Type != agentstate.TurnItemTypeFinalResponse && item.Type != agentstate.TurnItemTypeAssistantMessage {
			continue
		}
		var payload struct {
			FinalContract FinalContract `json:"finalContract"`
		}
		if len(item.Payload.Data) > 0 && json.Unmarshal(item.Payload.Data, &payload) == nil && payload.FinalContract.Status != "" {
			return string(payload.FinalContract.Status)
		}
	}
	return ""
}

func goldenPendingApprovalHasRollbackContract(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, approval := range snapshot.PendingApprovals {
		if approval.Mutating && approval.RollbackContract.SchemaVersion == ActionRollbackContractSchemaVersion {
			return true
		}
	}
	return false
}

func assertGoldenTimelineOrder(t *testing.T, snapshot *TurnSnapshot, expected []string) {
	t.Helper()
	if len(expected) == 0 {
		return
	}
	var actual []string
	for _, item := range snapshot.AgentItems {
		actual = append(actual, string(item.Type))
	}
	cursor := 0
	for _, want := range expected {
		found := false
		for cursor < len(actual) {
			if actual[cursor] == want {
				found = true
				cursor++
				break
			}
			cursor++
		}
		if !found {
			t.Fatalf("timeline order %v missing %q in actual %v", expected, want, actual)
		}
	}
}

func hasEventType(events []LifecycleEvent, typ EventType) bool {
	for _, event := range events {
		if event.Type == typ {
			return true
		}
	}
	return false
}

func eventTypes(events []LifecycleEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, string(event.Type))
	}
	return out
}

func TestRuntimeProviderResponseAdapterPreservesToolCalls(t *testing.T) {
	msg := runtimeMessageFromProviderResponse(modelrouter.ProviderResponse{
		Output: "assistant",
		ToolCalls: []promptinput.ModelInputToolCall{{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"x"}`),
		}},
	})
	if msg.Role != "assistant" || msg.Content != "assistant" || len(msg.ToolCalls) != 1 {
		t.Fatalf("runtime message = %#v, want assistant message with tool call", msg)
	}
	if got := msg.ToolCalls[0]; got.ID != "call-1" || got.Name != "read_file" || string(got.Arguments) != `{"path":"x"}` {
		t.Fatalf("runtime tool call = %#v, want provider-neutral tool call metadata preserved", got)
	}
}

func TestRuntimeProviderResponseAdapterPreservesReasoningContent(t *testing.T) {
	msg := runtimeMessageFromProviderResponse(modelrouter.ProviderResponse{
		Output:           "需要继续检查。",
		ReasoningContent: "先检查工具状态。",
	})
	if msg.Role != "assistant" || msg.Content != "需要继续检查。" {
		t.Fatalf("runtime message = %#v, want assistant content", msg)
	}
	if msg.ReasoningContent != "先检查工具状态。" {
		t.Fatalf("ReasoningContent = %q, want provider reasoning preserved", msg.ReasoningContent)
	}
	items := promptInputMessagesFromRuntime([]Message{msg})
	if len(items) != 1 || items[0].ReasoningContent != "先检查工具状态。" {
		t.Fatalf("prompt input messages = %#v, want reasoning content preserved", items)
	}
}

func TestToolForToolCallMatchesProviderSafeName(t *testing.T) {
	tools := []promptcompiler.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{Name: "coroot.list_services", Description: "List services."},
		},
	}

	toolDef := toolForToolCall(tools, ToolCall{Name: "coroot_list_services"})
	if toolDef == nil {
		t.Fatal("toolForToolCall() = nil, want match by provider-safe name")
	}
	if got := toolDef.Metadata().Name; got != "coroot.list_services" {
		t.Fatalf("matched tool name = %q, want canonical coroot.list_services", got)
	}
}

func TestToolDispatcherDispatchesProviderSafeName(t *testing.T) {
	emitter := &testMockEventEmitter{}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "coroot.list_services", Description: "List services."},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "services"}, nil
		},
	}
	lookup := assembledToolLookup{byName: map[string]tooling.Tool{}}
	addToolLookupName(lookup.byName, toolDef.Metadata().Name, toolDef)

	result := NewToolDispatcher(lookup, nil, emitter).Dispatch(
		context.Background(),
		"sess-provider-name",
		"turn-provider-name",
		ToolCall{ID: "call-1", Name: "coroot_list_services", Arguments: json.RawMessage(`{}`)},
		SessionTypeHost,
		ModeInspect,
	)

	if result.Error != "" || result.Content != "services" {
		t.Fatalf("Dispatch() = %#v, want successful execution", result)
	}
	if result.Metadata.Name != "coroot.list_services" {
		t.Fatalf("Dispatch() metadata name = %q, want canonical coroot.list_services", result.Metadata.Name)
	}
}

func TestToolLifecyclePayloadBudgetBoundaries(t *testing.T) {
	summary, resultForEvent, preview, rawRef, bytes, truncated := summarizeToolLifecycleResultForEvent("turn-1", "call-1", " first line \nsecond line")
	if summary != "first line" || resultForEvent != "first line \nsecond line" || len(preview) == 0 || rawRef != "" || bytes == 0 || truncated {
		t.Fatalf("small result summary=%q result=%q preview=%s rawRef=%q bytes=%d truncated=%v", summary, resultForEvent, preview, rawRef, bytes, truncated)
	}

	medium := strings.Repeat("x", inlineToolLifecycleResultBytes+10)
	_, resultForEvent, preview, rawRef, _, truncated = summarizeToolLifecycleResultForEvent("turn-1", "call-1", medium)
	if resultForEvent == medium || len(preview) == 0 || rawRef == "" || !truncated {
		t.Fatalf("medium result not summarized correctly: resultLen=%d preview=%d rawRef=%q truncated=%v", len(resultForEvent), len(preview), rawRef, truncated)
	}

	huge := strings.Repeat("x", maxToolLifecyclePayloadBytes+10)
	_, _, preview, rawRef, _, truncated = summarizeToolLifecycleResultForEvent("turn-1", "call-1", huge)
	if len(preview) == 0 || rawRef == "" || !truncated {
		t.Fatalf("huge result preview=%d rawRef=%q truncated=%v, want preview with raw ref", len(preview), rawRef, truncated)
	}
	var previewText string
	if err := json.Unmarshal(preview, &previewText); err != nil {
		t.Fatalf("huge preview decode error = %v", err)
	}
	if len([]byte(previewText)) > inlineToolLifecycleResultBytes+len("...") {
		t.Fatalf("huge preview len = %d, want bounded preview", len([]byte(previewText)))
	}
	if got := rawToolLifecycleRef("", "call-1"); got != "" {
		t.Fatalf("rawToolLifecycleRef with missing turn = %q, want empty", got)
	}
	if got := truncateToolLifecycleBytes("你好世界", len("你好")+1); !strings.HasSuffix(got, "...") {
		t.Fatalf("truncateToolLifecycleBytes unicode result = %q, want ellipsis", got)
	}
}

func TestRunnerCallbackEmitsLifecycleEvents(t *testing.T) {
	emitter := &testMockEventEmitter{}
	cb := NewRunnerCallback("sess-1", "turn-1", emitter)
	cb.OnToolStart("read_file", json.RawMessage(`{"path":"x"}`))
	cb.OnToolComplete("read_file", strings.Repeat("x", inlineToolLifecycleResultBytes+1))
	cb.OnToolFailed("read_file", errors.New("boom"))

	if len(emitter.events) != 3 {
		t.Fatalf("events len = %d, want 3", len(emitter.events))
	}
	if emitter.events[0].Type != EventToolStarted || emitter.events[1].Type != EventToolCompleted || emitter.events[2].Type != EventToolFailed {
		t.Fatalf("events = %#v, want started/completed/failed", emitter.events)
	}
	if !strings.Contains(string(emitter.events[1].Payload), "rawRef") {
		t.Fatalf("completed payload missing rawRef: %s", emitter.events[1].Payload)
	}
}

func TestRecoveryHelpersReturnStructuredErrors(t *testing.T) {
	if msg := RecoverToolExec("panic_tool", func() error { panic("boom") }); !strings.Contains(msg, "panic_tool") || !strings.Contains(msg, "boom") {
		t.Fatalf("RecoverToolExec panic msg = %q", msg)
	}
	if msg := RecoverToolExec("error_tool", func() error { return errors.New("bad input") }); msg != "bad input" {
		t.Fatalf("RecoverToolExec error msg = %q, want bad input", msg)
	}
	if err := SafeExecute(func() error { panic("safe boom") }); err == nil || !strings.Contains(err.Error(), "safe boom") {
		t.Fatalf("SafeExecute panic err = %v, want recovered panic", err)
	}
	if err := SafeExecute(func() error { return errors.New("plain error") }); err == nil || err.Error() != "plain error" {
		t.Fatalf("SafeExecute error = %v, want plain error", err)
	}
}

func TestMiscRuntimeHelpers(t *testing.T) {
	if got := spillContentBytes(&tooling.ResultSpill{Bytes: 12}, "fallback"); got != 12 {
		t.Fatalf("spillContentBytes bytes = %d, want 12", got)
	}
	if got := spillContentBytes(&tooling.ResultSpill{Content: []byte("abc")}, "fallback"); got != 3 {
		t.Fatalf("spillContentBytes content = %d, want 3", got)
	}
	if got := spillContentBytes(nil, "fallback"); got != len("fallback") {
		t.Fatalf("spillContentBytes nil = %d, want fallback len", got)
	}
	if got := externalReferenceLabel(ExternalReference{CardRef: "card-1"}); got != "card-1" {
		t.Fatalf("externalReferenceLabel card = %q", got)
	}
	if got := externalReferenceLabel(ExternalReference{FilePath: "/tmp/file"}); got != "/tmp/file" {
		t.Fatalf("externalReferenceLabel file = %q", got)
	}
	if got := externalReferenceLabel(ExternalReference{ID: "ref-1"}); got != "ref-1" {
		t.Fatalf("externalReferenceLabel id = %q", got)
	}
	if got := externalReferenceLabel(ExternalReference{}); got != "external-reference" {
		t.Fatalf("externalReferenceLabel fallback = %q", got)
	}
	if got := firstNonEmpty("", "  ", "value"); got != "value" {
		t.Fatalf("firstNonEmpty = %q, want value", got)
	}
}

func TestReasoningSummaryKeyAndItemIDFallbacks(t *testing.T) {
	event := modelrouter.ReasoningStreamEvent{ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", SummaryIndex: 2}
	if got := reasoningSummaryKey(event); got != "item-1:2" {
		t.Fatalf("reasoningSummaryKey = %q, want item-1:2", got)
	}
	if got := reasoningItemID(event); got != "item-1" {
		t.Fatalf("reasoningItemID = %q, want item-1", got)
	}
	if got := reasoningItemID(modelrouter.ReasoningStreamEvent{ThreadID: "thread-1", TurnID: "turn-1", SummaryIndex: 3}); got != "turn-1:reasoning:3" {
		t.Fatalf("reasoningItemID fallback = %q", got)
	}
}
