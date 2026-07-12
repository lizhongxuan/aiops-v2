package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
)

type assistantTransportStoryResult struct {
	State           appui.AiopsTransportState
	NormalizedState map[string]any
	Snapshot        *runtimekernel.TurnSnapshot
	TraceRef        string
	RawTransport    string
}

const (
	assistantTransportStoryRequestTimeout = 5 * time.Second
	assistantTransportStoryClientTimeout  = 6 * time.Second
)

func loadAssistantTransportStories(t *testing.T) []assistantTransportStory {
	t.Helper()
	dir := filepath.Join("testdata", "assistant_transport_story")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read assistant transport stories from %s: %v", dir, err)
	}
	stories := make([]assistantTransportStory, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read assistant transport story %s: %v", path, err)
		}
		var story assistantTransportStory
		if err := json.Unmarshal(raw, &story); err != nil {
			t.Fatalf("decode assistant transport story %s: %v", path, err)
		}
		if strings.TrimSpace(story.Name) == "" || len(storyTransportRequests(story)) == 0 {
			t.Fatalf("assistant transport story %s must define name and at least one request command", path)
		}
		if len(story.ProviderResponses) == 0 {
			t.Fatalf("assistant transport story %s must define providerResponses", path)
		}
		stories = append(stories, story)
	}
	sort.Slice(stories, func(i, j int) bool { return stories[i].Name < stories[j].Name })
	if len(stories) == 0 {
		t.Fatalf("no assistant transport stories found in %s", dir)
	}
	return stories
}

func runAssistantTransportStory(t *testing.T, story assistantTransportStory) assistantTransportStoryResult {
	t.Helper()

	registry := tooling.NewRegistry()
	toolControls := newStoryToolControls(story.ToolOutcomes)
	for _, outcome := range story.ToolOutcomes {
		outcome := outcome
		if strings.TrimSpace(outcome.Name) == "" {
			t.Fatalf("story %q has tool outcome without name", story.Name)
		}
		schemaData := outcome.InputSchema
		if len(schemaData) == 0 {
			schemaData = json.RawMessage(`{"type":"object","additionalProperties":true}`)
		}
		toolDef := &tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:             outcome.Name,
				Description:      firstStoryValue(outcome.Description, "deterministic story tool "+outcome.Name),
				Origin:           tooling.ToolOriginBuiltin,
				Layer:            tooling.ToolLayerCore,
				AlwaysLoad:       true,
				RiskLevel:        tooling.ToolRiskLevel(firstStoryValue(outcome.Risk, string(tooling.ToolRiskLow))),
				Mutating:         outcome.Mutating,
				RequiresApproval: outcome.Approval != nil,
				Discovery:        tooling.ToolDiscoveryMetadata{PermissionScope: outcome.PermissionScope},
			},
			InputSchemaData: schemaData,
			ReadOnlyFunc:    func(json.RawMessage) bool { return !outcome.Mutating },
			CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
				if outcome.Approval == nil {
					return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
				}
				return tooling.PermissionDecision{
					Action: tooling.PermissionActionNeedApproval,
					Reason: firstStoryValue(outcome.Approval.Reason, "story approval required"),
					Approval: &tooling.PermissionApprovalPayload{
						Reason:         firstStoryValue(outcome.Approval.Reason, "story approval required"),
						Risk:           firstStoryValue(outcome.Approval.Risk, outcome.Risk, string(tooling.ToolRiskHigh)),
						Source:         firstStoryValue(outcome.Approval.Source, "assistant_transport_story"),
						ExpectedEffect: outcome.Approval.ExpectedEffect,
						Rollback:       outcome.Approval.Rollback,
						Validation:     outcome.Approval.Validation,
					},
				}
			},
			ExecuteFunc: func(ctx context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
				toolControls.markStarted(outcome.Name)
				if outcome.BlockUntilCancelled {
					<-ctx.Done()
					return tooling.ToolResult{Error: ctx.Err().Error()}, ctx.Err()
				}
				if outcome.Error != "" {
					return tooling.ToolResult{Error: outcome.Error}, errors.New(outcome.Error)
				}
				return tooling.ToolResult{Content: outcome.Content}, nil
			},
		}
		if outcome.Mutating {
			toolDef.Meta.Layer = tooling.ToolLayerMutation
			toolDef.Meta.ResourceLocks = []tooling.ToolResourceLockKey{{ResourceType: "story_resource", ResourceID: storySlug(outcome.Name), OperationKind: "mutation"}}
			toolDef.Meta.Idempotency = tooling.ToolIdempotencyMetadata{Strategy: tooling.ToolIdempotencyStrategyArgumentsHash, PostCheckRefs: append([]string(nil), outcome.PostChecks...)}
		}
		if outcome.Approval != nil {
			toolDef.Meta.Discovery.PermissionScope = "argument_scoped"
		}
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("register story tool %q: %v", outcome.Name, err)
		}
	}

	provider := newStoryProvider(t, story.ProviderResponses)
	router := modelrouter.NewRouter("story", map[string]modelrouter.ChatModel{"story": provider}, nil)
	router.SetProviderConfigResolver(storyProviderConfigResolver{maxContextTokens: story.MaxContextTokens})
	sessions := runtimekernel.NewSessionManager()
	traceRoot := t.TempDir()
	kernel := runtimekernel.NewRuntimeKernel(runtimekernel.RuntimeKernelConfig{
		ToolSource:  storyToolSource{registry: registry},
		Compiler:    promptcompiler.NewCompiler(),
		Policy:      &policyengine.Engine{ModePolicy: policyengine.NewDefaultModePolicies(), CompletionPolicy: &policyengine.DefaultCompletionEvaluator{}},
		Projector:   storyEventEmitter{},
		ModelRouter: router,
		Sessions:    sessions,
		DebugConfig: func(context.Context) runtimekernel.RuntimeDebugConfig {
			return runtimekernel.RuntimeDebugConfig{ModelInputTrace: true, ModelInputTraceRoot: traceRoot}
		},
	})

	sessionID := "story-session-" + storySlug(story.Name)
	threadID := "story-thread-" + storySlug(story.Name)
	if strings.TrimSpace(story.SessionType) != "" || strings.TrimSpace(story.Mode) != "" || story.ContextMaxTokens > 0 || len(story.SeedMessages) > 0 {
		sessionType := runtimekernel.SessionType(firstStoryValue(story.SessionType, string(runtimekernel.SessionTypeWorkspace)))
		mode := runtimekernel.Mode(firstStoryValue(story.Mode, string(runtimekernel.ModeChat)))
		session := sessions.GetOrCreate(sessionID, sessionType, mode)
		if story.ContextMaxTokens > 0 {
			session.Context.MaxTokens = story.ContextMaxTokens
		}
		for index, content := range story.SeedMessages {
			role := "user"
			if index%2 == 1 {
				role = "assistant"
			}
			session.Messages = append(session.Messages, runtimekernel.Message{ID: fmt.Sprintf("seed-%d", index), Role: role, Content: content, Timestamp: time.Unix(int64(index+1), 0).UTC()})
		}
		sessions.Update(session)
	}
	initial := appui.NewAiopsTransportState(sessionID, threadID)
	initial.UpdatedAt = "2000-01-01T00:00:00Z"
	httpServer := httptest.NewServer(NewHTTPServer(appui.NewServices(kernel, sessions)).Handler())
	defer httpServer.Close()
	result := runAssistantTransportStoryRequests(t, story, httpServer.URL, initial, threadID, sessions, toolControls)
	if err := provider.assertExhausted(); err != nil {
		failAssistantTransportStory(t, story, result, "deterministic provider script mismatch: %v", err)
		return result
	}
	return result
}

func storyTransportRequests(story assistantTransportStory) []storyTransportRequest {
	if len(story.Requests) > 0 {
		return append([]storyTransportRequest(nil), story.Requests...)
	}
	if len(story.Command) > 0 {
		return []storyTransportRequest{{Command: story.Command}}
	}
	return nil
}

type storyAsyncHTTPResult struct {
	result assistantTransportStoryResult
	err    error
}

func runAssistantTransportStoryRequests(t *testing.T, story assistantTransportStory, baseURL string, initial appui.AiopsTransportState, threadID string, sessions *runtimekernel.SessionManager, controls *storyToolControls) assistantTransportStoryResult {
	t.Helper()
	state := initial
	combined := assistantTransportStoryResult{State: initial}
	client := &http.Client{Timeout: assistantTransportStoryClientTimeout}
	var pending <-chan storyAsyncHTTPResult
	for _, request := range storyTransportRequests(story) {
		command := resolveStoryTransportCommand(t, request.Command, state, sessions)
		requestState := hydrateStoryTransportState(t, state, sessions)
		payload, err := json.Marshal(map[string]any{"state": requestState, "threadId": threadID, "commands": []map[string]any{command}})
		if err != nil {
			failAssistantTransportStory(t, story, combined, "marshal request: %v", err)
		}
		if request.Concurrent {
			ch := make(chan storyAsyncHTTPResult, 1)
			go func(requestState appui.AiopsTransportState, payload []byte) {
				ctx, cancel := context.WithTimeout(context.Background(), assistantTransportStoryRequestTimeout)
				defer cancel()
				result, err := executeAssistantTransportStoryHTTP(ctx, client, baseURL, requestState, payload, sessions)
				ch <- storyAsyncHTTPResult{result: result, err: err}
			}(requestState, payload)
			pending = ch
			controls.waitStarted(t, request.WaitForTool)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), assistantTransportStoryRequestTimeout)
		stepResult, requestErr := executeAssistantTransportStoryHTTP(ctx, client, baseURL, requestState, payload, sessions)
		cancel()
		if requestErr != nil {
			failAssistantTransportStory(t, story, stepResult, "AssistantTransport request failed: %v", requestErr)
		}
		for refreshAttempt := 0; refreshAttempt < 3 && storyResultNeedsProjectionRefresh(stepResult); refreshAttempt++ {
			refreshPayload, marshalErr := json.Marshal(map[string]any{"state": stepResult.State, "threadId": threadID, "commands": []map[string]any{}})
			if marshalErr != nil {
				failAssistantTransportStory(t, story, stepResult, "marshal projection refresh: %v", marshalErr)
			}
			refreshCtx, refreshCancel := context.WithTimeout(context.Background(), assistantTransportStoryRequestTimeout)
			refreshed, refreshErr := executeAssistantTransportStoryHTTP(refreshCtx, client, baseURL, stepResult.State, refreshPayload, sessions)
			refreshCancel()
			if refreshErr != nil {
				failAssistantTransportStory(t, story, refreshed, "AssistantTransport projection refresh failed: %v", refreshErr)
			}
			refreshed.RawTransport = stepResult.RawTransport + refreshed.RawTransport
			stepResult = refreshed
		}
		combined = mergeStoryTransportResult(combined, stepResult)
		state = stepResult.State
	}
	if pending != nil {
		async := <-pending
		if async.err != nil {
			failAssistantTransportStory(t, story, async.result, "concurrent AssistantTransport request failed: %v", async.err)
		}
		combined.RawTransport = async.result.RawTransport + combined.RawTransport
	}
	return combined
}

func storyResultNeedsProjectionRefresh(result assistantTransportStoryResult) bool {
	if result.Snapshot == nil || strings.TrimSpace(result.Snapshot.FinalOutput) == "" {
		return false
	}
	turn := result.State.Turns[result.State.CurrentTurnID]
	if turn.Final == nil {
		return true
	}
	snapshotHasFinalResponse := false
	for _, item := range result.Snapshot.AgentItems {
		if string(item.Type) == "final_response" {
			snapshotHasFinalResponse = true
			break
		}
	}
	if !snapshotHasFinalResponse {
		return false
	}
	for _, item := range turn.Timeline {
		if item.Type == "final_response" {
			return false
		}
	}
	return true
}

func mergeStoryTransportResult(acc, next assistantTransportStoryResult) assistantTransportStoryResult {
	raw := acc.RawTransport + next.RawTransport
	acc = next
	acc.RawTransport = raw
	return acc
}

func assertAssistantTransportStory(t *testing.T, story assistantTransportStory, result assistantTransportStoryResult) {
	t.Helper()
	if result.Snapshot == nil {
		failAssistantTransportStory(t, story, result, "runtime turn snapshot is missing")
	}
	turn, ok := result.State.Turns[result.State.CurrentTurnID]
	if !ok {
		failAssistantTransportStory(t, story, result, "current transport turn %q is missing", result.State.CurrentTurnID)
	}
	approvalLifecycle, approvalErr := projectStoryApprovalLifecycle(result.Snapshot)
	if approvalErr != nil {
		failAssistantTransportStory(t, story, result, "canonical approval lifecycle is invalid: %v", approvalErr)
	}
	got := storyTransportAssert{
		TurnStatus:          string(turn.Status),
		Lifecycle:           string(result.Snapshot.Lifecycle),
		Messages:            projectStoryMessages(turn),
		ModelVisibleTools:   projectStoryModelVisibleTools(result.Snapshot),
		ActualTools:         projectStoryActualTools(result.Snapshot),
		ApprovalLifecycle:   approvalLifecycle,
		Target:              projectStoryTarget(result.Snapshot),
		FinalFacts:          projectStoryFinalFacts(turn),
		TransportProjection: projectStoryTransportProjection(result.State, turn),
		Evidence:            projectStoryEvidence(result.Snapshot),
		TraceHashes:         projectStoryTraceHashes(result.Snapshot),
	}
	if story.Want.ContextFacts != nil {
		got.ContextFacts = projectStoryContextFacts(result.Snapshot)
	}
	normalizeStoryAssert(&got)
	want := story.Want
	normalizeStoryAssert(&want)
	gotJSON, _ := json.MarshalIndent(got, "", "  ")
	wantJSON, _ := json.MarshalIndent(want, "", "  ")
	if !bytes.Equal(gotJSON, wantJSON) {
		failAssistantTransportStory(t, story, result, "assertion mismatch\nwant=%s\n got=%s", wantJSON, gotJSON)
	}
	if result.TraceRef == "" {
		failAssistantTransportStory(t, story, result, "model input trace ref is missing")
	}
}

func projectStoryContextFacts(snapshot *runtimekernel.TurnSnapshot) *storyContextFacts {
	facts := &storyContextFacts{GovernanceKinds: []string{}}
	if snapshot == nil {
		return facts
	}
	facts.CompactedSegmentCount = len(snapshot.CompactedSegments)
	for _, event := range snapshot.ContextGovernanceEvents {
		facts.GovernanceKinds = append(facts.GovernanceKinds, string(event.Layer)+":"+event.Kind)
	}
	sort.Strings(facts.GovernanceKinds)
	return facts
}

func projectStoryMessages(turn appui.AiopsTransportTurn) []storyMessage {
	messages := make([]storyMessage, 0, len(turn.Process)+1)
	for _, block := range turn.Process {
		if block.Kind == appui.AiopsTransportProcessKindAssistant && strings.TrimSpace(block.Text) != "" {
			messages = append(messages, storyMessage{Phase: firstStoryValue(block.Phase, "commentary"), Text: normalizeStoryMessageText(block.Text)})
		}
	}
	if turn.Final != nil && strings.TrimSpace(turn.Final.Text) != "" {
		messages = append(messages, storyMessage{Phase: "final_answer", Text: normalizeStoryMessageText(turn.Final.Text)})
	}
	return messages
}

func normalizeStoryMessageText(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{") {
		return value
	}
	var payload map[string]any
	if json.Unmarshal([]byte(value), &payload) != nil {
		return value
	}
	if _, ok := payload["approvalId"]; ok {
		payload["approvalId"] = "<approval-id>"
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return value
	}
	return strings.TrimSpace(encoded.String())
}

func projectStoryModelVisibleTools(snapshot *runtimekernel.TurnSnapshot) []string {
	seen := map[string]bool{}
	if snapshot != nil {
		for _, iteration := range snapshot.Iterations {
			for _, name := range iteration.VisibleTools {
				if strings.TrimSpace(name) != "" {
					seen[name] = true
				}
			}
		}
	}
	return sortedStoryKeys(seen)
}

func projectStoryActualTools(snapshot *runtimekernel.TurnSnapshot) []storyToolAssert {
	tools := make([]storyToolAssert, 0)
	if snapshot == nil {
		return tools
	}
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			tools = append(tools, storyToolAssert{
				ID:          invocation.ToolCallID,
				Name:        invocation.ToolName,
				Status:      string(invocation.Status),
				FailureKind: invocation.FailureKind,
			})
		}
	}
	return tools
}

func projectStoryApprovalLifecycle(snapshot *runtimekernel.TurnSnapshot) ([]string, error) {
	if snapshot == nil {
		return []string{}, nil
	}
	type approvalFact struct {
		ApprovalID string `json:"approvalId"`
		ToolCallID string `json:"toolCallId"`
		ToolName   string `json:"toolName"`
		Status     string `json:"status"`
	}
	requested := map[string]approvalFact{}
	approvedCalls := map[string]string{}
	lifecycle := make([]string, 0)
	appendFact := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range lifecycle {
			if existing == value {
				return
			}
		}
		lifecycle = append(lifecycle, value)
	}
	for _, item := range snapshot.AgentItems {
		if item.Type != agentstate.TurnItemTypeApprovalRequested && item.Type != agentstate.TurnItemTypeApprovalDecided {
			continue
		}
		var fact approvalFact
		if err := json.Unmarshal(item.Payload.Data, &fact); err != nil {
			return nil, fmt.Errorf("decode %s item %q: %w", item.Type, item.ID, err)
		}
		fact.ApprovalID = strings.TrimSpace(fact.ApprovalID)
		fact.ToolCallID = strings.TrimSpace(fact.ToolCallID)
		fact.ToolName = strings.TrimSpace(fact.ToolName)
		fact.Status = strings.ToLower(strings.TrimSpace(fact.Status))
		if fact.ApprovalID == "" || fact.ToolCallID == "" || fact.ToolName == "" {
			return nil, fmt.Errorf("%s item %q lacks approval/tool correlation: %#v", item.Type, item.ID, fact)
		}
		switch item.Type {
		case agentstate.TurnItemTypeApprovalRequested:
			requested[fact.ApprovalID] = fact
			appendFact(fact.ToolName + ":requested")
		case agentstate.TurnItemTypeApprovalDecided:
			request, ok := requested[fact.ApprovalID]
			if !ok {
				return nil, fmt.Errorf("approval_decided item %q has no matching approval_requested item for %q", item.ID, fact.ApprovalID)
			}
			if request.ToolCallID != fact.ToolCallID || request.ToolName != fact.ToolName {
				return nil, fmt.Errorf("approval %q correlation changed: requested=%s#%s decided=%s#%s", fact.ApprovalID, request.ToolName, request.ToolCallID, fact.ToolName, fact.ToolCallID)
			}
			if fact.Status != "approved" && fact.Status != "denied" {
				return nil, fmt.Errorf("approval_decided item %q has unsupported status %q", item.ID, fact.Status)
			}
			appendFact(fact.ToolName + ":" + fact.Status)
			if fact.Status == "approved" {
				approvedCalls[fact.ToolCallID] = fact.ToolName
			}
		}
	}
	for _, evidence := range snapshot.PendingEvidence {
		if toolName := strings.TrimSpace(evidence.ToolName); toolName != "" {
			appendFact(toolName + ":evidence_required")
		}
	}
	for _, iteration := range snapshot.Iterations {
		for _, invocation := range iteration.ToolInvocations {
			if invocation.Status == runtimekernel.ToolInvocationCompleted && approvedCalls[invocation.ToolCallID] == invocation.ToolName {
				appendFact(invocation.ToolName + ":executed")
			}
		}
	}
	return lifecycle, nil
}

func projectStoryTarget(snapshot *runtimekernel.TurnSnapshot) storyTargetAssert {
	target := storyTargetAssert{Binding: "none", ResourceRefs: []string{}}
	if snapshot == nil {
		return target
	}
	target.Binding = firstStoryValue(snapshot.Metadata["aiops.target.binding"], "none")
	target.HostID = firstStoryValue(snapshot.Metadata["aiops.target.hostId"], snapshot.Metadata["aiops.target.selectedHostId"], snapshot.Metadata["hostId"])
	for _, value := range strings.Split(snapshot.Metadata["aiops.target.refs"], ",") {
		if value = strings.TrimSpace(value); value != "" {
			target.ResourceRefs = append(target.ResourceRefs, value)
		}
	}
	sort.Strings(target.ResourceRefs)
	return target
}

func projectStoryFinalFacts(turn appui.AiopsTransportTurn) storyFinalFacts {
	facts := storyFinalFacts{}
	if turn.Final != nil {
		facts.SchemaVersion = turn.Final.SchemaVersion
		facts.Status = string(turn.Final.Status)
		facts.Confidence = turn.Final.Confidence
		facts.CheckedEvidenceRefs = append([]string(nil), turn.Final.CheckedEvidenceRefs...)
		facts.UncheckedRequirements = append([]string(nil), turn.Final.UncheckedRequirements...)
		facts.FailedToolImpacts = append([]appui.AiopsTransportFailedToolImpact(nil), turn.Final.FailedToolImpacts...)
		facts.ApprovedActions = append([]string(nil), turn.Final.ApprovedActions...)
		facts.PerformedActions = append([]string(nil), turn.Final.PerformedActions...)
		facts.PostChecks = append([]string(nil), turn.Final.PostChecks...)
		facts.RequiredPostChecks = append([]string(nil), turn.Final.RequiredPostChecks...)
		facts.Limitations = append([]string(nil), turn.Final.Limitations...)
	}
	normalizeStoryFinalFacts(&facts)
	return facts
}

func projectStoryTransportProjection(state appui.AiopsTransportState, turn appui.AiopsTransportTurn) storyTransportProjection {
	projection := storyTransportProjection{
		SchemaVersion:        state.SchemaVersion,
		StateStatus:          string(state.Status),
		TurnStatus:           string(turn.Status),
		ProcessKinds:         []string{},
		ProcessStatuses:      []string{},
		TimelineTypes:        []string{},
		PendingApprovalCount: len(state.PendingApprovals),
		TurnCount:            len(state.TurnOrder),
	}
	for _, block := range turn.Process {
		projection.ProcessKinds = append(projection.ProcessKinds, string(block.Kind))
		projection.ProcessStatuses = append(projection.ProcessStatuses, string(block.Status))
	}
	for _, item := range turn.Timeline {
		projection.TimelineTypes = append(projection.TimelineTypes, item.Type)
	}
	if turn.Final != nil {
		projection.FinalStatus = string(turn.Final.Status)
	}
	return projection
}

func projectStoryEvidence(snapshot *runtimekernel.TurnSnapshot) []string {
	seen := map[string]bool{}
	if snapshot == nil {
		return sortedStoryKeys(seen)
	}
	for _, item := range snapshot.AgentItems {
		var payload struct {
			ID           string   `json:"id"`
			Ref          string   `json:"ref"`
			EvidenceID   string   `json:"evidenceId"`
			EvidenceRefs []string `json:"evidenceRefs"`
		}
		_ = json.Unmarshal(item.Payload.Data, &payload)
		switch item.Type {
		case agentstate.TurnItemTypeEvidence, agentstate.TurnItemTypeEvidenceCollected:
			if item.Payload.Kind == "user_provided" {
				seen["user_provided_evidence"] = true
				continue
			}
			for _, ref := range []string{payload.ID, payload.Ref, payload.EvidenceID} {
				if strings.TrimSpace(ref) != "" {
					seen[strings.TrimSpace(ref)] = true
				}
			}
		case agentstate.TurnItemTypeToolResult:
			for _, ref := range payload.EvidenceRefs {
				if strings.TrimSpace(ref) != "" {
					seen[strings.TrimSpace(ref)] = true
				}
			}
		}
	}
	return sortedStoryKeys(seen)
}

func projectStoryTraceHashes(snapshot *runtimekernel.TurnSnapshot) storyTraceHashes {
	out := storyTraceHashes{PromptFingerprint: map[string]string{}, ToolSurfaceFingerprints: []string{}, ToolPolicyHashes: []string{}}
	if snapshot == nil {
		return out
	}
	out.StablePromptHash = snapshot.StablePromptHash
	out.StableToolFingerprint = snapshot.StableToolFingerprint
	out.GovernanceSnapshot = snapshot.GovernanceSnapshot
	seenSurface := map[string]bool{}
	seenPolicy := map[string]bool{}
	for _, iteration := range snapshot.Iterations {
		for key, value := range iteration.PromptFingerprint {
			if strings.TrimSpace(value) != "" {
				out.PromptFingerprint[key] = value
			}
		}
		if value := strings.TrimSpace(iteration.ToolSurfaceFingerprint); value != "" {
			seenSurface[value] = true
		}
		if iteration.ToolSurfaceSnapshot != nil {
			if value := strings.TrimSpace(iteration.ToolSurfaceSnapshot.PolicySnapshotHash); value != "" {
				seenPolicy[value] = true
			}
		}
	}
	out.ToolSurfaceFingerprints = sortedStoryKeys(seenSurface)
	out.ToolPolicyHashes = sortedStoryKeys(seenPolicy)
	return out
}

func normalizeStoryAssert(assertion *storyTransportAssert) {
	if assertion.Messages == nil {
		assertion.Messages = []storyMessage{}
	}
	if assertion.ModelVisibleTools == nil {
		assertion.ModelVisibleTools = []string{}
	}
	if assertion.ActualTools == nil {
		assertion.ActualTools = []storyToolAssert{}
	}
	if assertion.ApprovalLifecycle == nil {
		assertion.ApprovalLifecycle = []string{}
	}
	if assertion.Evidence == nil {
		assertion.Evidence = []string{}
	}
	if assertion.Target.ResourceRefs == nil {
		assertion.Target.ResourceRefs = []string{}
	}
	normalizeStoryFinalFacts(&assertion.FinalFacts)
	if assertion.TransportProjection.ProcessKinds == nil {
		assertion.TransportProjection.ProcessKinds = []string{}
	}
	if assertion.TransportProjection.ProcessStatuses == nil {
		assertion.TransportProjection.ProcessStatuses = []string{}
	}
	if assertion.TransportProjection.TimelineTypes == nil {
		assertion.TransportProjection.TimelineTypes = []string{}
	}
	if assertion.TraceHashes.PromptFingerprint == nil {
		assertion.TraceHashes.PromptFingerprint = map[string]string{}
	}
	if assertion.TraceHashes.ToolSurfaceFingerprints == nil {
		assertion.TraceHashes.ToolSurfaceFingerprints = []string{}
	}
	if assertion.TraceHashes.ToolPolicyHashes == nil {
		assertion.TraceHashes.ToolPolicyHashes = []string{}
	}
	if assertion.ContextFacts != nil && assertion.ContextFacts.GovernanceKinds == nil {
		assertion.ContextFacts.GovernanceKinds = []string{}
	}
	sort.Strings(assertion.ModelVisibleTools)
	sort.Strings(assertion.ApprovalLifecycle)
	sort.Strings(assertion.Evidence)
}

func normalizeStoryFinalFacts(facts *storyFinalFacts) {
	if facts.CheckedEvidenceRefs == nil {
		facts.CheckedEvidenceRefs = []string{}
	}
	if facts.UncheckedRequirements == nil {
		facts.UncheckedRequirements = []string{}
	}
	if facts.FailedToolImpacts == nil {
		facts.FailedToolImpacts = []appui.AiopsTransportFailedToolImpact{}
	}
	if facts.ApprovedActions == nil {
		facts.ApprovedActions = []string{}
	}
	if facts.PerformedActions == nil {
		facts.PerformedActions = []string{}
	}
	if facts.PostChecks == nil {
		facts.PostChecks = []string{}
	}
	if facts.RequiredPostChecks == nil {
		facts.RequiredPostChecks = []string{}
	}
	if facts.Limitations == nil {
		facts.Limitations = []string{}
	}
}

func executeAssistantTransportStoryHTTP(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	initial appui.AiopsTransportState,
	payload []byte,
	sessions *runtimekernel.SessionManager,
) (assistantTransportStoryResult, error) {
	stateValue, err := assistantTransportStoryStateValue(initial)
	if err != nil {
		return assistantTransportStoryResult{}, fmt.Errorf("encode initial transport state: %w", err)
	}
	var raw bytes.Buffer
	capture := func(cause error) (assistantTransportStoryResult, error) {
		result, captureErr := captureAssistantTransportStoryResult(initial, stateValue, sessions, raw.String())
		if captureErr != nil {
			return result, fmt.Errorf("%w (capture partial story result: %v)", cause, captureErr)
		}
		return result, cause
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/assistant/transport", bytes.NewReader(payload))
	if err != nil {
		return capture(fmt.Errorf("build AssistantTransport request: %w", err))
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/plain")
	response, err := client.Do(request)
	if err != nil {
		return capture(fmt.Errorf("POST /api/v1/assistant/transport: %w", err))
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if line != "" {
			raw.WriteString(line)
			if strings.HasPrefix(strings.TrimSpace(line), "aui-state:") {
				next, applyErr := applyAssistantTransportStoryFrame(stateValue, line)
				if applyErr != nil {
					return capture(fmt.Errorf("apply AssistantTransport state frame: %w", applyErr))
				}
				stateValue = next
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return capture(fmt.Errorf("read AssistantTransport response: %w", readErr))
		}
	}
	result, captureErr := captureAssistantTransportStoryResult(initial, stateValue, sessions, raw.String())
	if captureErr != nil {
		return result, fmt.Errorf("capture AssistantTransport result: %w", captureErr)
	}
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("AssistantTransport status=%d body=%s", response.StatusCode, raw.String())
	}
	return result, nil
}

func assistantTransportStoryStateValue(state appui.AiopsTransportState) (any, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func applyAssistantTransportStoryFrame(state any, line string) (any, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "aui-state:") {
		return state, nil
	}
	var ops []struct {
		Type  string          `json:"type"`
		Path  json.RawMessage `json:"path"`
		Value any             `json:"value"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "aui-state:")), &ops); err != nil {
		return state, fmt.Errorf("decode state ops: %w", err)
	}
	next := state
	for index, op := range ops {
		if len(op.Path) == 0 || bytes.Equal(bytes.TrimSpace(op.Path), []byte("null")) {
			return state, fmt.Errorf("op[%d]: path is required and must be an array", index)
		}
		var path []any
		if err := json.Unmarshal(op.Path, &path); err != nil {
			return state, fmt.Errorf("op[%d]: decode path: %w", index, err)
		}
		updated, err := applyAssistantTransportStoryOpValue(next, op.Type, path, op.Value)
		if err != nil {
			return state, fmt.Errorf("op[%d]: %w", index, err)
		}
		next = updated
	}
	return next, nil
}

func applyAssistantTransportStoryOpValue(state any, opType string, path []any, value any) (any, error) {
	var updater func(any) (any, error)
	switch opType {
	case assistantTransportStreamOpSet:
		updater = func(any) (any, error) { return value, nil }
	case assistantTransportStreamOpAppendText:
		appendValue, ok := value.(string)
		if !ok {
			return state, fmt.Errorf("append-text value must be string, got %T", value)
		}
		updater = func(current any) (any, error) {
			text, ok := current.(string)
			if !ok {
				return nil, fmt.Errorf("expected string at path %v, got %T", path, current)
			}
			return text + appendValue, nil
		}
	default:
		return state, fmt.Errorf("invalid operation type %q", opType)
	}
	return updateAssistantTransportStoryPath(state, path, updater)
}

func updateAssistantTransportStoryPath(state any, path []any, updater func(any) (any, error)) (any, error) {
	if len(path) == 0 {
		return updater(state)
	}
	if state == nil {
		state = map[string]any{}
	}
	switch current := state.(type) {
	case map[string]any:
		key, ok := path[0].(string)
		if !ok {
			return state, fmt.Errorf("expected object key at path %v, got %T", path, path[0])
		}
		child, err := updateAssistantTransportStoryPath(current[key], path[1:], updater)
		if err != nil {
			return state, err
		}
		next := make(map[string]any, len(current)+1)
		for existingKey, existingValue := range current {
			next[existingKey] = existingValue
		}
		next[key] = child
		return next, nil
	case []any:
		index, err := assistantTransportStoryArrayIndex(path[0])
		if err != nil {
			return state, fmt.Errorf("path %v: %w", path, err)
		}
		if index < 0 || index > len(current) {
			return state, fmt.Errorf("array index %d out of bounds for length %d", index, len(current))
		}
		next := append([]any(nil), current...)
		if index == len(next) {
			next = append(next, nil)
		}
		child, err := updateAssistantTransportStoryPath(next[index], path[1:], updater)
		if err != nil {
			return state, err
		}
		next[index] = child
		return next, nil
	default:
		return state, fmt.Errorf("invalid intermediate %T at path %v", state, path)
	}
}

func assistantTransportStoryArrayIndex(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed > float64(math.MaxInt) || typed < float64(math.MinInt) {
			return 0, fmt.Errorf("expected integer array index, got %v", typed)
		}
		return int(typed), nil
	case string:
		index, err := strconv.Atoi(typed)
		if err != nil {
			return 0, fmt.Errorf("expected array index, got %q", typed)
		}
		return index, nil
	default:
		return 0, fmt.Errorf("expected array index, got %T", value)
	}
}

func captureAssistantTransportStoryResult(initial appui.AiopsTransportState, stateValue any, sessions *runtimekernel.SessionManager, raw string) (assistantTransportStoryResult, error) {
	encoded, err := json.Marshal(stateValue)
	if err != nil {
		return assistantTransportStoryResult{RawTransport: raw}, err
	}
	var state appui.AiopsTransportState
	if err := json.Unmarshal(encoded, &state); err != nil {
		return assistantTransportStoryResult{RawTransport: raw}, err
	}
	if state.SessionID == "" {
		state.SessionID = initial.SessionID
	}
	snapshot, snapshotErr := latestAssistantTransportStorySnapshot(sessions, state.SessionID)
	if snapshotErr != nil {
		return assistantTransportStoryResult{State: state, RawTransport: raw}, snapshotErr
	}
	traceRef := ""
	if snapshot != nil && len(snapshot.Iterations) > 0 {
		traceRef = snapshot.Iterations[len(snapshot.Iterations)-1].ModelInputTraceFile
	}
	normalized, err := normalizedAssistantTransportStoryState(state)
	if err != nil {
		return assistantTransportStoryResult{State: state, Snapshot: snapshot, TraceRef: traceRef, RawTransport: raw}, err
	}
	return assistantTransportStoryResult{
		State:           state,
		NormalizedState: normalized,
		Snapshot:        snapshot,
		TraceRef:        traceRef,
		RawTransport:    raw,
	}, nil
}

func latestAssistantTransportStorySnapshot(sessions *runtimekernel.SessionManager, sessionID string) (*runtimekernel.TurnSnapshot, error) {
	if sessions == nil {
		return nil, nil
	}
	session, err := sessions.GetSnapshot(sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	if session.CurrentTurn != nil {
		return session.CurrentTurn, nil
	}
	if len(session.TurnHistory) > 0 {
		return &session.TurnHistory[len(session.TurnHistory)-1], nil
	}
	return nil, nil
}

func normalizedAssistantTransportStoryState(state appui.AiopsTransportState) (map[string]any, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	normalizeAssistantTransportStoryJSON(normalized, state.CurrentTurnID)
	return normalized, nil
}

func normalizeAssistantTransportStoryJSON(state map[string]any, turnID string) {
	if state == nil {
		return
	}
	if state["updatedAt"] != nil && state["updatedAt"] != "" {
		state["updatedAt"] = "<timestamp>"
	}
	if state["currentTurnId"] == turnID && turnID != "" {
		state["currentTurnId"] = "<turn-id>"
	}
	if order, ok := state["turnOrder"].([]any); ok {
		for index, value := range order {
			if value == turnID && turnID != "" {
				order[index] = "<turn-id>"
			}
		}
	}
	turns, ok := state["turns"].(map[string]any)
	if !ok || turnID == "" {
		return
	}
	turn, exists := turns[turnID]
	if !exists {
		return
	}
	normalizeAssistantTransportStoryTurnJSON(turn, turnID)
	delete(turns, turnID)
	turns["<turn-id>"] = turn
}

func normalizeAssistantTransportStoryTurnJSON(value any, turnID string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isProtectedAssistantTransportStoryFact(key) {
				continue
			}
			if isStoryTimestampKey(key) && child != "" {
				typed[key] = "<timestamp>"
				continue
			}
			if (key == "id" || key == "turnId") && turnID != "" {
				if text, ok := child.(string); ok {
					typed[key] = strings.ReplaceAll(text, turnID, "<turn-id>")
				}
			}
			normalizeAssistantTransportStoryTurnJSON(typed[key], turnID)
		}
	case []any:
		for index := range typed {
			normalizeAssistantTransportStoryTurnJSON(typed[index], turnID)
		}
	}
}

func isProtectedAssistantTransportStoryFact(key string) bool {
	switch key {
	case "toolCallId", "toolCallIds", "approvalId", "evidenceRefs", "checkedEvidenceRefs", "payload", "metadata", "inlineData", "externalReferences", "rawRef":
		return true
	default:
		return false
	}
}

func isStoryTimestampKey(key string) bool {
	switch key {
	case "createdAt", "updatedAt", "startedAt", "completedAt", "requestedAt", "resolvedAt", "timestamp":
		return true
	default:
		return false
	}
}

func failAssistantTransportStory(t *testing.T, story assistantTransportStory, result assistantTransportStoryResult, format string, args ...any) {
	t.Helper()
	cause := fmt.Errorf(format, args...)
	t.Fatal(formatAssistantTransportStoryFailure(story, result, cause))
}

func formatAssistantTransportStoryFailure(story assistantTransportStory, result assistantTransportStoryResult, cause error) string {
	command, _ := json.MarshalIndent(story.Command, "", "  ")
	state, _ := json.MarshalIndent(result.NormalizedState, "", "  ")
	snapshot, _ := json.MarshalIndent(result.Snapshot, "", "  ")
	return fmt.Sprintf("%v\ncommand=%s\nlatest transport state=%s\nturn snapshot=%s\ntrace ref=%s\nraw transport=%s", cause, command, state, snapshot, result.TraceRef, result.RawTransport)
}

type storyProvider struct {
	mu        sync.Mutex
	responses []*schema.Message
}

func newStoryProvider(t *testing.T, fixtures []storyProviderResponse) *storyProvider {
	t.Helper()
	responses := make([]*schema.Message, 0, len(fixtures))
	for _, fixture := range fixtures {
		if !strings.EqualFold(strings.TrimSpace(fixture.Role), "assistant") {
			t.Fatalf("story provider response role=%q, want assistant", fixture.Role)
		}
		calls := make([]schema.ToolCall, 0, len(fixture.ToolCalls))
		for _, call := range fixture.ToolCalls {
			calls = append(calls, schema.ToolCall{ID: call.ID, Type: "function", Function: schema.FunctionCall{Name: call.Name, Arguments: string(call.Arguments)}})
		}
		responses = append(responses, schema.AssistantMessage(fixture.Content, calls))
	}
	return &storyProvider{responses: responses}
}

func (p *storyProvider) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return p.next()
}

func (p *storyProvider) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := p.next()
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (p *storyProvider) BindTools([]*schema.ToolInfo) error { return nil }

func (p *storyProvider) next() (*schema.Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) == 0 {
		return nil, errors.New("assistant transport story provider has no response remaining")
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *storyProvider) assertExhausted() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.responses) != 0 {
		return fmt.Errorf("%d scripted provider response(s) were not consumed", len(p.responses))
	}
	return nil
}

type storyProviderConfigResolver struct{ maxContextTokens int }

func (r storyProviderConfigResolver) ResolveProviderConfig(modelrouter.AgentKind) (modelrouter.ProviderConfig, bool) {
	maxTokens := r.maxContextTokens
	if maxTokens <= 0 {
		maxTokens = runtimekernel.DefaultMaxTokens
	}
	return modelrouter.ProviderConfig{Provider: "story", Model: "story", MaxContextTokens: maxTokens}, true
}

type storyToolSource struct{ registry *tooling.Registry }

func (s storyToolSource) CompileContext(session runtimekernel.SessionType, mode runtimekernel.Mode) promptcompiler.CompileContext {
	return promptcompiler.CompileContext{SessionType: string(session), Mode: string(mode), AssembledTools: s.registry.AssembleTools(string(session), string(mode))}
}

func (s storyToolSource) AssembleToolPool(session runtimekernel.SessionType, mode runtimekernel.Mode) []tool.BaseTool {
	return s.registry.AssembleToolPool(string(session), string(mode))
}

func (s storyToolSource) CompileContextWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []promptcompiler.Tool {
	return s.registry.CompileContextWithMetadata(string(session), string(mode), metadata)
}

func (s storyToolSource) AssembleToolPoolWithMetadata(session runtimekernel.SessionType, mode runtimekernel.Mode, metadata map[string]string) []tool.BaseTool {
	return s.registry.AssembleToolPoolWithMetadata(string(session), string(mode), metadata)
}

type storyEventEmitter struct{}

func (storyEventEmitter) Emit(runtimekernel.LifecycleEvent) {}

func sortedStoryKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func firstStoryValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func storySlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "-", "/", "-", "_", "-").Replace(value)
	return value
}

type storyToolControls struct {
	mu      sync.Mutex
	started map[string]chan struct{}
	once    map[string]*sync.Once
}

func newStoryToolControls(outcomes []storyToolOutcome) *storyToolControls {
	controls := &storyToolControls{started: map[string]chan struct{}{}, once: map[string]*sync.Once{}}
	for _, outcome := range outcomes {
		controls.started[outcome.Name] = make(chan struct{})
		controls.once[outcome.Name] = &sync.Once{}
	}
	return controls
}

func (c *storyToolControls) markStarted(name string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	started := c.started[name]
	once := c.once[name]
	c.mu.Unlock()
	if started != nil && once != nil {
		once.Do(func() { close(started) })
	}
}

func (c *storyToolControls) waitStarted(t *testing.T, name string) {
	t.Helper()
	if strings.TrimSpace(name) == "" {
		return
	}
	c.mu.Lock()
	started := c.started[name]
	c.mu.Unlock()
	if started == nil {
		t.Fatalf("story waitForTool %q has no matching tool outcome", name)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("story tool %q did not start", name)
	}
}

func hydrateStoryTransportState(t *testing.T, state appui.AiopsTransportState, sessions *runtimekernel.SessionManager) appui.AiopsTransportState {
	t.Helper()
	next := state
	if sessions == nil {
		return next
	}
	session := mustAssistantTransportStorySessionSnapshot(t, sessions, next.SessionID)
	if session == nil || session.CurrentTurn == nil {
		return next
	}
	next.CurrentTurnID = session.CurrentTurn.ID
	if session.CurrentTurn.Lifecycle == runtimekernel.TurnLifecycleRunning {
		next.Status = appui.AiopsTransportStatusWorking
	}
	return next
}

func resolveStoryTransportCommand(t *testing.T, command map[string]any, state appui.AiopsTransportState, sessions *runtimekernel.SessionManager) map[string]any {
	t.Helper()
	raw, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("marshal story command: %v", err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatalf("clone story command: %v", err)
	}
	approvalID := ""
	if storyTransportValueContains(cloned, "<pending-approval>") {
		approvalID = resolveStoryPendingApprovalID(t, state, sessions)
	}
	turnID := state.CurrentTurnID
	if sessions != nil {
		if session := mustAssistantTransportStorySessionSnapshot(t, sessions, state.SessionID); session != nil && session.CurrentTurn != nil {
			turnID = session.CurrentTurn.ID
		}
	}
	resolved := resolveStoryTransportValue(cloned, map[string]string{"<pending-approval>": approvalID, "<current-turn>": turnID})
	return resolved.(map[string]any)
}

func storyTransportValueContains(value any, target string) bool {
	switch typed := value.(type) {
	case string:
		return typed == target
	case map[string]any:
		for _, child := range typed {
			if storyTransportValueContains(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if storyTransportValueContains(child, target) {
				return true
			}
		}
	}
	return false
}

func resolveStoryPendingApprovalID(t *testing.T, state appui.AiopsTransportState, sessions *runtimekernel.SessionManager) string {
	t.Helper()
	if sessions == nil {
		t.Fatal("story command uses <pending-approval> without a runtime session manager")
	}
	session := mustAssistantTransportStorySessionSnapshot(t, sessions, state.SessionID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("story command uses <pending-approval> without a published current turn")
	}
	if len(session.CurrentTurn.PendingApprovals) != 1 {
		t.Fatalf("story command requires exactly one canonical pending approval, got %d", len(session.CurrentTurn.PendingApprovals))
	}
	pending := session.CurrentTurn.PendingApprovals[0]
	if strings.TrimSpace(pending.ID) == "" || strings.TrimSpace(pending.ToolCallID) == "" || strings.TrimSpace(pending.ToolName) == "" {
		t.Fatalf("canonical pending approval lacks correlation facts: %#v", pending)
	}
	transportPending, ok := state.PendingApprovals[pending.ID]
	if !ok {
		t.Fatalf("transport pending approvals do not contain canonical approval %q", pending.ID)
	}
	if transportPending.TurnID != "" && transportPending.TurnID != pending.TurnID {
		t.Fatalf("approval %q turn correlation differs: transport=%q runtime=%q", pending.ID, transportPending.TurnID, pending.TurnID)
	}
	matchedItem := false
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type != agentstate.TurnItemTypeApprovalRequested {
			continue
		}
		var fact struct {
			ApprovalID string `json:"approvalId"`
			ToolCallID string `json:"toolCallId"`
			ToolName   string `json:"toolName"`
		}
		if err := json.Unmarshal(item.Payload.Data, &fact); err != nil {
			t.Fatalf("decode canonical approval_requested item %q: %v", item.ID, err)
		}
		if fact.ApprovalID == pending.ID {
			if fact.ToolCallID != pending.ToolCallID || fact.ToolName != pending.ToolName {
				t.Fatalf("approval %q item correlation differs: item=%s#%s pending=%s#%s", pending.ID, fact.ToolName, fact.ToolCallID, pending.ToolName, pending.ToolCallID)
			}
			matchedItem = true
		}
	}
	if !matchedItem {
		t.Fatalf("canonical approval_requested item missing for pending approval %q", pending.ID)
	}
	return pending.ID
}

func resolveStoryTransportValue(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case string:
		if replacement, ok := replacements[typed]; ok {
			return replacement
		}
		return typed
	case map[string]any:
		for key, child := range typed {
			typed[key] = resolveStoryTransportValue(child, replacements)
		}
	case []any:
		for index := range typed {
			typed[index] = resolveStoryTransportValue(typed[index], replacements)
		}
	}
	return value
}

func mustAssistantTransportStorySessionSnapshot(t *testing.T, sessions *runtimekernel.SessionManager, sessionID string) *runtimekernel.SessionState {
	t.Helper()
	if sessions == nil {
		return nil
	}
	session, err := sessions.GetSnapshot(sessionID)
	if err != nil {
		t.Fatalf("get published session snapshot %q: %v", sessionID, err)
	}
	return session
}
