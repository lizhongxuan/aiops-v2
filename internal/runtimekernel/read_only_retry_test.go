package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/tooling"
)

func TestReadOnlyRetryPolicy(t *testing.T) {
	base := ReadOnlyRetryInput{
		Config: ReadOnlyRetryConfig{
			Enabled:     true,
			MaxPerCall:  1,
			MaxPerTurn:  3,
			BackoffBase: 10 * time.Millisecond,
			BackoffMax:  100 * time.Millisecond,
		},
		FailureKind:                      string(toolfailure.KindTimeout),
		OriginalArgumentsHash:            "sha256:args",
		EffectiveArgumentsHash:           "sha256:args",
		OriginalToolSurfaceFingerprint:   "surface-1",
		EffectiveToolSurfaceFingerprint:  "surface-1",
		CompletedRetryAttemptsForCall:    0,
		CompletedRetryAttemptsForTurn:    0,
		ProspectiveRetryAttemptsThisCall: 1,
	}

	if got := DecideReadOnlyRetry(base); !got.Allowed || !got.Eligible {
		t.Fatalf("read-only timeout decision = %#v, want allowed eligible", got)
	}

	mutating := base
	mutating.Mutating = true
	if got := DecideReadOnlyRetry(mutating); got.Allowed || got.Eligible {
		t.Fatalf("mutating decision = %#v, want denied not eligible", got)
	}

	invalidArgs := base
	invalidArgs.FailureKind = string(toolfailure.KindInvalidArguments)
	if got := DecideReadOnlyRetry(invalidArgs); got.Allowed || got.Eligible {
		t.Fatalf("invalid args decision = %#v, want denied", got)
	}

	expired := base
	expired.FailureKind = string(toolfailure.KindMCPSessionExpired)
	if got := DecideReadOnlyRetry(expired); got.Allowed || got.Eligible {
		t.Fatalf("mcp session expired decision = %#v, want denied", got)
	}

	argsChanged := base
	argsChanged.EffectiveArgumentsHash = "sha256:changed"
	if got := DecideReadOnlyRetry(argsChanged); got.Allowed || got.Eligible {
		t.Fatalf("changed args decision = %#v, want denied", got)
	}

	surfaceChanged := base
	surfaceChanged.EffectiveToolSurfaceFingerprint = "surface-2"
	if got := DecideReadOnlyRetry(surfaceChanged); got.Allowed || got.Eligible {
		t.Fatalf("changed tool surface decision = %#v, want denied", got)
	}

	overBudget := base
	overBudget.CompletedRetryAttemptsForTurn = 3
	if got := DecideReadOnlyRetry(overBudget); got.Allowed || got.Eligible {
		t.Fatalf("over budget decision = %#v, want denied", got)
	}

	disabled := base
	disabled.Config.Enabled = false
	if got := DecideReadOnlyRetry(disabled); got.Allowed || !got.Eligible || !strings.Contains(got.Reason, "disabled") {
		t.Fatalf("disabled decision = %#v, want eligible but disabled", got)
	}
}

func TestFailedToolModelGuidanceOnlyCitesCompletedToolResults(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mutating    bool
		failureKind string
		finalStatus string
	}{
		{name: "retryable_read_timeout", failureKind: string(toolfailure.KindTimeout), finalStatus: string(ToolInvocationFailed)},
		{name: "invalid_arguments", failureKind: string(toolfailure.KindInvalidArguments), finalStatus: string(ToolInvocationFailed)},
		{name: "tool_business_error", failureKind: string(toolfailure.KindToolBusinessError), finalStatus: string(ToolInvocationFailed)},
		{name: "blocked", failureKind: string(toolfailure.KindPolicyDenied), finalStatus: string(ToolInvocationBlocked)},
		{name: "mutating", mutating: true, failureKind: string(toolfailure.KindTimeout), finalStatus: string(ToolInvocationFailed)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guidance := failedToolModelGuidance(tc.mutating, tc.failureKind, tc.finalStatus)
			for _, want := range []string{
				"Only cite tools or resources that appear in completed tool results",
				"do not claim a tool, MCP resource, log, metric, topology, or config was checked",
			} {
				if !strings.Contains(guidance, want) {
					t.Fatalf("guidance = %q, want %q", guidance, want)
				}
			}
		})
	}
}

func TestReadOnlyRetryDecisionIncludesFailureSignatureSwitchPath(t *testing.T) {
	signature := BuildFailureSignature("read_metrics", json.RawMessage(`{"resourceType":"synthetic_resource","resourceId":"resource-a"}`), ToolResult{
		Error: "context deadline exceeded request-id=abc after 1000ms",
	})
	got := DecideReadOnlyRetry(ReadOnlyRetryInput{
		Config: ReadOnlyRetryConfig{
			Enabled:     true,
			MaxPerCall:  3,
			MaxPerTurn:  6,
			BackoffBase: 0,
			BackoffMax:  0,
		},
		FailureKind:                      string(toolfailure.KindTimeout),
		OriginalArgumentsHash:            "sha256:args",
		EffectiveArgumentsHash:           "sha256:args",
		OriginalToolSurfaceFingerprint:   "surface-1",
		EffectiveToolSurfaceFingerprint:  "surface-1",
		CompletedRetryAttemptsForCall:    2,
		CompletedRetryAttemptsForTurn:    2,
		ProspectiveRetryAttemptsThisCall: 3,
		FailureSignature:                 signature,
		FailureSignatureSeenCount:        3,
	})

	if got.Allowed {
		t.Fatalf("decision = %#v, want retry blocked after repeated same failure signature", got)
	}
	if got.FailureSignature == nil {
		t.Fatalf("decision = %#v, want failure signature decision", got)
	}
	if got.FailureSignature.Action != "switch_path" {
		t.Fatalf("failure signature decision = %#v, want switch_path", got.FailureSignature)
	}
	if got.FailureSignature.SwitchPathReason == "" || !strings.Contains(got.Reason, "switch path") {
		t.Fatalf("decision = %#v, want model-visible switch path reason", got)
	}
}

func TestReadOnlyRetryFlagOffRecordsSkippedAttempt(t *testing.T) {
	executor := readOnlySequenceToolExecutor([]tooling.ToolResult{{Error: "context deadline exceeded"}})
	dispatcher := NewToolDispatcher(readOnlyRetryLookup(executor), nil, &testMockEventEmitter{}).
		WithToolSurfaceFingerprint("surface-1").
		WithReadOnlyRetryConfig(ReadOnlyRetryConfig{
			Enabled:     false,
			MaxPerCall:  1,
			MaxPerTurn:  3,
			BackoffBase: 0,
			BackoffMax:  0,
		})

	result := dispatcher.Dispatch(context.Background(), "sess-retry", "turn-retry", ToolCall{
		ID:        "call-retry",
		Name:      "read_metrics",
		Arguments: json.RawMessage(`{"namespace":"prod"}`),
	}, SessionTypeHost, ModeInspect)

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
	if result.Error == "" {
		t.Fatalf("Dispatch() = %#v, want timeout error", result)
	}
	if len(result.Attempts) != 1 {
		t.Fatalf("Attempts = %#v, want one skipped retry", result.Attempts)
	}
	attempt := result.Attempts[0]
	if attempt.Action != ToolAttemptActionRetry || attempt.Outcome != ToolAttemptOutcomeSkipped || attempt.TriggerFailureKind != string(toolfailure.KindTimeout) {
		t.Fatalf("attempt = %#v, want retry skipped timeout", attempt)
	}
}

func TestReadOnlyRetryDoesNotRetryDynamicDestructiveTool(t *testing.T) {
	executor := &dynamicRiskSequenceToolExecutor{
		sequenceToolExecutor: sequenceToolExecutor{
			results: []tooling.ToolResult{{Error: "context deadline exceeded"}, {Content: "should-not-run"}},
		},
		readOnly:    false,
		destructive: true,
	}
	dispatcher := NewToolDispatcher(readOnlyRetryLookup(executor), nil, &testMockEventEmitter{}).
		WithToolSurfaceFingerprint("surface-1").
		WithReadOnlyRetryConfig(ReadOnlyRetryConfig{
			Enabled:     true,
			MaxPerCall:  1,
			MaxPerTurn:  3,
			BackoffBase: 0,
			BackoffMax:  0,
		})

	result := dispatcher.Dispatch(context.Background(), "sess-retry", "turn-retry", ToolCall{
		ID:        "call-retry",
		Name:      "read_metrics",
		Arguments: json.RawMessage(`{"namespace":"prod","mutation":"restart"}`),
	}, SessionTypeHost, ModeExecute)

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1 for dynamic destructive tool", executor.calls)
	}
	if result.Error == "" {
		t.Fatalf("Dispatch() = %#v, want original timeout error", result)
	}
	if len(result.Attempts) != 0 {
		t.Fatalf("Attempts = %#v, want no retry attempt for dynamic destructive tool", result.Attempts)
	}
}

func TestReadOnlyRetryFlagOnRetriesTimeoutOnce(t *testing.T) {
	executor := readOnlySequenceToolExecutor([]tooling.ToolResult{{Error: "context deadline exceeded"}, {Content: "healthy"}})
	observer := &toolRecordingObserver{}
	dispatcher := NewToolDispatcher(readOnlyRetryLookup(executor), nil, &testMockEventEmitter{}).
		WithObserver(observer).
		WithToolSurfaceFingerprint("surface-1").
		WithReadOnlyRetryConfig(ReadOnlyRetryConfig{
			Enabled:     true,
			MaxPerCall:  1,
			MaxPerTurn:  3,
			BackoffBase: 0,
			BackoffMax:  0,
		})

	result := dispatcher.Dispatch(context.Background(), "sess-retry", "turn-retry", ToolCall{
		ID:        "call-retry",
		Name:      "read_metrics",
		Arguments: json.RawMessage(`{"namespace":"prod"}`),
	}, SessionTypeHost, ModeInspect)

	if executor.calls != 2 {
		t.Fatalf("executor calls = %d, want 2", executor.calls)
	}
	if result.Error != "" || result.Content != "healthy" {
		t.Fatalf("Dispatch() = %#v, want successful retry", result)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("Attempts = %#v, want retry started and completed", result.Attempts)
	}
	if result.Attempts[0].Action != ToolAttemptActionRetry || result.Attempts[0].Outcome != ToolAttemptOutcomeStarted {
		t.Fatalf("first attempt = %#v, want retry started", result.Attempts[0])
	}
	if result.Attempts[1].Action != ToolAttemptActionRetry || result.Attempts[1].Outcome != ToolAttemptOutcomeCompleted {
		t.Fatalf("second attempt = %#v, want retry completed", result.Attempts[1])
	}
	if len(observer.spans) != 1 {
		t.Fatalf("observer spans = %d, want 1", len(observer.spans))
	}
	span := observer.spans[0]
	if span.attrs["tool.attempt_count"] != 2 || span.attrs["tool.last_attempt_action"] != "retry" || span.attrs["tool.last_attempt_outcome"] != "completed" {
		t.Fatalf("observer attempt attrs = %#v", span.attrs)
	}
}

func TestReadOnlyRetryCancelDuringBackoffDoesNotDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	executor := readOnlySequenceToolExecutor([]tooling.ToolResult{{Error: "context deadline exceeded"}, {Content: "should-not-run"}})
	executor.onCall = func(call int) {
		if call == 1 {
			cancel()
		}
	}
	dispatcher := NewToolDispatcher(readOnlyRetryLookup(executor), nil, &testMockEventEmitter{}).
		WithToolSurfaceFingerprint("surface-1").
		WithReadOnlyRetryConfig(ReadOnlyRetryConfig{
			Enabled:     true,
			MaxPerCall:  1,
			MaxPerTurn:  3,
			BackoffBase: time.Hour,
			BackoffMax:  time.Hour,
		})

	result := dispatcher.Dispatch(ctx, "sess-retry", "turn-retry", ToolCall{
		ID:        "call-retry",
		Name:      "read_metrics",
		Arguments: json.RawMessage(`{"namespace":"prod"}`),
	}, SessionTypeHost, ModeInspect)

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
	if result.Error == "" {
		t.Fatalf("Dispatch() = %#v, want original timeout error", result)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].Outcome != ToolAttemptOutcomeSkipped {
		t.Fatalf("Attempts = %#v, want skipped retry after cancel", result.Attempts)
	}
}

func readOnlyRetryLookup(executor ToolExecutor) *mockToolLookup {
	return &mockToolLookup{
		tools: map[string]mockToolEntry{
			"read_metrics": {
				desc: ToolDescriptor{
					Metadata: tooling.ToolMetadata{
						Name:      "read_metrics",
						Origin:    tooling.ToolOriginBuiltin,
						Mutating:  false,
						RiskLevel: tooling.ToolRiskLow,
					},
					InputSchema: json.RawMessage(`{"type":"object"}`),
				},
				executor: executor,
			},
		},
	}
}

type sequenceToolExecutor struct {
	results []tooling.ToolResult
	errors  []error
	calls   int
	onCall  func(int)
}

func (e *sequenceToolExecutor) Execute(ctx context.Context, args json.RawMessage) (tooling.ToolResult, error) {
	e.calls++
	if e.onCall != nil {
		e.onCall(e.calls)
	}
	idx := e.calls - 1
	if idx < len(e.errors) && e.errors[idx] != nil {
		return tooling.ToolResult{}, e.errors[idx]
	}
	if idx < len(e.results) {
		return e.results[idx], nil
	}
	return tooling.ToolResult{}, nil
}

type dynamicRiskSequenceToolExecutor struct {
	sequenceToolExecutor
	readOnly    bool
	destructive bool
}

func readOnlySequenceToolExecutor(results []tooling.ToolResult) *dynamicRiskSequenceToolExecutor {
	return &dynamicRiskSequenceToolExecutor{
		sequenceToolExecutor: sequenceToolExecutor{results: results},
		readOnly:             true,
		destructive:          false,
	}
}

func (e *dynamicRiskSequenceToolExecutor) IsReadOnly(json.RawMessage) bool {
	return e.readOnly
}

func (e *dynamicRiskSequenceToolExecutor) IsDestructive(json.RawMessage) bool {
	return e.destructive
}
