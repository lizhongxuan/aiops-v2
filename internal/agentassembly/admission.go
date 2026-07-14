package agentassembly

import "strings"

const (
	RuntimeRoleProfileOnly  = "profile_only"
	RuntimeRoleRuntimeAgent = "runtime_agent"
)

type AgentKindAdmissionSpec struct {
	AgentKind          string   `json:"agentKind,omitempty"`
	Profile            string   `json:"profile,omitempty"`
	RuntimeRole        string   `json:"runtimeRole,omitempty"`
	Route              string   `json:"route,omitempty"`
	ToolSurface        string   `json:"toolSurface,omitempty"`
	ContextSelector    string   `json:"contextSelector,omitempty"`
	LoopPolicy         string   `json:"loopPolicy,omitempty"`
	FinalContract      string   `json:"finalContract,omitempty"`
	EvalSuite          string   `json:"evalSuite,omitempty"`
	ProfileSections    []string `json:"profileSections,omitempty"`
	LeakageTests       []string `json:"leakageTests,omitempty"`
	PromptCompiler     string   `json:"promptCompiler,omitempty"`
	ToolRegistry       string   `json:"toolRegistry,omitempty"`
	RuntimeLoop        string   `json:"runtimeLoop,omitempty"`
	LearnedAssetScopes []string `json:"learnedAssetScopes,omitempty"`
}

type AgentKindAdmissionResult struct {
	Passed          bool     `json:"passed"`
	MissingFields   []string `json:"missingFields,omitempty"`
	ForbiddenFields []string `json:"forbiddenFields,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
}

func ValidateAgentKindAdmission(spec AgentKindAdmissionSpec) AgentKindAdmissionResult {
	spec = normalizeAgentKindAdmissionSpec(spec)
	var missing []string
	var forbidden []string
	var reasons []string

	requireString := func(field, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, field)
		}
	}
	requireList := func(field string, values []string) {
		if len(compactAdmissionStrings(values)) == 0 {
			missing = append(missing, field)
		}
	}
	forbidString := func(field, value string) {
		if strings.TrimSpace(value) != "" {
			forbidden = append(forbidden, field)
		}
	}

	requireString("agentKind", spec.AgentKind)
	requireString("profile", spec.Profile)
	requireString("runtimeRole", spec.RuntimeRole)
	requireString("finalContract", spec.FinalContract)
	requireString("evalSuite", spec.EvalSuite)

	switch spec.RuntimeRole {
	case RuntimeRoleProfileOnly:
		requireList("profileSections", spec.ProfileSections)
		requireList("leakageTests", spec.LeakageTests)
		forbidString("route", spec.Route)
		forbidString("toolSurface", spec.ToolSurface)
		forbidString("contextSelector", spec.ContextSelector)
		forbidString("loopPolicy", spec.LoopPolicy)
	case RuntimeRoleRuntimeAgent:
		requireString("route", spec.Route)
		requireString("toolSurface", spec.ToolSurface)
		requireString("contextSelector", spec.ContextSelector)
		requireString("loopPolicy", spec.LoopPolicy)
	case "":
		// Already covered by missing runtimeRole.
	default:
		forbidden = append(forbidden, "runtimeRole")
		reasons = append(reasons, "runtime_role_must_be_profile_only_or_runtime_agent")
	}

	forbidString("promptCompiler", spec.PromptCompiler)
	forbidString("toolRegistry", spec.ToolRegistry)
	forbidString("runtimeLoop", spec.RuntimeLoop)
	if hasGlobalLearnedAssetScope(spec.LearnedAssetScopes) {
		forbidden = append(forbidden, "learnedAssetScopes")
		reasons = append(reasons, "learned_assets_must_be_scoped_by_agent_profile_route_or_resource")
	}

	missing = uniqueSortedStrings(missing)
	forbidden = uniqueSortedStrings(forbidden)
	reasons = uniqueSortedStrings(reasons)
	return AgentKindAdmissionResult{
		Passed:          len(missing) == 0 && len(forbidden) == 0,
		MissingFields:   missing,
		ForbiddenFields: forbidden,
		Reasons:         reasons,
	}
}

func normalizeAgentKindAdmissionSpec(spec AgentKindAdmissionSpec) AgentKindAdmissionSpec {
	spec.AgentKind = strings.TrimSpace(spec.AgentKind)
	spec.Profile = strings.TrimSpace(spec.Profile)
	spec.RuntimeRole = strings.ToLower(strings.TrimSpace(spec.RuntimeRole))
	spec.Route = strings.TrimSpace(spec.Route)
	spec.ToolSurface = strings.TrimSpace(spec.ToolSurface)
	spec.ContextSelector = strings.TrimSpace(spec.ContextSelector)
	spec.LoopPolicy = strings.TrimSpace(spec.LoopPolicy)
	spec.FinalContract = strings.TrimSpace(spec.FinalContract)
	spec.EvalSuite = strings.TrimSpace(spec.EvalSuite)
	spec.ProfileSections = compactAdmissionStrings(spec.ProfileSections)
	spec.LeakageTests = compactAdmissionStrings(spec.LeakageTests)
	spec.PromptCompiler = strings.TrimSpace(spec.PromptCompiler)
	spec.ToolRegistry = strings.TrimSpace(spec.ToolRegistry)
	spec.RuntimeLoop = strings.TrimSpace(spec.RuntimeLoop)
	spec.LearnedAssetScopes = compactAdmissionStrings(spec.LearnedAssetScopes)
	return spec
}

func compactAdmissionStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func hasGlobalLearnedAssetScope(scopes []string) bool {
	for _, scope := range scopes {
		normalized := strings.ToLower(strings.TrimSpace(scope))
		switch normalized {
		case "global", "all", "*", "any":
			return true
		}
		if strings.HasPrefix(normalized, "global:") || strings.Contains(normalized, "scope:global") {
			return true
		}
	}
	return false
}
