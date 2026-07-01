package promptcompiler

import (
	"strings"
	"testing"
)

func TestProfileFragmentsNoCrossProfileLeakage(t *testing.T) {
	cases := []struct {
		name      string
		ctx       CompileContext
		sectionID string
		forbidden []string
	}{
		{
			name:      "advisor excludes host and RCA internals",
			ctx:       CompileContext{Mode: "chat", Profile: PromptProfileAdvisor},
			sectionID: "profile.advisor",
			forbidden: []string{
				"runtime approval",
				"database recovery",
				"replication",
				"exec_command",
				"host-bound child",
				"Delegate clear sub-tasks",
			},
		},
		{
			name:      "evidence RCA excludes delegation and mutation approval",
			ctx:       CompileContext{Mode: "inspect", Profile: PromptProfileEvidenceRCA},
			sectionID: "profile.evidence_rca",
			forbidden: []string{
				"host-bound child",
				"Delegate clear sub-tasks",
				"runtime approval",
				"Mutations require",
			},
		},
		{
			name:      "host worker excludes public web methodology and manager delegation",
			ctx:       CompileContext{SessionType: "host", Mode: "execute", Profile: PromptProfileHostWorker, HostContext: "host-a"},
			sectionID: "profile.host_worker",
			forbidden: []string{
				"safe public sources",
				"current external facts",
				"public web",
				"Delegate clear sub-tasks",
				"child agents",
			},
		},
		{
			name:      "host manager excludes direct host command execution",
			ctx:       CompileContext{SessionType: "host", Mode: "execute", Profile: PromptProfileHostManager, HostContext: "host-a"},
			sectionID: "profile.host_manager",
			forbidden: []string{
				"exec_command",
				"Operate only within the bound host scope",
				"Use read-only inspection before any risky action",
				"verification after the action",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled, err := NewCompiler().Compile(tc.ctx)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			section := compiledPromptSectionForTest(t, compiled, tc.sectionID)
			for _, forbidden := range tc.forbidden {
				if strings.Contains(section.Content, forbidden) {
					t.Fatalf("%s leaked %q:\n%s", tc.sectionID, forbidden, section.Content)
				}
			}
		})
	}
}

func compiledPromptSectionForTest(t *testing.T, compiled CompiledPrompt, id string) PromptCompiledSection {
	t.Helper()
	for _, section := range compiled.Envelope.Sections {
		if section.ID == id {
			return section
		}
	}
	t.Fatalf("compiled prompt missing section %q; sections=%v", id, compiledSectionIDsForTest(compiled.Envelope.Sections))
	return PromptCompiledSection{}
}

func compiledSectionIDsForTest(sections []PromptCompiledSection) []string {
	ids := make([]string, 0, len(sections))
	for _, section := range sections {
		ids = append(ids, section.ID)
	}
	return ids
}
