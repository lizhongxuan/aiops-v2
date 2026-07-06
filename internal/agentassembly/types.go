package agentassembly

import "aiops-v2/internal/resourcebinding"

type Lifecycle string

const (
	LifecycleSessionScope  Lifecycle = "session_scope"
	LifecycleTurnScope     Lifecycle = "turn_scope"
	LifecycleRequestScope  Lifecycle = "request_scope"
	LifecycleDispatchScope Lifecycle = "dispatch_scope"
)

type AgentAssemblySnapshot struct {
	AgentKind         string                                    `json:"agentKind,omitempty"`
	Profile           string                                    `json:"profile,omitempty"`
	RuntimeRole       string                                    `json:"runtimeRole,omitempty"`
	RouteReason       []string                                  `json:"routeReason,omitempty"`
	ResourceBindings  []resourcebinding.ResourceBindingSnapshot `json:"resourceBindings,omitempty"`
	SessionTargets    []resourcebinding.ResourceRef             `json:"sessionTargets,omitempty"`
	RoleBindings      []resourcebinding.ResourceRoleBinding     `json:"roleBindings,omitempty"`
	ToolSurface       ToolSurfaceSnapshot                       `json:"toolSurface,omitempty"`
	ContextSelector   ContextSelectorSnapshot                   `json:"contextSelector,omitempty"`
	PromptSections    PromptSectionSnapshot                     `json:"promptSections,omitempty"`
	LoopPolicy        LoopPolicySnapshot                        `json:"loopPolicy,omitempty"`
	FinalContract     FinalContractSnapshot                     `json:"finalContract,omitempty"`
	ProfilePromptHash string                                    `json:"profilePromptHash,omitempty"`
	SpecHash          string                                    `json:"specHash,omitempty"`
	TraceTags         map[string]string                         `json:"traceTags,omitempty"`
	Lifecycle         Lifecycle                                 `json:"lifecycle,omitempty"`
}

type ContextSelectorSnapshot struct {
	Lifecycle Lifecycle `json:"lifecycle,omitempty"`
	Policy    string    `json:"policy,omitempty"`
	Budget    string    `json:"budget,omitempty"`
	Hash      string    `json:"hash,omitempty"`
}

type LoopPolicySnapshot struct {
	Lifecycle      Lifecycle `json:"lifecycle,omitempty"`
	MaxIterations  int       `json:"maxIterations,omitempty"`
	ToolCallPolicy string    `json:"toolCallPolicy,omitempty"`
	Hash           string    `json:"hash,omitempty"`
}

type FinalContractSnapshot struct {
	Lifecycle Lifecycle `json:"lifecycle,omitempty"`
	Shape     string    `json:"shape,omitempty"`
	Hash      string    `json:"hash,omitempty"`
}
