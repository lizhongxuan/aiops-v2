package appui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/tooling"
)

type generalOpsIntentFrameEvalCase struct {
	ID       string                        `json:"id"`
	Input    string                        `json:"input"`
	Expected generalOpsIntentFrameExpected `json:"expected"`
}

type generalOpsIntentFrameExpected struct {
	Intent         string   `json:"intent"`
	DataScopes     []string `json:"data_scopes,omitempty"`
	RiskBudget     []string `json:"risk_budget,omitempty"`
	AllowPublicWeb *bool    `json:"allow_public_web,omitempty"`
	AllowHostExec  any      `json:"allow_host_exec,omitempty"`
}

func TestGeneralOpsIntentFrameEvalCases(t *testing.T) {
	cases := readGeneralOpsIntentFrameEvalCases(t)
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			envelope := BuildEvidenceEnvelope(tc.Input, nil, nil)
			frame := BuildIntentFrame(tc.Input, envelope, nil)
			decision := tooling.DecideToolSurface(frame, tooling.ApprovalSnapshot{}, nil)

			if string(frame.Kind) != tc.Expected.Intent {
				t.Fatalf("intent = %q, want %q; frame=%#v", frame.Kind, tc.Expected.Intent, frame)
			}
			for _, want := range tc.Expected.DataScopes {
				if !runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScope(want)) &&
					!runtimecontract.ContainsDataScope(frame.Evidence.DataScopes, runtimecontract.DataScope(want)) {
					t.Fatalf("data scopes = frame:%#v evidence:%#v, missing %q", frame.DataScopes, frame.Evidence.DataScopes, want)
				}
			}
			for _, want := range tc.Expected.RiskBudget {
				if !runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRisk(want)) {
					t.Fatalf("risk budget = %#v, missing %q", frame.RiskBudget, want)
				}
			}
			if tc.Expected.AllowPublicWeb != nil && decision.AllowPublicWeb != *tc.Expected.AllowPublicWeb {
				t.Fatalf("AllowPublicWeb = %v, want %v; decision=%#v frame=%#v", decision.AllowPublicWeb, *tc.Expected.AllowPublicWeb, decision, frame)
			}
			assertExpectedHostExecSurface(t, tc.Expected.AllowHostExec, decision)
		})
	}
}

func readGeneralOpsIntentFrameEvalCases(t *testing.T) []generalOpsIntentFrameEvalCase {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "eval_cases", "general_ops_intent_frame", "cases.jsonl")
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open eval cases: %v", err)
	}
	defer file.Close()

	var cases []generalOpsIntentFrameEvalCase
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		var tc generalOpsIntentFrameEvalCase
		if err := json.Unmarshal(line, &tc); err != nil {
			t.Fatalf("parse eval case %q: %v", string(line), err)
		}
		cases = append(cases, tc)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan eval cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("eval cases are empty")
	}
	return cases
}

func assertExpectedHostExecSurface(t *testing.T, expected any, decision tooling.SurfaceDecision) {
	t.Helper()
	switch want := expected.(type) {
	case nil:
		return
	case bool:
		if decision.AllowHostExec != want {
			t.Fatalf("AllowHostExec = %v, want %v; decision=%#v", decision.AllowHostExec, want, decision)
		}
	case string:
		if want != "approval_required" {
			t.Fatalf("unknown allow_host_exec expectation %q", want)
		}
		if decision.AllowHostExec {
			t.Fatalf("AllowHostExec = true, want approval required; decision=%#v", decision)
		}
		if !containsToolSurfaceReason(decision.Reasons, "host_exec_requires_approval") {
			t.Fatalf("decision reasons = %#v, want host_exec_requires_approval", decision.Reasons)
		}
	default:
		t.Fatalf("unsupported allow_host_exec expectation %#v", expected)
	}
}

func containsToolSurfaceReason(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
