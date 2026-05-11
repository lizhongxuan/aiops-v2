package visual

import (
	"time"

	"runner/state"
	"runner/workflow"
)

const (
	GraphVersion = "v1"

	NodeTypeStart              NodeType = "start"
	NodeTypeAction             NodeType = "action"
	NodeTypeCondition          NodeType = "condition"
	NodeTypeParallel           NodeType = "parallel"
	NodeTypeJoin               NodeType = "join"
	NodeTypeLoop               NodeType = "loop"
	NodeTypeHandler            NodeType = "handler"
	NodeTypeGroup              NodeType = "group"
	NodeTypeSubflow            NodeType = "subflow"
	NodeTypeManualApproval     NodeType = "manual_approval"
	NodeTypeVariableAggregator NodeType = "variable_aggregator"
	NodeTypeEnd                NodeType = "end"

	EdgeKindNext             EdgeKind = "next"
	EdgeKindSuccess          EdgeKind = "success"
	EdgeKindFailure          EdgeKind = "failure"
	EdgeKindCondition        EdgeKind = "condition"
	EdgeKindIf               EdgeKind = "if"
	EdgeKindElse             EdgeKind = "else"
	EdgeKindAlways           EdgeKind = "always"
	EdgeKindApprovalApproved EdgeKind = "approval_approved"
	EdgeKindApprovalRejected EdgeKind = "approval_rejected"
)

type NodeType string

type EdgeKind string

type Graph struct {
	Version  string            `json:"version" yaml:"version"`
	Workflow workflow.Workflow `json:"workflow" yaml:"workflow"`
	Layout   Layout            `json:"layout,omitempty" yaml:"layout,omitempty"`
	Nodes    []Node            `json:"nodes" yaml:"nodes"`
	Edges    []Edge            `json:"edges" yaml:"edges"`
	UI       map[string]any    `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type Overlay struct {
	RunID  string                  `json:"run_id,omitempty"`
	Status string                  `json:"status,omitempty"`
	Nodes  map[string]NodeRunState `json:"nodes,omitempty"`
	Edges  map[string]EdgeRunState `json:"edges,omitempty"`
}

type Layout struct {
	Direction string         `json:"direction,omitempty" yaml:"direction,omitempty"`
	Viewport  Viewport       `json:"viewport,omitempty" yaml:"viewport,omitempty"`
	UI        map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type Viewport struct {
	X    float64 `json:"x" yaml:"x"`
	Y    float64 `json:"y" yaml:"y"`
	Zoom float64 `json:"zoom" yaml:"zoom"`
}

type Position struct {
	X float64 `json:"x" yaml:"x"`
	Y float64 `json:"y" yaml:"y"`
}

type Node struct {
	ID          string                  `json:"id" yaml:"id"`
	Type        NodeType                `json:"type" yaml:"type"`
	Position    Position                `json:"position" yaml:"position"`
	StepName    string                  `json:"step_name,omitempty" yaml:"step_name,omitempty"`
	StepID      string                  `json:"step_id,omitempty" yaml:"step_id,omitempty"`
	Step        *workflow.Step          `json:"step,omitempty" yaml:"step,omitempty"`
	HandlerName string                  `json:"handler_name,omitempty" yaml:"handler_name,omitempty"`
	Handler     *workflow.Handler       `json:"handler,omitempty" yaml:"handler,omitempty"`
	Approval    *ApprovalSpec           `json:"approval,omitempty" yaml:"approval,omitempty"`
	Condition   *ConditionSpec          `json:"condition,omitempty" yaml:"condition,omitempty"`
	Subflow     *SubflowSpec            `json:"subflow,omitempty" yaml:"subflow,omitempty"`
	Aggregator  *VariableAggregatorSpec `json:"aggregator,omitempty" yaml:"aggregator,omitempty"`
	Join        *JoinSpec               `json:"join,omitempty" yaml:"join,omitempty"`
	Loop        *LoopSpec               `json:"loop,omitempty" yaml:"loop,omitempty"`
	ParentID    string                  `json:"parent_id,omitempty" yaml:"parent_id,omitempty"`
	Label       string                  `json:"label,omitempty" yaml:"label,omitempty"`
	Collapsed   bool                    `json:"collapsed,omitempty" yaml:"collapsed,omitempty"`
	Ports       []Port                  `json:"ports,omitempty" yaml:"ports,omitempty"`
	Inputs      []InputParamSpec        `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs     []OutputParamSpec       `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	UI          map[string]any          `json:"ui,omitempty" yaml:"ui,omitempty"`
	State       *NodeRunState           `json:"state,omitempty" yaml:"-"`
}

type Port struct {
	ID       string         `json:"id" yaml:"id"`
	Type     string         `json:"type" yaml:"type"`
	Label    string         `json:"label,omitempty" yaml:"label,omitempty"`
	Required bool           `json:"required,omitempty" yaml:"required,omitempty"`
	UI       map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type ApprovalSpec struct {
	Subjects  []string       `json:"subjects,omitempty" yaml:"subjects,omitempty"`
	Timeout   string         `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	OnTimeout string         `json:"on_timeout,omitempty" yaml:"on_timeout,omitempty"`
	UI        map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type ConditionSpec struct {
	If   string                `json:"if,omitempty" yaml:"if,omitempty"`
	Elif []ConditionBranchSpec `json:"elif,omitempty" yaml:"elif,omitempty"`
	Else bool                  `json:"else,omitempty" yaml:"else,omitempty"`
	UI   map[string]any        `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type ConditionBranchSpec struct {
	Expression string         `json:"expression,omitempty" yaml:"expression,omitempty"`
	UI         map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type SubflowSpec struct {
	WorkflowName string         `json:"workflow_name,omitempty" yaml:"workflow_name,omitempty"`
	Vars         map[string]any `json:"vars,omitempty" yaml:"vars,omitempty"`
	UI           map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type VariableAggregatorSpec struct {
	OutputKey string                         `json:"output_key,omitempty" yaml:"output_key,omitempty"`
	Strategy  string                         `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	Sources   []VariableAggregatorSourceSpec `json:"sources,omitempty" yaml:"sources,omitempty"`
	UI        map[string]any                 `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type VariableAggregatorSourceSpec struct {
	Expression string       `json:"expression,omitempty" yaml:"expression,omitempty"`
	Variable   *VariableRef `json:"variable,omitempty" yaml:"variable,omitempty"`
}

type JoinSpec struct {
	Strategy         string         `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	FailureThreshold int            `json:"failure_threshold,omitempty" yaml:"failure_threshold,omitempty"`
	UI               map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type LoopSpec struct {
	Mode           string         `json:"mode,omitempty" yaml:"mode,omitempty"`
	Items          []any          `json:"items,omitempty" yaml:"items,omitempty"`
	ItemsVariable  string         `json:"items_variable,omitempty" yaml:"items_variable,omitempty"`
	WhileCondition string         `json:"while_condition,omitempty" yaml:"while_condition,omitempty"`
	MaxIterations  int            `json:"max_iterations,omitempty" yaml:"max_iterations,omitempty"`
	ItemVar        string         `json:"item_var,omitempty" yaml:"item_var,omitempty"`
	IndexVar       string         `json:"index_var,omitempty" yaml:"index_var,omitempty"`
	UI             map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type InputParamSpec struct {
	Key         string         `json:"key" yaml:"key"`
	Type        string         `json:"type,omitempty" yaml:"type,omitempty"`
	Label       string         `json:"label,omitempty" yaml:"label,omitempty"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool           `json:"required,omitempty" yaml:"required,omitempty"`
	Default     any            `json:"default,omitempty" yaml:"default,omitempty"`
	ValueSource ValueSource    `json:"value_source,omitempty" yaml:"value_source,omitempty"`
	UI          map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type OutputParamSpec struct {
	Key           string         `json:"key" yaml:"key"`
	Type          string         `json:"type,omitempty" yaml:"type,omitempty"`
	Label         string         `json:"label,omitempty" yaml:"label,omitempty"`
	Description   string         `json:"description,omitempty" yaml:"description,omitempty"`
	Required      bool           `json:"required,omitempty" yaml:"required,omitempty"`
	ExtractSource ExtractSource  `json:"extract_source,omitempty" yaml:"extract_source,omitempty"`
	UI            map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
}

type ValueSource struct {
	Type       string       `json:"type,omitempty" yaml:"type,omitempty"`
	Value      any          `json:"value,omitempty" yaml:"value,omitempty"`
	Variable   *VariableRef `json:"variable,omitempty" yaml:"variable,omitempty"`
	Expression string       `json:"expression,omitempty" yaml:"expression,omitempty"`
	SecretRef  string       `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
	EnvKey     string       `json:"env_key,omitempty" yaml:"env_key,omitempty"`
}

type ExtractSource struct {
	Type       string `json:"type,omitempty" yaml:"type,omitempty"`
	Path       string `json:"path,omitempty" yaml:"path,omitempty"`
	Expression string `json:"expression,omitempty" yaml:"expression,omitempty"`
	Value      any    `json:"value,omitempty" yaml:"value,omitempty"`
}

type VariableRef struct {
	Scope  string `json:"scope,omitempty" yaml:"scope,omitempty"`
	NodeID string `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Path   string `json:"path,omitempty" yaml:"path,omitempty"`
}

type Edge struct {
	ID         string         `json:"id" yaml:"id"`
	Source     string         `json:"source" yaml:"source"`
	SourcePort string         `json:"source_port,omitempty" yaml:"source_port,omitempty"`
	Target     string         `json:"target" yaml:"target"`
	TargetPort string         `json:"target_port,omitempty" yaml:"target_port,omitempty"`
	Kind       EdgeKind       `json:"kind,omitempty" yaml:"kind,omitempty"`
	Condition  string         `json:"condition,omitempty" yaml:"condition,omitempty"`
	UI         map[string]any `json:"ui,omitempty" yaml:"ui,omitempty"`
	State      *EdgeRunState  `json:"state,omitempty" yaml:"-"`
}

type NodeRunState struct {
	RunID      string                      `json:"run_id,omitempty"`
	Status     string                      `json:"status,omitempty"`
	Message    string                      `json:"message,omitempty"`
	StartedAt  time.Time                   `json:"started_at,omitempty"`
	FinishedAt time.Time                   `json:"finished_at,omitempty"`
	Hosts      map[string]state.HostResult `json:"hosts,omitempty"`
	Output     map[string]any              `json:"output,omitempty"`
}

type EdgeRunState struct {
	RunID      string    `json:"run_id,omitempty"`
	Status     string    `json:"status,omitempty"`
	Message    string    `json:"message,omitempty"`
	SelectedAt time.Time `json:"selected_at,omitempty"`
}

func executableNodeType(t NodeType) bool {
	switch t {
	case NodeTypeAction, NodeTypeCondition, NodeTypeParallel, NodeTypeJoin, NodeTypeLoop, NodeTypeSubflow, NodeTypeManualApproval, NodeTypeVariableAggregator, NodeTypeEnd:
		return true
	default:
		return false
	}
}

func continuationRequiredNodeType(t NodeType) bool {
	switch t {
	case NodeTypeStart, NodeTypeAction, NodeTypeCondition, NodeTypeParallel, NodeTypeJoin, NodeTypeLoop, NodeTypeSubflow, NodeTypeManualApproval, NodeTypeVariableAggregator:
		return true
	default:
		return false
	}
}

func stepBackedNodeType(t NodeType) bool {
	switch t {
	case NodeTypeAction, NodeTypeCondition, NodeTypeSubflow, NodeTypeManualApproval:
		return true
	default:
		return false
	}
}

func validNodeType(t NodeType) bool {
	switch t {
	case NodeTypeStart, NodeTypeAction, NodeTypeCondition, NodeTypeParallel, NodeTypeJoin, NodeTypeLoop, NodeTypeHandler, NodeTypeGroup, NodeTypeSubflow, NodeTypeManualApproval, NodeTypeVariableAggregator, NodeTypeEnd:
		return true
	default:
		return false
	}
}

func validEdgeKind(k EdgeKind) bool {
	switch k {
	case "", EdgeKindNext, EdgeKindSuccess, EdgeKindFailure, EdgeKindCondition, EdgeKindIf, EdgeKindElse, EdgeKindAlways, EdgeKindApprovalApproved, EdgeKindApprovalRejected:
		return true
	default:
		return false
	}
}
