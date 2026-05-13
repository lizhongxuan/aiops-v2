package workflow

type Workflow struct {
	Version       string         `json:"version" yaml:"version"`
	Name          string         `json:"name" yaml:"name"`
	Description   string         `json:"description" yaml:"description"`
	EnvPackages   []string       `json:"env_packages" yaml:"env_packages"`
	ValidationEnv string         `json:"validation_env" yaml:"validation_env"`
	XRunnerUI     *GraphUISpec   `json:"x_runner_ui,omitempty" yaml:"x_runner_ui,omitempty"`
	XRunnerGraph  *GraphSpec     `json:"x_runner_graph,omitempty" yaml:"x_runner_graph,omitempty"`
	Inventory     Inventory      `json:"inventory" yaml:"inventory"`
	Vars          map[string]any `json:"vars" yaml:"vars"`
	Plan          Plan           `json:"plan" yaml:"plan"`
	Steps         []Step         `json:"steps" yaml:"steps"`
	Handlers      []Handler      `json:"handlers" yaml:"handlers"`
	Tests         []Test         `json:"tests" yaml:"tests"`
	Extensions    map[string]any `json:"-" yaml:",inline"`
}

type Inventory struct {
	Groups map[string]Group `json:"groups" yaml:"groups"`
	Hosts  map[string]Host  `json:"hosts" yaml:"hosts"`
	Vars   map[string]any   `json:"vars" yaml:"vars"`
}

type Group struct {
	Hosts []string       `json:"hosts" yaml:"hosts"`
	Vars  map[string]any `json:"vars" yaml:"vars"`
}

type Host struct {
	Address string         `json:"address" yaml:"address"`
	Vars    map[string]any `json:"vars" yaml:"vars"`
}

type Plan struct {
	Mode     string `json:"mode" yaml:"mode"`
	Strategy string `json:"strategy" yaml:"strategy"`
}

type GraphSpec struct {
	Version string          `json:"version" yaml:"version"`
	Nodes   []GraphNodeSpec `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Edges   []GraphEdgeSpec `json:"edges,omitempty" yaml:"edges,omitempty"`
	UI      map[string]any  `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphUISpec struct {
	Version string          `json:"version" yaml:"version"`
	Layout  GraphLayoutSpec `json:"layout,omitempty" yaml:"layout,omitempty"`
	Nodes   []GraphNodeSpec `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Edges   []GraphEdgeSpec `json:"edges,omitempty" yaml:"edges,omitempty"`
	UI      map[string]any  `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphLayoutSpec struct {
	Direction string         `json:"direction,omitempty" yaml:"direction,omitempty"`
	Viewport  GraphViewport  `json:"viewport,omitempty" yaml:"viewport,omitempty"`
	UI        map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphViewport struct {
	X    float64 `json:"x" yaml:"x"`
	Y    float64 `json:"y" yaml:"y"`
	Zoom float64 `json:"zoom" yaml:"zoom"`
}

type GraphNodeSpec struct {
	ID          string            `json:"id" yaml:"id"`
	Type        string            `json:"type" yaml:"type"`
	Position    GraphPosition     `json:"position,omitempty" yaml:"position,omitempty"`
	Step        string            `json:"step,omitempty" yaml:"step,omitempty"`
	StepName    string            `json:"step_name,omitempty" yaml:"step_name,omitempty"`
	StepID      string            `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	Handler     string            `json:"handler,omitempty" yaml:"handler,omitempty"`
	HandlerName string            `json:"handler_name,omitempty" yaml:"handler_name,omitempty"`
	ParentID    string            `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Label       string            `json:"label,omitempty" yaml:"label,omitempty"`
	Collapsed   bool              `json:"collapsed,omitempty" yaml:"collapsed,omitempty"`
	Ports       []GraphPortSpec   `json:"ports,omitempty" yaml:"ports,omitempty"`
	Inputs      []GraphInputSpec  `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     []GraphOutputSpec `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Data        GraphNodeDataSpec `json:"data,omitempty" yaml:"data,omitempty"`
	UI          map[string]any    `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphNodeDataSpec struct {
	StepName    string                       `json:"stepName,omitempty" yaml:"stepName,omitempty"`
	HandlerName string                       `json:"handlerName,omitempty" yaml:"handlerName,omitempty"`
	Label       string                       `json:"label,omitempty" yaml:"label,omitempty"`
	Collapsed   bool                         `json:"collapsed,omitempty" yaml:"collapsed,omitempty"`
	Approval    *GraphApprovalSpec           `json:"approval,omitempty" yaml:"approval,omitempty"`
	Condition   *GraphConditionSpec          `json:"condition,omitempty" yaml:"condition,omitempty"`
	Subflow     *GraphSubflowSpec            `json:"subflow,omitempty" yaml:"subflow,omitempty"`
	Aggregator  *GraphVariableAggregatorSpec `json:"aggregator,omitempty" yaml:"aggregator,omitempty"`
	Join        *GraphJoinSpec               `json:"join,omitempty" yaml:"join,omitempty"`
	Loop        *GraphLoopSpec               `json:"loop,omitempty" yaml:"loop,omitempty"`
	Inputs      []GraphInputSpec             `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     []GraphOutputSpec            `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	UI          map[string]any               `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphPortSpec struct {
	ID       string         `json:"id" yaml:"id"`
	Type     string         `json:"type" yaml:"type"`
	Label    string         `json:"label,omitempty" yaml:"label,omitempty"`
	Required bool           `json:"required,omitempty" yaml:"required,omitempty"`
	UI       map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphInputSpec struct {
	Key         string               `json:"key" yaml:"key"`
	Type        string               `json:"type,omitempty" yaml:"type,omitempty"`
	Label       string               `json:"label,omitempty" yaml:"label,omitempty"`
	Description string               `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool                 `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any                  `json:"default,omitempty" yaml:"default,omitempty"`
	ValueSource GraphValueSourceSpec `json:"value_source,omitempty" yaml:"value_source,omitempty"`
	UI          map[string]any       `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphOutputSpec struct {
	Key           string                 `json:"key" yaml:"key"`
	Type          string                 `json:"type,omitempty" yaml:"type,omitempty"`
	Label         string                 `json:"label,omitempty" yaml:"label,omitempty"`
	Description   string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Required      bool                   `json:"required,omitempty" yaml:"required,omitempty"`
	ExtractSource GraphExtractSourceSpec `json:"extract_source,omitempty" yaml:"extract_source,omitempty"`
	UI            map[string]any         `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphValueSourceSpec struct {
	Type       string                `json:"type,omitempty" yaml:"type,omitempty"`
	Value      any                   `json:"value,omitempty" yaml:"value,omitempty"`
	Variable   *GraphVariableRefSpec `json:"variable,omitempty" yaml:"variable,omitempty"`
	Expression string                `json:"expression,omitempty" yaml:"expression,omitempty"`
	SecretRef  string                `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
	EnvKey     string                `json:"env_key,omitempty" yaml:"env_key,omitempty"`
}

type GraphExtractSourceSpec struct {
	Type       string `json:"type,omitempty" yaml:"type,omitempty"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
	Expression string `json:"expression,omitempty" yaml:"expression,omitempty"`
	Value      any    `json:"value,omitempty" yaml:"value,omitempty"`
}

type GraphVariableRefSpec struct {
	Scope  string `json:"scope,omitempty" yaml:"scope,omitempty"`
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
}

type GraphPosition struct {
	X float64 `json:"x" yaml:"x"`
	Y float64 `json:"y" yaml:"y"`
}

type GraphEdgeSpec struct {
	ID         string         `json:"id" yaml:"id"`
	Source     string         `json:"source" yaml:"source"`
	SourcePort string         `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	Target     string         `json:"target" yaml:"target"`
	TargetPort string         `json:"target_port,omitempty" yaml:"target_port,omitempty"`
	Kind       string         `json:"kind,omitempty" yaml:"kind,omitempty"`
	Condition  string         `json:"condition,omitempty" yaml:"condition,omitempty"`
	UI         map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphApprovalSpec struct {
	Subjects  []string       `json:"subjects,omitempty" yaml:"subjects,omitempty"`
	Timeout   string         `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	OnTimeout string         `json:"on_timeout,omitempty" yaml:"on_timeout,omitempty"`
	UI        map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphConditionSpec struct {
	If   string                     `json:"if,omitempty" yaml:"if,omitempty"`
	Elif []GraphConditionBranchSpec `json:"elif,omitempty" yaml:"elif,omitempty"`
	Else bool                       `json:"else,omitempty" yaml:"else,omitempty"`
	UI   map[string]any             `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphConditionBranchSpec struct {
	Expression string         `json:"expression,omitempty" yaml:"expression,omitempty"`
	UI         map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphSubflowSpec struct {
	WorkflowName string         `json:"workflow_name,omitempty" yaml:"workflow_name,omitempty"`
	Vars         map[string]any `json:"vars,omitempty" yaml:"vars,omitempty"`
	UI           map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphVariableAggregatorSpec struct {
	OutputKey string                              `json:"output_key,omitempty" yaml:"output_key,omitempty"`
	Strategy  string                              `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	Sources   []GraphVariableAggregatorSourceSpec `json:"sources,omitempty" yaml:"sources,omitempty"`
	UI        map[string]any                      `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphVariableAggregatorSourceSpec struct {
	Expression string                `json:"expression,omitempty" yaml:"expression,omitempty"`
	Variable   *GraphVariableRefSpec `json:"variable,omitempty" yaml:"variable,omitempty"`
}

type GraphJoinSpec struct {
	Strategy         string         `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	FailureThreshold int            `json:"failure_threshold,omitempty" yaml:"failure_threshold,omitempty"`
	UI               map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type GraphLoopSpec struct {
	Mode           string         `json:"mode,omitempty" yaml:"mode,omitempty"`
	Items          []any          `json:"items,omitempty" yaml:"items,omitempty"`
	ItemsVariable  string         `json:"items_variable,omitempty" yaml:"items_variable,omitempty"`
	WhileCondition string         `json:"while_condition,omitempty" yaml:"while_condition,omitempty"`
	MaxIterations  int            `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
	ItemVar        string         `json:"item_var,omitempty" yaml:"item_var,omitempty"`
	IndexVar       string         `json:"index_var,omitempty" yaml:"index_var,omitempty"`
	UI             map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type Step struct {
	Name            string         `json:"name" yaml:"name"`
	ID              string         `json:"id,omitempty" yaml:"id,omitempty"`
	Targets         []string       `json:"targets" yaml:"targets"`
	Action          string         `json:"action" yaml:"action"`
	Args            map[string]any `json:"args" yaml:"args"`
	MustVars        []string       `json:"must_vars" yaml:"must_vars"`
	When            string         `json:"when" yaml:"when"`
	Loop            []any          `json:"loop" yaml:"loop"`
	Retries         int            `json:"retries" yaml:"retries"`
	Timeout         string         `json:"timeout" yaml:"timeout"`
	ContinueOnError bool           `json:"continue_on_error" yaml:"continue_on_error"`
	ExpectVars      []string       `json:"expect_vars" yaml:"expect_vars"`
	Notify          []string       `json:"notify" yaml:"notify"`
}

type Handler struct {
	Name    string         `json:"name" yaml:"name"`
	Action  string         `json:"action" yaml:"action"`
	Args    map[string]any `json:"args" yaml:"args"`
	When    string         `json:"when" yaml:"when"`
	Retries int            `json:"retries" yaml:"retries"`
	Timeout string         `json:"timeout" yaml:"timeout"`
}

type Test struct {
	Name   string         `json:"name" yaml:"name"`
	Action string         `json:"action" yaml:"action"`
	Args   map[string]any `json:"args" yaml:"args"`
}
