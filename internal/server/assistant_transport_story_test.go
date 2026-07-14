package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

type assistantTransportStory struct {
	Name              string                  `json:"name"`
	Command           map[string]any          `json:"command"`
	Requests          []storyTransportRequest `json:"requests,omitempty"`
	ProviderResponses []storyProviderResponse `json:"providerResponses"`
	ToolOutcomes      []storyToolOutcome      `json:"toolOutcomes"`
	HostManager       *storyHostManager       `json:"hostManager,omitempty"`
	MaxContextTokens  int                     `json:"maxContextTokens,omitempty"`
	SessionType       string                  `json:"sessionType,omitempty"`
	Mode              string                  `json:"mode,omitempty"`
	ContextMaxTokens  int                     `json:"contextMaxTokens,omitempty"`
	SeedMessages      []string                `json:"seedMessages,omitempty"`
	Want              storyTransportAssert    `json:"want"`
}

type storyHostManager struct {
	MissionID string                   `json:"missionId"`
	Children  []storyHostChildScenario `json:"children"`
}

type storyHostChildScenario struct {
	HostID string `json:"hostId"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type storyTransportRequest struct {
	Command     map[string]any `json:"command"`
	Concurrent  bool           `json:"concurrent,omitempty"`
	WaitForTool string         `json:"waitForTool,omitempty"`
}

type storyProviderResponse struct {
	Role      string          `json:"role"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []storyToolCall `json:"toolCalls,omitempty"`
}

type storyToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type storyToolOutcome struct {
	Name                string                    `json:"name"`
	Description         string                    `json:"description,omitempty"`
	InputSchema         json.RawMessage           `json:"inputSchema,omitempty"`
	Content             string                    `json:"content,omitempty"`
	Error               string                    `json:"error,omitempty"`
	Outcome             tooling.ToolResultOutcome `json:"outcome,omitempty"`
	Risk                string                    `json:"risk,omitempty"`
	Mutating            bool                      `json:"mutating,omitempty"`
	RollbackDeclaration *storyRollbackDeclaration `json:"rollbackDeclaration,omitempty"`
	Approval            *storyToolApproval        `json:"approval,omitempty"`
	PostChecks          []string                  `json:"postChecks,omitempty"`
	PermissionScope     string                    `json:"permissionScope,omitempty"`
	BlockUntilCancelled bool                      `json:"blockUntilCancelled,omitempty"`
}

type storyRollbackDeclaration struct {
	Strategy  tooling.ToolRollbackStrategy `json:"strategy"`
	Reference string                       `json:"reference"`
}

type storyToolApproval struct {
	Reason         string `json:"reason,omitempty"`
	Risk           string `json:"risk,omitempty"`
	Source         string `json:"source,omitempty"`
	ExpectedEffect string `json:"expectedEffect,omitempty"`
	Rollback       string `json:"rollback,omitempty"`
	Validation     string `json:"validation,omitempty"`
}

type storyTransportAssert struct {
	ProviderCallCount   *int                     `json:"providerCallCount,omitempty"`
	TurnStatus          string                   `json:"turnStatus"`
	Lifecycle           string                   `json:"lifecycle"`
	Messages            []storyMessage           `json:"messages"`
	ModelVisibleTools   []string                 `json:"modelVisibleTools"`
	ActualTools         []storyToolAssert        `json:"actualTools"`
	ApprovalLifecycle   []string                 `json:"approvalLifecycle"`
	Target              storyTargetAssert        `json:"target"`
	FinalFacts          storyFinalFacts          `json:"finalFacts"`
	TransportProjection storyTransportProjection `json:"transportProjection"`
	Evidence            []string                 `json:"evidence"`
	TraceHashes         storyTraceHashes         `json:"traceHashes"`
	ContextFacts        *storyContextFacts       `json:"contextFacts,omitempty"`
}

type storyMessage struct {
	Phase string `json:"phase"`
	Text  string `json:"text"`
}

type storyToolAssert struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	FailureKind string `json:"failureKind,omitempty"`
}

type storyTargetAssert struct {
	Binding      string   `json:"binding"`
	HostID       string   `json:"hostId,omitempty"`
	ResourceRefs []string `json:"resourceRefs"`
}

type storyFinalFacts struct {
	SchemaVersion         string                                 `json:"schemaVersion,omitempty"`
	Status                string                                 `json:"status,omitempty"`
	Confidence            string                                 `json:"confidence,omitempty"`
	CheckedEvidenceRefs   []string                               `json:"checkedEvidenceRefs"`
	UncheckedRequirements []string                               `json:"uncheckedRequirements"`
	FailedToolImpacts     []appui.AiopsTransportFailedToolImpact `json:"failedToolImpacts"`
	ApprovedActions       []string                               `json:"approvedActions"`
	PerformedActions      []string                               `json:"performedActions"`
	PostChecks            []string                               `json:"postChecks"`
	RequiredPostChecks    []string                               `json:"requiredPostChecks"`
	Limitations           []string                               `json:"limitations"`
}

type storyTransportProjection struct {
	SchemaVersion        string   `json:"schemaVersion"`
	StateStatus          string   `json:"stateStatus"`
	TurnStatus           string   `json:"turnStatus"`
	ProcessKinds         []string `json:"processKinds"`
	ProcessStatuses      []string `json:"processStatuses"`
	TimelineTypes        []string `json:"timelineTypes"`
	FinalStatus          string   `json:"finalStatus,omitempty"`
	PendingApprovalCount int      `json:"pendingApprovalCount"`
	TurnCount            int      `json:"turnCount"`
}

type storyTraceHashes struct {
	PromptFingerprint       map[string]string `json:"promptFingerprint"`
	StablePromptHash        string            `json:"stablePromptHash"`
	StableToolFingerprint   string            `json:"stableToolFingerprint"`
	ToolSurfaceFingerprints []string          `json:"toolSurfaceFingerprints"`
	ToolPolicyHashes        []string          `json:"toolPolicyHashes"`
	GovernanceSnapshot      string            `json:"governanceSnapshot"`
}

type storyContextFacts struct {
	CompactedSegmentCount int      `json:"compactedSegmentCount"`
	GovernanceKinds       []string `json:"governanceKinds"`
}

func TestAssistantTransportStories(t *testing.T) {
	for _, story := range loadAssistantTransportStories(t) {
		story := story
		t.Run(story.Name, func(t *testing.T) {
			result := runAssistantTransportStory(t, story)
			assertAssistantTransportStory(t, story, result)
		})
	}
}

func TestAssistantTransportStoryCorpusCoversP0Contract(t *testing.T) {
	requiredNames := []string{
		"approval_denied",
		"approval_resume",
		"basic_no_tool",
		"cancelled_running_tool",
		"context_compaction_resume",
		"evidence_rca_no_exec",
		"invalid_arguments",
		"multi_host_manager",
		"mutation_missing_rollback",
		"mutation_missing_target",
		"partial_mutation_postcheck_failed",
		"same_session_host_carryover",
		"single_readonly_tool",
		"tool_not_found",
	}
	requiredWantFields := []string{
		"turnStatus",
		"modelVisibleTools",
		"actualTools",
		"approvalLifecycle",
		"target",
		"finalFacts",
		"transportProjection",
		"traceHashes",
	}

	dir := filepath.Join("testdata", "assistant_transport_story")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read story corpus: %v", err)
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var fixture map[string]any
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("decode %s: %v", entry.Name(), err)
		}
		name, _ := fixture["name"].(string)
		seen[name] = true
		want, _ := fixture["want"].(map[string]any)
		for _, field := range requiredWantFields {
			if _, ok := want[field]; !ok {
				t.Errorf("story %q want.%s is required", name, field)
			}
		}
		traceHashes, _ := want["traceHashes"].(map[string]any)
		providerFree, _ := want["providerCallCount"].(float64)
		if providerFree == 0 {
			if _, explicit := want["providerCallCount"]; explicit {
				continue
			}
		}
		promptHashes, _ := traceHashes["promptFingerprint"].(map[string]any)
		for _, hash := range []string{"version", "compilerVersion", "absoluteSystemHash", "roleProfileHash", "stableRuntimeContractHash", "stablePrefixHash", "turnStableHash", "turnPrefixHash", "currentUserInputHash", "developerHash", "protocolStateHash", "runtimePolicyHash", "stableHash", "systemHash", "toolRegistryHash"} {
			if strings.TrimSpace(fmt.Sprint(promptHashes[hash])) == "" {
				t.Errorf("story %q want.traceHashes.promptFingerprint.%s is required", name, hash)
			}
		}
		for _, hash := range []string{"stablePromptHash", "stableToolFingerprint"} {
			if strings.TrimSpace(fmt.Sprint(traceHashes[hash])) == "" {
				t.Errorf("story %q want.traceHashes.%s is required", name, hash)
			}
		}
		for _, hashes := range []string{"toolSurfaceFingerprints", "toolPolicyHashes"} {
			values, _ := traceHashes[hashes].([]any)
			if len(values) == 0 {
				t.Errorf("story %q want.traceHashes.%s must be non-empty", name, hashes)
			}
		}
		if _, ok := traceHashes["governanceSnapshot"]; !ok {
			t.Errorf("story %q want.traceHashes.governanceSnapshot is required", name)
		}
	}
	for _, name := range requiredNames {
		if !seen[name] {
			t.Errorf("required P0 story %q is missing", name)
		}
	}
}

func TestAssistantTransportStoryCorpusRejectsSemanticShells(t *testing.T) {
	stories := map[string]assistantTransportStory{}
	for _, story := range loadAssistantTransportStories(t) {
		stories[story.Name] = story
	}

	approval := stories["approval_resume"]
	approvalMutationReady := false
	for _, outcome := range approval.ToolOutcomes {
		if outcome.Name == "restart_service" && outcome.Mutating && storyRollbackReady(outcome) {
			approvalMutationReady = true
		}
	}
	if !approvalMutationReady || approval.Want.TurnStatus != "completed" ||
		!storyHasToolStatus(approval, "restart_service", string(runtimekernel.ToolInvocationCompleted)) ||
		!storyContains(approval.Want.ApprovalLifecycle, "restart_service:approved") ||
		!storyContains(approval.Want.ApprovalLifecycle, "restart_service:executed") ||
		len(approval.Want.FinalFacts.ApprovedActions) == 0 || len(approval.Want.FinalFacts.PerformedActions) == 0 {
		t.Errorf("approval_resume must prove an approved mutation resumed to a completed invocation/turn: want=%#v", approval.Want)
	}

	partial := stories["partial_mutation_postcheck_failed"]
	partialMutation := false
	for _, outcome := range partial.ToolOutcomes {
		if outcome.Mutating && storyRollbackReady(outcome) && outcome.Approval != nil && len(outcome.PostChecks) > 0 && outcome.Outcome == tooling.ToolResultOutcomePartial {
			partialMutation = true
		}
	}
	if !partialMutation ||
		!storyHasToolStatus(partial, "install_package", string(runtimekernel.ToolInvocationPartial)) ||
		!storyHasToolStatus(partial, "verify_package_state", string(runtimekernel.ToolInvocationFailed)) ||
		len(partial.Want.FinalFacts.FailedToolImpacts) < 2 || len(partial.Want.FinalFacts.RequiredPostChecks) == 0 {
		t.Errorf("partial_mutation_postcheck_failed must prove typed partial mutation plus failed post-check facts: want=%#v", partial.Want)
	}

	readonly := stories["single_readonly_tool"]
	if readonly.Want.Target.Binding != "host" || readonly.Want.Target.HostID == "" ||
		!storyContains(readonly.Want.Target.ResourceRefs, "host:"+readonly.Want.Target.HostID) ||
		len(readonly.ToolOutcomes) != 1 || readonly.ToolOutcomes[0].Mutating ||
		!storyHasToolStatus(readonly, readonly.ToolOutcomes[0].Name, string(runtimekernel.ToolInvocationCompleted)) {
		t.Errorf("single_readonly_tool must be a completed read-only call bound to exactly one host resource: want=%#v outcomes=%#v", readonly.Want, readonly.ToolOutcomes)
	}

	denied := stories["approval_denied"]
	deniedMutation := false
	deniedMutationName := ""
	for _, outcome := range denied.ToolOutcomes {
		if outcome.Mutating && outcome.Approval != nil && strings.TrimSpace(outcome.Approval.Rollback) != "" && storyRollbackReady(outcome) {
			deniedMutation = true
			deniedMutationName = outcome.Name
		}
	}
	if !deniedMutation || denied.Want.Target.Binding != "host" || denied.Want.Target.HostID == "" ||
		!storyContains(denied.Want.Target.ResourceRefs, "host:"+denied.Want.Target.HostID) ||
		!storyContains(denied.Want.ApprovalLifecycle, deniedMutationName+":requested") ||
		!storyContains(denied.Want.ApprovalLifecycle, deniedMutationName+":denied") ||
		len(denied.Want.FinalFacts.PerformedActions) != 0 {
		t.Errorf("approval_denied must prove a single-host mutation was requested, denied, and never performed: want=%#v outcomes=%#v", denied.Want, denied.ToolOutcomes)
	}

	missingRollback := stories["mutation_missing_rollback"]
	missingRollbackMutation := false
	for _, outcome := range missingRollback.ToolOutcomes {
		if outcome.Mutating && !storyRollbackReady(outcome) {
			missingRollbackMutation = true
		}
	}
	if !missingRollbackMutation || missingRollback.Want.ProviderCallCount == nil || *missingRollback.Want.ProviderCallCount != 0 ||
		len(missingRollback.ProviderResponses) != 0 || len(missingRollback.Want.ModelVisibleTools) != 0 || len(missingRollback.Want.ActualTools) != 0 ||
		missingRollback.Want.TurnStatus != "failed" || missingRollback.Want.Lifecycle != "failed" {
		t.Errorf("mutation_missing_rollback must fail closed before provider sampling or dispatch: want=%#v responses=%#v", missingRollback.Want, missingRollback.ProviderResponses)
	}

	evidenceRCA := stories["evidence_rca_no_exec"]
	if len(evidenceRCA.Want.Evidence) == 0 || !storyContains(evidenceRCA.Want.Evidence, "user_provided_evidence") ||
		len(evidenceRCA.Want.ModelVisibleTools) != 0 || len(evidenceRCA.Want.ActualTools) != 0 ||
		evidenceRCA.Want.Target.Binding != "none" || len(evidenceRCA.Want.FinalFacts.PerformedActions) != 0 {
		t.Errorf("evidence_rca_no_exec must cover user evidence while proving zero host/tool execution: want=%#v", evidenceRCA.Want)
	}

	carryover := stories["same_session_host_carryover"]
	if carryover.Want.Target.Binding != "host" || carryover.Want.Target.HostID == "" ||
		!storyContains(carryover.Want.Target.ResourceRefs, "host:"+carryover.Want.Target.HostID) ||
		!storyHasToolStatus(carryover, "read_agent_status", string(runtimekernel.ToolInvocationCompleted)) {
		t.Errorf("same_session_host_carryover must preserve host identity and resourceRefs on the follow-up turn: want=%#v", carryover.Want)
	}

	multi := stories["multi_host_manager"]
	hosts := map[string]bool{}
	toolCalls := 0
	spawnCalls := 0
	waitCalls := 0
	for _, response := range multi.ProviderResponses {
		for _, call := range response.ToolCalls {
			toolCalls++
			if call.Name == "spawn_host_agent" {
				spawnCalls++
			}
			if call.Name == "wait_host_agents" {
				waitCalls++
			}
			var arguments map[string]any
			if json.Unmarshal(call.Arguments, &arguments) == nil {
				if hostID := strings.TrimSpace(fmt.Sprint(arguments["hostId"])); hostID != "" {
					hosts[hostID] = true
				}
				if assignments, ok := arguments["assignments"].([]any); ok {
					for _, raw := range assignments {
						assignment, _ := raw.(map[string]any)
						if hostID := strings.TrimSpace(fmt.Sprint(assignment["hostId"])); hostID != "" {
							hosts[hostID] = true
						}
					}
				}
			}
		}
	}
	if multi.HostManager == nil || multi.HostManager.MissionID != "$runtime" || len(multi.HostManager.Children) < 2 {
		t.Errorf("multi_host_manager must configure at least two children on the real hostops/AgentManager lifecycle")
	}
	if len(multi.ToolOutcomes) != 0 {
		t.Errorf("multi_host_manager must not use synthetic toolOutcomes: %#v", multi.ToolOutcomes)
	}
	for _, forbidden := range []string{"inspect_host_a", "inspect_host_b", "wait_host_tasks"} {
		if storyUsesTool(multi, forbidden) {
			t.Errorf("multi_host_manager must not use semantic-shell tool %q", forbidden)
		}
	}
	if toolCalls < 2 || spawnCalls != 1 || len(hosts) < 2 || waitCalls == 0 || len(multi.ProviderResponses) < 3 || len(multi.Want.ActualTools) < 2 {
		t.Errorf("multi_host_manager must call production spawn_host_agent then wait_host_agents before synthesis for at least two hosts")
	}
	if multi.Want.Target.Binding != "multi_host" {
		t.Errorf("multi_host_manager target binding = %q, want multi_host", multi.Want.Target.Binding)
	}
}

func storyRollbackReady(outcome storyToolOutcome) bool {
	if outcome.RollbackDeclaration == nil {
		return false
	}
	return (tooling.ToolRollbackMetadata{
		Strategy:  outcome.RollbackDeclaration.Strategy,
		Reference: outcome.RollbackDeclaration.Reference,
	}).DeclarativelyReady()
}

func storyContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func storyHasToolStatus(story assistantTransportStory, name, status string) bool {
	for _, tool := range story.Want.ActualTools {
		if tool.Name == name && tool.Status == status {
			return true
		}
	}
	return false
}

func storyUsesTool(story assistantTransportStory, name string) bool {
	for _, response := range story.ProviderResponses {
		for _, call := range response.ToolCalls {
			if call.Name == name {
				return true
			}
		}
	}
	for _, outcome := range story.ToolOutcomes {
		if outcome.Name == name {
			return true
		}
	}
	return false
}

func TestAssistantTransportStoryRunnerUsesCanonicalLifecycleAndEvidenceFactsOnly(t *testing.T) {
	raw, err := os.ReadFile("assistant_transport_story_fixture_test.go")
	if err != nil {
		t.Fatalf("read story runner: %v", err)
	}
	source := string(raw)
	for _, forbidden := range []string{"observeStoryApprovalDecision("} {
		if strings.Contains(source, forbidden) {
			t.Errorf("story runner must not synthesize lifecycle from outgoing command: found %q", forbidden)
		}
	}
	evidenceStart := strings.Index(source, "func projectStoryEvidence(")
	if evidenceStart < 0 {
		t.Fatal("projectStoryEvidence source start is missing")
	}
	evidenceEnd := strings.Index(source[evidenceStart:], "\nfunc projectStoryTraceHashes(")
	if evidenceEnd < 0 {
		t.Fatal("projectStoryEvidence source boundaries are missing")
	}
	evidenceSource := source[evidenceStart : evidenceStart+evidenceEnd]
	if strings.Contains(evidenceSource, "Final.CheckedEvidenceRefs") {
		t.Error("story evidence projection must not self-prove from final checkedEvidenceRefs")
	}
}

func TestAssistantTransportStoryAccumulatorSupportsRootObjectAndArrayPaths(t *testing.T) {
	var state any = map[string]any{"stale": true}
	var err error
	state, err = applyAssistantTransportStoryOpValue(state, assistantTransportStreamOpSet, nil, map[string]any{"items": []any{}})
	if err != nil {
		t.Fatalf("root set: %v", err)
	}
	state, err = applyAssistantTransportStoryOpValue(state, assistantTransportStreamOpSet, []any{"items", 0}, map[string]any{})
	if err != nil {
		t.Fatalf("array insert: %v", err)
	}
	state, err = applyAssistantTransportStoryOpValue(state, assistantTransportStreamOpSet, []any{"items", "0", "name"}, "first")
	if err != nil {
		t.Fatalf("nested array set: %v", err)
	}
	want := map[string]any{"items": []any{map[string]any{"name": "first"}}}
	if !reflect.DeepEqual(state, want) {
		t.Fatalf("state = %#v, want %#v", state, want)
	}
}

func TestAssistantTransportStoryAccumulatorRejectsInvalidPathsAndAppendTargets(t *testing.T) {
	tests := []struct {
		name  string
		state any
		type_ string
		path  []any
		value any
	}{
		{name: "primitive intermediate", state: map[string]any{"turns": "invalid"}, type_: assistantTransportStreamOpSet, path: []any{"turns", "turn-1"}, value: map[string]any{}},
		{name: "array index type", state: map[string]any{"items": []any{}}, type_: assistantTransportStreamOpSet, path: []any{"items", "nope"}, value: "x"},
		{name: "array index out of bounds", state: map[string]any{"items": []any{}}, type_: assistantTransportStreamOpSet, path: []any{"items", 2}, value: "x"},
		{name: "append target not string", state: map[string]any{"message": 7}, type_: assistantTransportStreamOpAppendText, path: []any{"message"}, value: "x"},
		{name: "append value not string", state: map[string]any{"message": "ok"}, type_: assistantTransportStreamOpAppendText, path: []any{"message"}, value: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, _ := json.Marshal(tt.state)
			got, err := applyAssistantTransportStoryOpValue(tt.state, tt.type_, tt.path, tt.value)
			if err == nil {
				t.Fatalf("apply op returned nil error for state=%#v path=%#v", tt.state, tt.path)
			}
			after, _ := json.Marshal(got)
			if string(after) != string(before) {
				t.Fatalf("failed op mutated state: before=%s after=%s", before, after)
			}
		})
	}
}

func TestAssistantTransportStoryAccumulatorRejectsMissingOrNullPath(t *testing.T) {
	for _, frame := range []string{
		`aui-state:[{"type":"set","value":{"status":"idle"}}]`,
		`aui-state:[{"type":"set","path":null,"value":{"status":"idle"}}]`,
	} {
		if _, err := applyAssistantTransportStoryFrame(map[string]any{"status": "working"}, frame); err == nil {
			t.Fatalf("apply frame error = nil, want malformed path rejection: %s", frame)
		}
	}
}

func TestNormalizeAssistantTransportStoryJSONPreservesFactIdentifiersAndPayloads(t *testing.T) {
	turnID := "turn-runtime-123"
	state := map[string]any{
		"currentTurnId": turnID,
		"turnOrder":     []any{turnID},
		"updatedAt":     "2026-07-12T00:00:00Z",
		"turns": map[string]any{
			turnID: map[string]any{
				"id":        turnID,
				"updatedAt": "2026-07-12T00:00:01Z",
				"process": []any{map[string]any{
					"id":           "block-" + turnID,
					"toolCallId":   "call-fact-" + turnID,
					"approvalId":   "approval-fact-" + turnID,
					"evidenceRefs": []any{"evidence-fact-" + turnID},
					"payload":      map[string]any{"source": "payload-fact-" + turnID},
					"metadata":     map[string]any{"source": "metadata-fact-" + turnID},
				}},
			},
		},
		"artifacts": map[string]any{
			"artifact-fact-" + turnID: map[string]any{"preview": "artifact-payload-" + turnID},
		},
	}
	normalizeAssistantTransportStoryJSON(state, turnID)
	turns := state["turns"].(map[string]any)
	turn := turns["<turn-id>"].(map[string]any)
	block := turn["process"].([]any)[0].(map[string]any)
	if state["currentTurnId"] != "<turn-id>" || state["updatedAt"] != "<timestamp>" || turn["id"] != "<turn-id>" || turn["updatedAt"] != "<timestamp>" || block["id"] != "block-<turn-id>" {
		t.Fatalf("runtime identity/time normalization incomplete: %#v", state)
	}
	if block["toolCallId"] != "call-fact-"+turnID || block["approvalId"] != "approval-fact-"+turnID || block["evidenceRefs"].([]any)[0] != "evidence-fact-"+turnID {
		t.Fatalf("fact identifiers were normalized: %#v", block)
	}
	if block["payload"].(map[string]any)["source"] != "payload-fact-"+turnID || block["metadata"].(map[string]any)["source"] != "metadata-fact-"+turnID {
		t.Fatalf("payload or metadata facts were normalized: %#v", block)
	}
	artifacts := state["artifacts"].(map[string]any)
	if artifacts["artifact-fact-"+turnID].(map[string]any)["preview"] != "artifact-payload-"+turnID {
		t.Fatalf("artifact facts were normalized: %#v", artifacts)
	}
}

func TestAssistantTransportStoryProviderRejectsUnusedResponses(t *testing.T) {
	provider := newStoryProvider(t, []storyProviderResponse{
		{Role: "assistant", Content: "first"},
		{Role: "assistant", Content: "unused"},
	})
	if _, err := provider.Generate(context.Background(), nil); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := provider.assertExhausted(); err == nil {
		t.Fatal("assertExhausted() error = nil, want unused scripted response error")
	}
}

func TestAssistantTransportStoryHTTPTimeoutPreservesPartialDiagnostics(t *testing.T) {
	initial := appui.NewAiopsTransportState("story-session-timeout", "story-thread-timeout")
	sessions := runtimekernel.NewSessionManager()
	session := sessions.GetOrCreate(initial.SessionID, runtimekernel.SessionTypeWorkspace, runtimekernel.ModeChat)
	session.CurrentTurn = &runtimekernel.TurnSnapshot{
		ID:        "turn-timeout",
		SessionID: initial.SessionID,
		Iterations: []runtimekernel.IterationState{{
			ModelInputTraceFile: "trace-timeout.json",
		}},
	}
	sessions.Update(session)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`aui-state:[{"type":"set","path":["status"],"value":"working"}]` + "\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result, err := executeAssistantTransportStoryHTTP(ctx, &http.Client{Timeout: time.Second}, server.URL, initial, []byte(`{}`), sessions)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("execute error = %v, want context deadline exceeded", err)
	}
	if result.State.Status != appui.AiopsTransportStatusWorking || result.Snapshot == nil || result.TraceRef != "trace-timeout.json" || !strings.Contains(result.RawTransport, "aui-state:") {
		t.Fatalf("partial result = %#v, want accumulated state/snapshot/trace/raw transport", result)
	}
	details := formatAssistantTransportStoryFailure(assistantTransportStory{Name: "timeout", Command: map[string]any{"type": "add-message"}}, result, err)
	for _, want := range []string{"command=", "latest transport state=", "turn snapshot=", "trace ref=trace-timeout.json"} {
		if !strings.Contains(details, want) {
			t.Fatalf("diagnostic missing %q:\n%s", want, details)
		}
	}
}

func TestAssistantTransportStoryHTTPFailuresPreserveUnifiedDiagnostics(t *testing.T) {
	stateLine := `aui-state:[{"type":"set","path":["status"],"value":"working"}]` + "\n"
	readBoom := errors.New("story body read failed")
	tests := []struct {
		name       string
		client     *http.Client
		wantStatus appui.AiopsTransportStatus
	}{
		{
			name: "request",
			client: &http.Client{Transport: assistantTransportStoryRoundTripper(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("story request failed")
			})},
			wantStatus: appui.AiopsTransportStatusIdle,
		},
		{
			name: "read",
			client: &http.Client{Transport: assistantTransportStoryRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       &assistantTransportStoryReadErrorBody{data: []byte(stateLine), err: readBoom},
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})},
			wantStatus: appui.AiopsTransportStatusWorking,
		},
		{
			name: "status",
			client: &http.Client{Transport: assistantTransportStoryRoundTripper(func(request *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       io.NopCloser(strings.NewReader(stateLine)),
					Header:     make(http.Header),
					Request:    request,
				}, nil
			})},
			wantStatus: appui.AiopsTransportStatusWorking,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initial := appui.NewAiopsTransportState("story-session-"+tt.name, "story-thread-"+tt.name)
			sessions := runtimekernel.NewSessionManager()
			session := sessions.GetOrCreate(initial.SessionID, runtimekernel.SessionTypeWorkspace, runtimekernel.ModeChat)
			session.CurrentTurn = &runtimekernel.TurnSnapshot{ID: "turn-" + tt.name, SessionID: initial.SessionID, Iterations: []runtimekernel.IterationState{{ModelInputTraceFile: "trace-" + tt.name + ".json"}}}
			sessions.Update(session)

			result, err := executeAssistantTransportStoryHTTP(context.Background(), tt.client, "http://story.invalid", initial, []byte(`{}`), sessions)
			if err == nil {
				t.Fatal("execute error = nil, want failure")
			}
			if result.State.Status != tt.wantStatus || result.Snapshot == nil || result.TraceRef != "trace-"+tt.name+".json" {
				t.Fatalf("partial result = %#v, want status/snapshot/trace", result)
			}
			details := formatAssistantTransportStoryFailure(assistantTransportStory{Name: tt.name, Command: map[string]any{"type": "add-message"}}, result, err)
			for _, want := range []string{"command=", "latest transport state=", "turn snapshot=", "trace ref=trace-" + tt.name + ".json"} {
				if !strings.Contains(details, want) {
					t.Fatalf("diagnostic missing %q:\n%s", want, details)
				}
			}
		})
	}
}

type assistantTransportStoryRoundTripper func(*http.Request) (*http.Response, error)

func (f assistantTransportStoryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type assistantTransportStoryReadErrorBody struct {
	data []byte
	err  error
}

func (b *assistantTransportStoryReadErrorBody) Read(target []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, b.err
	}
	count := copy(target, b.data)
	b.data = b.data[count:]
	return count, nil
}

func (*assistantTransportStoryReadErrorBody) Close() error { return nil }
