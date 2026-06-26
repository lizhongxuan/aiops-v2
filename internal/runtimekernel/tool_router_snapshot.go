package runtimekernel

type RuntimeToolRouterSnapshot struct {
	RegisteredTools   []string            `json:"registeredTools,omitempty"`
	ModelVisibleTools []string            `json:"modelVisibleTools,omitempty"`
	DispatchableTools []string            `json:"dispatchableTools,omitempty"`
	HiddenReasons     map[string][]string `json:"hiddenReasons,omitempty"`
	PolicyHash        string              `json:"policyHash,omitempty"`
	Fingerprint       string              `json:"fingerprint,omitempty"`
}
