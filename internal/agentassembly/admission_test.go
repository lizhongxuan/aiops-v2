package agentassembly

import "testing"

func TestAgentKindAdmissionAcceptsProfileOnlyAgent(t *testing.T) {
	result := ValidateAgentKindAdmission(AgentKindAdmissionSpec{
		AgentKind:       "writer",
		Profile:         "writer.default",
		RuntimeRole:     RuntimeRoleProfileOnly,
		ProfileSections: []string{"profile.writer"},
		FinalContract:   "structured_writeup",
		EvalSuite:       "writer_profile_p0",
		LeakageTests:    []string{"writer_no_host_exec"},
		LearnedAssetScopes: []string{
			"agent_kind:writer",
			"profile:writer.default",
		},
	})
	if !result.Passed {
		t.Fatalf("result = %#v, want pass", result)
	}
}

func TestAgentKindAdmissionRejectsProfileOnlyRuntimeSurface(t *testing.T) {
	result := ValidateAgentKindAdmission(AgentKindAdmissionSpec{
		AgentKind:       "advisor",
		Profile:         "advisor.ops",
		RuntimeRole:     RuntimeRoleProfileOnly,
		ProfileSections: []string{"profile.advisor"},
		FinalContract:   "advisory_answer",
		EvalSuite:       "advisor_p0",
		LeakageTests:    []string{"advisor_no_host_exec"},
		ToolSurface:     "host_exec_tools",
	})
	if result.Passed || !containsString(result.ForbiddenFields, "toolSurface") {
		t.Fatalf("result = %#v, want profile-only tool surface rejection", result)
	}
}

func TestAgentKindAdmissionRequiresRuntimeAgentContract(t *testing.T) {
	result := ValidateAgentKindAdmission(AgentKindAdmissionSpec{
		AgentKind:   "host_worker",
		Profile:     "host.worker",
		RuntimeRole: RuntimeRoleRuntimeAgent,
		Route:       "host_bound_ops",
		ToolSurface: "host_scoped_exec",
		LoopPolicy:  "bounded_tool_loop",
	})
	if result.Passed {
		t.Fatalf("result = %#v, want missing context/final/eval rejection", result)
	}
	for _, field := range []string{"contextSelector", "finalContract", "evalSuite"} {
		if !containsString(result.MissingFields, field) {
			t.Fatalf("missing fields = %#v, want %s", result.MissingFields, field)
		}
	}
}

func TestAgentKindAdmissionRejectsPrivateRuntimeComponents(t *testing.T) {
	result := ValidateAgentKindAdmission(AgentKindAdmissionSpec{
		AgentKind:       "host_worker",
		Profile:         "host.worker",
		RuntimeRole:     RuntimeRoleRuntimeAgent,
		Route:           "host_bound_ops",
		ToolSurface:     "host_scoped_exec",
		ContextSelector: "host_context",
		LoopPolicy:      "bounded_tool_loop",
		FinalContract:   "host_task_report",
		EvalSuite:       "host_worker_p0",
		PromptCompiler:  "custom_prompt_compiler",
		ToolRegistry:    "custom_tool_registry",
		RuntimeLoop:     "custom_runtime_loop",
	})
	if result.Passed {
		t.Fatalf("result = %#v, want private runtime component rejection", result)
	}
	for _, field := range []string{"promptCompiler", "toolRegistry", "runtimeLoop"} {
		if !containsString(result.ForbiddenFields, field) {
			t.Fatalf("forbidden fields = %#v, want %s", result.ForbiddenFields, field)
		}
	}
}

func TestAgentKindAdmissionRejectsGlobalLearnedAsset(t *testing.T) {
	result := ValidateAgentKindAdmission(AgentKindAdmissionSpec{
		AgentKind:       "advisor",
		Profile:         "advisor.ops",
		RuntimeRole:     RuntimeRoleProfileOnly,
		ProfileSections: []string{"profile.advisor"},
		FinalContract:   "advisory_answer",
		EvalSuite:       "advisor_p0",
		LeakageTests:    []string{"advisor_no_host_exec"},
		LearnedAssetScopes: []string{
			"global",
		},
	})
	if result.Passed || !containsString(result.ForbiddenFields, "learnedAssetScopes") {
		t.Fatalf("result = %#v, want global learned asset rejection", result)
	}
}
