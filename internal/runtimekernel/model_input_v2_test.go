package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

type firstRequestTimeoutThenSuccessModel struct {
	streamAttempts int
	inputs         [][]*schema.Message
}

func (m *firstRequestTimeoutThenSuccessModel) Stream(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.inputs = append(m.inputs, cloneSchemaMessages(input))
	m.streamAttempts++
	if m.streamAttempts == 1 {
		return nil, context.DeadlineExceeded
	}
	return schema.StreamReaderFromArray([]*schema.Message{schema.AssistantMessage("retry complete", nil)}), nil
}

func (m *firstRequestTimeoutThenSuccessModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if m.streamAttempts == 1 {
		return nil, context.DeadlineExceeded
	}
	return nil, errors.New("generate should not be called after retry stream succeeds")
}

func (m *firstRequestTimeoutThenSuccessModel) BindTools(_ []*schema.ToolInfo) error { return nil }

func TestBuildRuntimePromptInputV2SelectsTypedCurrentInput(t *testing.T) {
	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType: "host",
		Mode:        "inspect",
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	toolHistory := []Message{
		{Role: "user", Content: "inspect redis"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "read_redis", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", Content: "redis evidence", ToolResult: &ToolResult{ToolCallID: "call-1", Content: "redis evidence"}},
	}
	tests := []struct {
		name          string
		iteration     int
		cause         *StepRevisionCause
		history       []Message
		wantRole      promptinput.ProviderRole
		wantSemantic  string
		wantContent   string
		wantCallGroup bool
	}{
		{
			name: "initial user", iteration: 0,
			history:  []Message{{Role: "user", Content: "inspect redis"}, {Role: "user", Content: "   "}},
			wantRole: promptinput.ProviderRoleUser, wantSemantic: "current_user_input", wantContent: "inspect redis",
		},
		{
			name: "first model retry replays initial user", iteration: 0,
			cause:    &StepRevisionCause{Kind: StepRevisionKindModelRetryResumed},
			history:  []Message{{Role: "user", Content: "inspect redis"}},
			wantRole: promptinput.ProviderRoleUser, wantSemantic: "current_user_input", wantContent: "inspect redis",
		},
		{
			name: "normal tool continuation", iteration: 1, history: toolHistory,
			wantRole: promptinput.ProviderRoleDeveloper, wantSemantic: "continuation_instruction", wantContent: "completed L4 tool results", wantCallGroup: true,
		},
		{
			name: "approval resume", iteration: 1, history: toolHistory,
			cause:    &StepRevisionCause{Kind: StepRevisionKindApprovalResumed, ApprovalID: "approval-1", ToolCallID: "call-1"},
			wantRole: promptinput.ProviderRoleDeveloper, wantSemantic: "continuation_instruction", wantContent: "approval resume", wantCallGroup: true,
		},
		{
			name: "user input resume", iteration: 2,
			cause: &StepRevisionCause{Kind: StepRevisionKindUserInputResumed},
			history: append(append([]Message(nil), toolHistory...),
				Message{Role: "assistant", Content: "Which cluster?"}, Message{Role: "user", Content: "production"}),
			wantRole: promptinput.ProviderRoleUser, wantSemantic: "current_user_input", wantContent: "production", wantCallGroup: true,
		},
		{
			name: "model retry", iteration: 2,
			cause:    &StepRevisionCause{Kind: StepRevisionKindModelRetryResumed},
			history:  []Message{{Role: "user", Content: "inspect redis"}, {Role: "assistant", Content: "partial reasoning"}},
			wantRole: promptinput.ProviderRoleDeveloper, wantSemantic: "continuation_instruction", wantContent: "model retry",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildRuntimePromptInputV2WithContextGovernance(tt.history, compiled, nil, tt.iteration, tt.cause)
			if err != nil {
				t.Fatalf("buildRuntimePromptInputV2WithContextGovernance() error = %v", err)
			}
			last := result.Items[len(result.Items)-1]
			if last.Source.Layer != string(promptcompiler.LayerCurrentUserInput) || last.ProviderRole != tt.wantRole || last.SemanticRole != tt.wantSemantic || !strings.Contains(last.Content, tt.wantContent) {
				t.Fatalf("last item = %#v, want role=%q semantic=%q content containing %q", last, tt.wantRole, tt.wantSemantic, tt.wantContent)
			}
			if countRuntimeModelInputLayer(result.Items, promptcompiler.LayerCurrentUserInput) != 1 {
				t.Fatalf("L6 count != 1: %#v", result.Items)
			}
			if tt.wantCallGroup && !runtimeModelInputHasCausalGroup(result.Items, "call-1") {
				t.Fatalf("tool causal group missing: %#v", result.Items)
			}
			again, err := buildRuntimePromptInputV2WithContextGovernance(tt.history, compiled, nil, tt.iteration, tt.cause)
			if err != nil || promptinput.StableModelInputHash(again.Items) != promptinput.StableModelInputHash(result.Items) {
				t.Fatalf("runtime V2 model input is not deterministic: err=%v", err)
			}
		})
	}
}

func TestRuntimePromptCurrentInputRejectsInvalidState(t *testing.T) {
	userHistory := []promptinput.Message{{Role: "user", Content: "inspect"}}
	tests := []struct {
		name      string
		history   []promptinput.Message
		iteration int
		cause     *StepRevisionCause
	}{
		{name: "negative iteration", history: userHistory, iteration: -1},
		{name: "missing initial user", iteration: 0},
		{name: "empty cause", history: userHistory, iteration: 1, cause: &StepRevisionCause{}},
		{name: "unknown cause", history: userHistory, iteration: 1, cause: &StepRevisionCause{Kind: "unknown"}},
		{name: "approval at iteration zero", history: userHistory, iteration: 0, cause: &StepRevisionCause{Kind: StepRevisionKindApprovalResumed, ApprovalID: "approval-1", ToolCallID: "call-1"}},
		{name: "resumed user missing", iteration: 1, cause: &StepRevisionCause{Kind: StepRevisionKindUserInputResumed}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := runtimePromptCurrentInput(tt.history, tt.iteration, tt.cause); err == nil {
				t.Fatal("runtimePromptCurrentInput() accepted invalid state")
			}
		})
	}
}

func TestBuildRuntimePromptInputV2UsesTransformedCurrentUser(t *testing.T) {
	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{SessionType: "host", Mode: "inspect"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	longEvidence := strings.Repeat("redis evidence line ", 90)
	result, err := buildRuntimePromptInputV2WithContextGovernance([]Message{
		{Role: "user", Content: longEvidence},
		{Role: "assistant", Content: "prior answer"},
		{Role: "user", Content: longEvidence},
	}, compiled, nil, 0, nil)
	if err != nil {
		t.Fatalf("buildRuntimePromptInputV2WithContextGovernance() error = %v", err)
	}
	tail := result.Items[len(result.Items)-1]
	if !strings.Contains(tail.Content, "User evidence repeated from previous turn") || tail.Content == longEvidence {
		t.Fatalf("L6 current user was not selected from transformed history: %#v", tail)
	}
}

func TestRunTurnUsesV2CurrentInputThenTypedToolContinuation(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("checking", []schema.ToolCall{{
			ID: "call-v2", Type: "function", Function: schema.FunctionCall{Name: "read_v2", Arguments: `{}`},
		}}),
		schema.AssistantMessage("done", nil),
	}}
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "read_v2", Description: "Read evidence"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "v2 evidence"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-v2-input", SessionType: SessionTypeHost, Mode: ModeInspect,
		TurnID: "turn-v2-input", Input: "inspect with v2",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model input count = %d, want 2", len(model.inputs))
	}
	firstLast := model.inputs[0][len(model.inputs[0])-1]
	if firstLast.Role != schema.User || firstLast.Content != "inspect with v2" {
		t.Fatalf("first provider tail = %#v, want current user", firstLast)
	}
	second := model.inputs[1]
	secondLast := second[len(second)-1]
	if secondLast.Role != schema.System || !strings.Contains(secondLast.Content, "completed L4 tool results") {
		t.Fatalf("second provider tail = %#v, want typed continuation", secondLast)
	}
	if !schemaMessagesHaveCausalGroup(second, "call-v2") {
		t.Fatalf("second provider input lost tool causal group: %s", schemaMessagesText(second))
	}
}

func TestResumeFirstModelTimeoutReplaysTypedInitialUser(t *testing.T) {
	model := &firstRequestTimeoutThenSuccessModel{}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	blocked, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-v2-first-timeout", SessionType: SessionTypeHost, Mode: ModeInspect,
		TurnID: "turn-v2-first-timeout", Input: "inspect after timeout",
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if blocked.Status != "blocked" {
		t.Fatalf("RunTurn() status = %q, want blocked", blocked.Status)
	}
	session := kernel.sessions.Get("sess-v2-first-timeout")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.LatestCheckpoint == nil {
		t.Fatalf("missing resumable timeout state: %#v", session)
	}
	if len(session.CurrentTurn.Iterations) != 0 {
		t.Fatalf("failed first request iterations = %d, want 0", len(session.CurrentTurn.Iterations))
	}
	resumed, err := kernel.ResumeTurn(context.Background(), ResumeRequest{
		SessionID: session.ID, TurnID: session.CurrentTurn.ID,
		CheckpointID: session.CurrentTurn.LatestCheckpoint.ID, ResumeState: TurnResumeStateResumable,
	})
	if err != nil {
		t.Fatalf("ResumeTurn() error = %v", err)
	}
	if resumed.Status != "completed" || len(model.inputs) < 2 {
		t.Fatalf("ResumeTurn() = %#v, inputs=%d", resumed, len(model.inputs))
	}
	lastInput := model.inputs[len(model.inputs)-1]
	tail := lastInput[len(lastInput)-1]
	if tail.Role != schema.User || tail.Content != "inspect after timeout" || tail.Extra["semantic_role"] != "current_user_input" || tail.Extra["source_layer"] != string(promptcompiler.LayerCurrentUserInput) {
		t.Fatalf("retry provider tail = %#v, want typed initial L6 user", tail)
	}
}

func countRuntimeModelInputLayer(items []promptinput.ModelInputItem, layer promptcompiler.PromptLogicalLayer) int {
	count := 0
	for _, item := range items {
		if item.Source.Layer == string(layer) {
			count++
		}
	}
	return count
}

func runtimeModelInputHasCausalGroup(items []promptinput.ModelInputItem, callID string) bool {
	for index, item := range items {
		if len(item.ToolCalls) == 1 && item.ToolCalls[0].ID == callID && index+1 < len(items) && items[index+1].ToolCallID == callID {
			return true
		}
	}
	return false
}

func schemaMessagesHaveCausalGroup(messages []*schema.Message, callID string) bool {
	for index, message := range messages {
		if message == nil || len(message.ToolCalls) != 1 || message.ToolCalls[0].ID != callID || index+1 >= len(messages) {
			continue
		}
		result := messages[index+1]
		return result != nil && result.Role == schema.Tool && result.ToolCallID == callID
	}
	return false
}
