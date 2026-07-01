package promptdiag

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/eval"
)

func TestDiagnoseClassifiesVisibleExpectedToolNotCalled(t *testing.T) {
	workspace := t.TempDir()
	casesDir := filepath.Join(workspace, "cases")
	traceDir := filepath.Join(workspace, "traces")
	evalDir := filepath.Join(workspace, "eval")
	writeTestJSON(t, filepath.Join(casesDir, "tool-case.json"), eval.Case{
		ID:       "tool-case",
		Category: "prompt",
		Priority: "P0",
		Input:    "read local file",
		Expected: eval.Expected{
			ExpectedToolCalls: []string{"read_file"},
		},
	})
	tracePath := filepath.Join(traceDir, "eval-run-tool-case", "turn-1", "iteration-000.json")
	writeTestJSON(t, tracePath, map[string]any{
		"schemaVersion": "aiops.trace/v2",
		"kind":          "runtime_model_input",
		"createdAt":     "2026-05-03T00:00:00Z",
		"sessionId":     "eval-run-tool-case",
		"turnId":        "turn-1",
		"iteration":     0,
		"promptFingerprint": map[string]string{
			"stableHash":       "stable-hash",
			"developerHash":    "developer-hash",
			"toolRegistryHash": "tool-hash",
		},
		"prompt": map[string]string{
			"stable":  "system rules",
			"dynamic": "runtime context",
		},
		"toolSurface": map[string]any{
			"modelVisibleTools": []string{"read_file"},
		},
		"stepContext": map[string]any{
			"modelInput": []map[string]string{
				{"providerRole": "system", "content": "rules"},
				{"providerRole": "user", "content": "read local file"},
			},
		},
	})
	turnItemsPath := filepath.Join(evalDir, "tool-case", "turn_items.json")
	writeTestJSON(t, turnItemsPath, []agentstate.TurnItem{
		{
			ID:     "model-0",
			Type:   agentstate.TurnItemTypeModelCall,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Data: rawJSON(t, map[string]any{
					"iteration": 0,
					"traceFile": filepath.ToSlash(strings.TrimSuffix(tracePath, ".json") + ".md"),
					"visibleTools": []string{
						"read_file",
					},
				}),
			},
		},
	})
	reportPath := filepath.Join(evalDir, "report.json")
	writeTestJSON(t, reportPath, eval.Report{
		RunID:    "eval-run",
		Agent:    "server",
		CasesDir: casesDir,
		Summary:  eval.ReportSummary{Total: 1, Failed: 1, AvgScore: 0.5},
		Cases: []eval.CaseScore{{
			CaseID:        "tool-case",
			Category:      "prompt",
			Priority:      "P0",
			Passed:        false,
			Score:         0.5,
			TotalChecks:   1,
			TurnItemsPath: turnItemsPath,
			Checks: []eval.CheckResult{{
				Name:    "expectedToolCalls",
				Passed:  false,
				Missing: []string{"read_file"},
			}},
		}},
	})

	diagnosis, err := Diagnose(context.Background(), Config{
		ReportPath:  reportPath,
		CasesDir:    casesDir,
		TraceDir:    traceDir,
		GeneratedAt: fixedTime(),
	})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if len(diagnosis.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(diagnosis.Cases))
	}
	got := diagnosis.Cases[0]
	if got.LikelyRootCause != RootCausePromptOrToolDescription {
		t.Fatalf("root cause = %q, want %q; hits=%#v", got.LikelyRootCause, RootCausePromptOrToolDescription, got.RuleHits)
	}
	if len(got.Evidence.TraceFiles) != 1 || !got.Evidence.HasUserMessage || got.Evidence.PromptSizeChars == 0 {
		t.Fatalf("trace evidence not populated: %#v", got.Evidence)
	}
	if got.Evidence.TraceTurnCount != 1 || got.Evidence.TraceIterationCount != 1 {
		t.Fatalf("trace turn summary = turns %d iterations %d, want 1/1", got.Evidence.TraceTurnCount, got.Evidence.TraceIterationCount)
	}
	if len(got.Suggestions) == 0 || got.Suggestions[0].Area != "prompt/tool_description" {
		t.Fatalf("suggestions = %#v, want prompt/tool_description", got.Suggestions)
	}
}

func TestDiagnoseReadsAnswerSummaryAndMatchesTraceByFingerprint(t *testing.T) {
	workspace := t.TempDir()
	traceDir := filepath.Join(workspace, "traces")
	evalDir := filepath.Join(workspace, "eval")
	fp := map[string]string{"stableHash": "stable", "developerHash": "dev", "toolRegistryHash": "tools"}
	tracePath := filepath.Join(traceDir, "custom-session", "turn-1", "iteration-000.json")
	writeTestJSON(t, tracePath, map[string]any{
		"schemaVersion":     "aiops.trace/v2",
		"createdAt":         "2026-05-03T00:00:00Z",
		"sessionId":         "custom-session",
		"turnId":            "turn-1",
		"iteration":         0,
		"promptFingerprint": fp,
		"stepContext": map[string]any{
			"modelInput": []map[string]string{{"providerRole": "user", "content": "hello"}},
		},
	})
	answerPath := filepath.Join(evalDir, "case-1", "answer.txt")
	if err := os.MkdirAll(filepath.Dir(answerPath), 0o755); err != nil {
		t.Fatalf("mkdir answer dir: %v", err)
	}
	if err := os.WriteFile(answerPath, []byte("第一行回答\n第二行证据\n第三行更多细节\n"), 0o644); err != nil {
		t.Fatalf("write answer: %v", err)
	}
	reportPath := filepath.Join(evalDir, "report.json")
	writeTestJSON(t, reportPath, eval.Report{
		RunID:   "run-1",
		Summary: eval.ReportSummary{Total: 1, Passed: 1, AvgScore: 1},
		Cases: []eval.CaseScore{{
			CaseID:             "case-1",
			Passed:             true,
			Score:              1,
			AnswerPath:         answerPath,
			PromptFingerprints: []map[string]string{fp},
		}},
	})
	diagnosis, err := Diagnose(context.Background(), Config{
		ReportPath:  reportPath,
		TraceDir:    traceDir,
		GeneratedAt: fixedTime(),
	})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	got := diagnosis.Cases[0].Evidence
	if got.AnswerCharCount == 0 || !strings.Contains(got.AnswerPreview, "第一行回答") {
		t.Fatalf("answer summary not populated: %#v", got)
	}
	if len(got.TraceFiles) != 1 || got.TraceFiles[0].SessionID != "custom-session" {
		t.Fatalf("fingerprint trace match failed: %#v", got.TraceFiles)
	}
}

func TestDiagnoseFlagsRegressionAgainstBaseline(t *testing.T) {
	workspace := t.TempDir()
	currentPath := filepath.Join(workspace, "current.json")
	baselinePath := filepath.Join(workspace, "baseline.json")
	fp := map[string]string{"stableHash": "same", "developerHash": "same-dev", "toolRegistryHash": "same-tool"}
	writeTestJSON(t, baselinePath, eval.Report{Cases: []eval.CaseScore{{
		CaseID:             "case-1",
		Passed:             true,
		Score:              1,
		PromptFingerprints: []map[string]string{fp},
		Checks:             []eval.CheckResult{{Name: "mustInclude", Passed: true}},
	}}})
	writeTestJSON(t, currentPath, eval.Report{
		RunID:   "current",
		Summary: eval.ReportSummary{Total: 1, Failed: 1, AvgScore: 0},
		Cases: []eval.CaseScore{{
			CaseID:             "case-1",
			Passed:             false,
			Score:              0,
			PromptFingerprints: []map[string]string{fp},
			Checks:             []eval.CheckResult{{Name: "mustInclude", Passed: false}},
		}},
	})

	diagnosis, err := Diagnose(context.Background(), Config{
		ReportPath:   currentPath,
		BaselinePath: baselinePath,
		TraceDir:     filepath.Join(workspace, "missing-traces"),
		GeneratedAt:  fixedTime(),
	})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	if diagnosis.Summary.Worse != 1 {
		t.Fatalf("worse = %d, want 1", diagnosis.Summary.Worse)
	}
	if diagnosis.Cases[0].LikelyRootCause != RootCauseRegression {
		t.Fatalf("root cause = %q, want regression; hits=%#v", diagnosis.Cases[0].LikelyRootCause, diagnosis.Cases[0].RuleHits)
	}
}

func TestWriteOutputsDoesNotPersistPromptContent(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	diagnosis := RunDiagnosis{
		RunID:       "run-1",
		GeneratedAt: fixedTime(),
		Summary: DiagnosisSummary{
			Total:           1,
			Failed:          1,
			RootCauseCounts: map[string]int{RootCausePrompt: 1},
		},
		Cases: []CaseDiagnosis{{
			CaseID:          "case-1",
			LikelyRootCause: RootCausePrompt,
			RuleHits: []RuleHit{{
				RuleID:    "answer-content",
				Severity:  "warning",
				RootCause: RootCausePrompt,
				Message:   "最终回答内容不符合断言。",
			}},
			Suggestions: []Suggestion{{Area: "prompt", Action: "改一条规则"}},
		}},
		Suggestions: []Suggestion{{Area: "prompt", Action: "改一条规则"}},
	}
	if err := WriteOutputs(outDir, diagnosis); err != nil {
		t.Fatalf("WriteOutputs: %v", err)
	}
	for _, name := range []string{"diagnosis.json", "diagnosis.zh.md", "compare.zh.md", "trace-links.md", "suggestions.zh.md", "failed-cases.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestWriteDraftCasesCreatesLocalDraftForFailedCase(t *testing.T) {
	workspace := t.TempDir()
	casesDir := filepath.Join(workspace, "cases")
	evalDir := filepath.Join(workspace, "eval")
	draftDir := filepath.Join(workspace, "drafts")
	writeTestJSON(t, filepath.Join(casesDir, "case-1.json"), eval.Case{
		ID:       "case-1",
		Category: "prompt",
		Priority: "P0",
		Input:    "请检查本地文件",
	})
	answerPath := filepath.Join(evalDir, "case-1", "answer.txt")
	if err := os.MkdirAll(filepath.Dir(answerPath), 0o755); err != nil {
		t.Fatalf("mkdir answer dir: %v", err)
	}
	if err := os.WriteFile(answerPath, []byte("需要调用 read_file 并说明证据。\n"), 0o644); err != nil {
		t.Fatalf("write answer: %v", err)
	}
	toolCallsPath := filepath.Join(evalDir, "case-1", "tool_calls.json")
	writeTestJSON(t, toolCallsPath, []eval.ToolCall{{ID: "call-1", Name: "read_file"}})
	turnItemsPath := filepath.Join(evalDir, "case-1", "turn_items.json")
	writeTestJSON(t, turnItemsPath, []agentstate.TurnItem{{Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted}})
	reportPath := filepath.Join(evalDir, "report.json")
	writeTestJSON(t, reportPath, eval.Report{
		RunID:   "run-1",
		Summary: eval.ReportSummary{Total: 1, Failed: 1},
		Cases: []eval.CaseScore{{
			CaseID:        "case-1",
			Category:      "prompt",
			Priority:      "P0",
			Passed:        false,
			Score:         0,
			AnswerPath:    answerPath,
			ToolCallsPath: toolCallsPath,
			TurnItemsPath: turnItemsPath,
			Checks:        []eval.CheckResult{{Name: "mustInclude", Passed: false}},
		}},
	})
	diagnosis, err := Diagnose(context.Background(), Config{
		ReportPath:  reportPath,
		CasesDir:    casesDir,
		TraceDir:    filepath.Join(workspace, "traces"),
		GeneratedAt: fixedTime(),
	})
	if err != nil {
		t.Fatalf("Diagnose: %v", err)
	}
	written, err := WriteDraftCases(context.Background(), Config{
		ReportPath: reportPath,
		CasesDir:   casesDir,
	}, diagnosis, draftDir)
	if err != nil {
		t.Fatalf("WriteDraftCases: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("written = %v, want one draft", written)
	}
	if _, err := os.Stat(filepath.Join(draftDir, "case-1.json.draft.md")); err != nil {
		t.Fatalf("expected sidecar: %v", err)
	}
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func rawJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	return data
}

func fixedTime() time.Time {
	return time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
}
