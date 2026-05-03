package promptdiag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateLLMSuggestionsUsesSummaryOnly(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization header = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		data, _ := json.Marshal(body)
		requestBody = string(data)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"建议收紧 tool 选择规则。"}}]}`))
	}))
	defer server.Close()

	diagnosis := RunDiagnosis{
		Summary: DiagnosisSummary{Total: 1, Failed: 1},
		Cases: []CaseDiagnosis{{
			CaseID:          "case-1",
			Passed:          false,
			Score:           0,
			LikelyRootCause: RootCausePrompt,
			FailedChecks:    []string{"expectedToolCalls"},
			Evidence: EvidenceSummary{
				AnswerPreview:        "短摘要",
				ToolCallCount:        0,
				PromptSizeChars:      1024,
				MissingExpectedTools: []string{"exec_command"},
			},
			RuleHits: []RuleHit{{RuleID: "expected-tool-not-called", Severity: "warning", RootCause: RootCausePrompt}},
		}},
	}

	suggestions, err := GenerateLLMSuggestions(context.Background(), Config{
		LLMBaseURL: server.URL + "/v1",
		LLMAPIKey:  "test-key",
		LLMModel:   "gpt-5.4",
	}, diagnosis)
	if err != nil {
		t.Fatalf("GenerateLLMSuggestions: %v", err)
	}
	if len(suggestions) != 1 || !suggestions[0].LLMAssisted {
		t.Fatalf("suggestions = %#v", suggestions)
	}
	if strings.Contains(requestBody, "RAW_PROMPT_SECRET") || strings.Contains(requestBody, "secret-value") {
		t.Fatalf("request body contains disallowed raw content: %s", requestBody)
	}
	if !strings.Contains(requestBody, "expected-tool-not-called") {
		t.Fatalf("request body missing rule summary: %s", requestBody)
	}
}
