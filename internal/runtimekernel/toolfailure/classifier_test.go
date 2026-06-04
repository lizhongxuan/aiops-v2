package toolfailure

import "testing"

func TestClassifyDispatchError(t *testing.T) {
	cases := []struct {
		name     string
		input    ClassificationInput
		wantKind ToolFailureKind
		wantAct  HandlingAction
	}{
		{
			name: "tool not found",
			input: ClassificationInput{
				Source:  "runtime",
				Outcome: "tool_failed",
				Error:   "tool not found: coroot.missing_tool",
			},
			wantKind: KindToolNotFound,
			wantAct:  ActionFeedErrorToModel,
		},
		{
			name: "policy denied",
			input: ClassificationInput{
				Source:  "policy",
				Outcome: "tool_denied",
				Error:   "denied: destructive command",
			},
			wantKind: KindPolicyDenied,
			wantAct:  ActionFailTurn,
		},
		{
			name: "timeout",
			input: ClassificationInput{
				Source:  "tool",
				Outcome: "tool_failed",
				Error:   "context deadline exceeded",
			},
			wantKind: KindTimeout,
			wantAct:  ActionFeedErrorToModel,
		},
		{
			name: "invalid arguments",
			input: ClassificationInput{
				Source:  "runtime",
				Outcome: "tool_failed",
				Error:   "invalid arguments: namespace is required",
			},
			wantKind: KindInvalidArguments,
			wantAct:  ActionFeedErrorToModel,
		},
		{
			name: "mcp session expired",
			input: ClassificationInput{
				Source:  "mcp",
				Outcome: "tool_failed",
				Error:   "JSON-RPC error: session expired",
			},
			wantKind: KindMCPSessionExpired,
			wantAct:  ActionFeedErrorToModel,
		},
	}

	classifier := NewClassifier()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.Classify(tc.input)
			if got.Kind != tc.wantKind || got.Action != tc.wantAct {
				t.Fatalf("got kind/action %q/%q, want %q/%q", got.Kind, got.Action, tc.wantKind, tc.wantAct)
			}
		})
	}
}
