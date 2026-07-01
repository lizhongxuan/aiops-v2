package promptcompiler

import (
	"strings"
	"testing"
)

func TestProfileFragmentsRenderOnlySelectedProfile(t *testing.T) {
	cases := []struct {
		profile   string
		wants     []string
		forbidden []string
	}{
		{
			profile: "advisor",
			wants: []string{
				"## Profile Rules",
				"Answer advisory questions from provided context",
				"State when no host, OpsGraph, Coroot, or OpsManual context",
				"Do not run host commands.",
			},
			forbidden: []string{
				"Build a concise incident timeline",
				"Operate only within the bound host scope",
				"Delegate clear sub-tasks",
			},
		},
		{
			profile: "evidence_rca",
			wants: []string{
				"## Profile Rules",
				"Build a concise incident timeline",
				"Separate observed facts, hypotheses, missing evidence, and assumptions.",
				"Do not run host commands unless this turn has explicit host scope and visible host tools.",
			},
			forbidden: []string{
				"Answer advisory questions",
				"Operate only within the bound host scope",
				"Delegate clear sub-tasks",
			},
		},
		{
			profile: "host_worker",
			wants: []string{
				"## Profile Rules",
				"Operate only within the bound host scope: host-a.",
				"Use read-only inspection before any risky action.",
				"Mutations require runtime approval and verification after the action.",
			},
			forbidden: []string{
				"Answer advisory questions",
				"Build a concise incident timeline",
				"Delegate clear sub-tasks",
			},
		},
		{
			profile: "host_manager",
			wants: []string{
				"## Profile Rules",
				"Create a compact plan for complex host work before delegation.",
				"Do not run host commands directly.",
				"Delegate clear sub-tasks to host-bound child agents",
			},
			forbidden: []string{
				"Answer advisory questions",
				"Build a concise incident timeline",
				"Operate only within the bound host scope",
				"exec_command",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.profile, func(t *testing.T) {
			compiled, err := NewCompiler().Compile(CompileContext{Mode: "execute", Profile: tc.profile, HostContext: "host-a"})
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			content := compiled.Developer.Content
			for _, want := range tc.wants {
				if !strings.Contains(content, want) {
					t.Fatalf("profile fragment missing %q:\n%s", want, content)
				}
			}
			for _, forbidden := range tc.forbidden {
				if strings.Contains(content, forbidden) {
					t.Fatalf("profile fragment leaked %q:\n%s", forbidden, content)
				}
			}
		})
	}
}
