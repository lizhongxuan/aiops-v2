package promptcompiler

import (
	"fmt"
	"strings"
	"testing"
)

func TestPromptBaselineBudgetByProfile(t *testing.T) {
	cases := []struct {
		name        string
		ctx         CompileContext
		maxTotal    int
		maxPolicy   int
		maxToolSize int
	}{
		{
			name: "advisor_chat_concise",
			ctx: CompileContext{
				SessionType: "workspace",
				Mode:        "chat",
				Profile:     PromptProfileAdvisor,
				AnswerStyle: "concise",
			},
			maxTotal:  8192,
			maxPolicy: 1024,
		},
		{
			name: "evidence_rca_inspect",
			ctx: CompileContext{
				SessionType: "workspace",
				Mode:        "inspect",
				Profile:     PromptProfileEvidenceRCA,
			},
			maxTotal:  10240,
			maxPolicy: 1024,
		},
		{
			name: "host_worker_execute",
			ctx: CompileContext{
				SessionType: "host",
				Mode:        "execute",
				Profile:     PromptProfileHostWorker,
				HostContext: "host-1",
			},
			maxTotal:  10240,
			maxPolicy: 1024,
		},
		{
			name: "host_manager_execute",
			ctx: CompileContext{
				SessionType:    "workspace",
				Mode:           "execute",
				Profile:        PromptProfileHostManager,
				HostOpsManager: true,
			},
			maxTotal:  10240,
			maxPolicy: 1024,
		},
		{
			name: "tool_surface_ten_tools",
			ctx: CompileContext{
				SessionType:    "workspace",
				Mode:           "inspect",
				Profile:        PromptProfileEvidenceRCA,
				AssembledTools: promptBudgetMockTools(10),
			},
			maxTotal:    12288,
			maxPolicy:   1024,
			maxToolSize: 2048,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled, err := NewCompiler().Compile(tc.ctx)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			total := promptMessagesBytes(compiled)
			if total > tc.maxTotal {
				t.Errorf("compiled prompt bytes = %d, want <= %d", total, tc.maxTotal)
			}
			if got := promptSectionBytes(compiled, "runtime.state"); got > tc.maxPolicy {
				t.Errorf("runtime.state bytes = %d, want <= %d", got, tc.maxPolicy)
			}
			if tc.maxToolSize > 0 {
				if got := promptSectionBytes(compiled, "tool.surface"); got > tc.maxToolSize {
					t.Errorf("tool.surface bytes = %d, want <= %d", got, tc.maxToolSize)
				}
			}
		})
	}
}

func TestAdvisorPromptDoesNotContainHostOpsOrRCAInternals(t *testing.T) {
	compiled := mustCompilePromptBudget(t, CompileContext{
		SessionType: "workspace",
		Mode:        "chat",
		Profile:     PromptProfileAdvisor,
		AnswerStyle: "concise",
	})
	assertPromptExcludes(t, compiled, []string{
		"Host Operations Manager",
		"database recovery",
		"replication RCA",
		"post-change",
		"before mutation",
	})
}

func TestEvidenceRCAPromptDoesNotContainHostManagerDelegation(t *testing.T) {
	compiled := mustCompilePromptBudget(t, CompileContext{
		SessionType: "workspace",
		Mode:        "inspect",
		Profile:     PromptProfileEvidenceRCA,
	})
	assertPromptExcludes(t, compiled, []string{
		"Spawn one host-bound child",
		"host-bound child agent",
		"one host-bound child agent per unique host",
		"主 Agent",
	})
}

func TestHostWorkerPromptDoesNotContainAdvisorWebGuidance(t *testing.T) {
	compiled := mustCompilePromptBudget(t, CompileContext{
		SessionType: "host",
		Mode:        "execute",
		Profile:     PromptProfileHostWorker,
		HostContext: "host-1",
	})
	assertPromptExcludes(t, compiled, []string{
		"public web",
		"public factual requests",
		"official documentation",
		"web/documentation lookups",
	})
}

func TestHostManagerPromptDoesNotContainDirectHostCommandInstructions(t *testing.T) {
	compiled := mustCompilePromptBudget(t, CompileContext{
		SessionType:    "workspace",
		Mode:           "execute",
		Profile:        PromptProfileHostManager,
		HostOpsManager: true,
	})
	assertPromptExcludes(t, compiled, []string{
		"exec_command when it is visible",
		"direct environment-bound inspection with exec_command",
		"pass the executable and args directly",
	})
}

func mustCompilePromptBudget(t *testing.T, ctx CompileContext) CompiledPrompt {
	t.Helper()
	compiled, err := NewCompiler().Compile(ctx)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return compiled
}

func assertPromptExcludes(t *testing.T, compiled CompiledPrompt, forbidden []string) {
	t.Helper()
	content := strings.ToLower(promptCompiledContent(compiled))
	for _, phrase := range forbidden {
		if strings.Contains(content, strings.ToLower(phrase)) {
			t.Errorf("prompt contains forbidden phrase %q", phrase)
		}
	}
}

func promptMessagesBytes(compiled CompiledPrompt) int {
	total := 0
	for _, section := range compiled.Envelope.Sections {
		if section.Content == "" {
			continue
		}
		total += len([]byte(section.Content))
	}
	return total
}

func promptSectionBytes(compiled CompiledPrompt, sectionID string) int {
	for _, section := range compiled.PromptSections {
		if section.ID == sectionID {
			return section.Bytes
		}
	}
	return 0
}

func promptCompiledContent(compiled CompiledPrompt) string {
	var b strings.Builder
	for _, section := range compiled.Envelope.Sections {
		if section.Content == "" {
			continue
		}
		b.WriteString(section.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func promptBudgetMockTools(count int) []Tool {
	tools := make([]Tool, 0, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("budget_tool_%02d", i)
		tools = append(tools, &mockToolRuntime{
			name:                name,
			metadataDescription: "Read operational evidence for " + name + ".",
			desc:                "Read operational evidence for " + name + ".",
			readOnly:            true,
			concurrencySafe:     true,
		})
	}
	return tools
}
