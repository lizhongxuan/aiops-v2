// Package edit owns the canonical, transport-neutral Workflow edit v2 contract.
package edit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"runner/workflow"
	"runner/workflow/visual"
)

const SchemaVersion = "aiops.workflow.edit.v2"

type WorkflowKey struct {
	TenantID   string `json:"tenantId"`
	WorkflowID string `json:"workflowId"`
}

func (k WorkflowKey) Validate() error {
	for name, value := range map[string]string{"tenantId": k.TenantID, "workflowId": k.WorkflowID} {
		trimmed := strings.TrimSpace(value)
		if value != trimmed || trimmed == "" || trimmed == "." || trimmed == ".." || strings.ContainsAny(trimmed, "/\\\x00") {
			return fmt.Errorf("invalid %s", name)
		}
	}
	return nil
}

type Revision struct {
	ResourceVersion string `json:"resourceVersion"`
	SemanticHash    string `json:"semanticHash"`
	LayoutHash      string `json:"layoutHash"`
}

type ValidationBasis struct {
	ResourceVersion           string `json:"resourceVersion"`
	SemanticHash              string `json:"semanticHash"`
	CatalogVersion            string `json:"catalogVersion"`
	ActionSchemaDigest        string `json:"actionSchemaDigest"`
	ReferenceSnapshotHash     string `json:"referenceSnapshotHash"`
	EnvironmentBindingVersion string `json:"environmentBindingVersion"`
}

func (b ValidationBasis) Validate() error {
	for name, value := range map[string]string{
		"resourceVersion": b.ResourceVersion, "semanticHash": b.SemanticHash,
		"catalogVersion": b.CatalogVersion, "actionSchemaDigest": b.ActionSchemaDigest,
		"referenceSnapshotHash": b.ReferenceSnapshotHash, "environmentBindingVersion": b.EnvironmentBindingVersion,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

type WorkflowEditSessionV2 struct {
	SchemaVersion     string      `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	ID                string      `json:"id"`
	Key               WorkflowKey `json:"workflowKey"`
	ActorID           string      `json:"actorId"`
	CanvasSessionID   string      `json:"canvasSessionId"`
	AgentSessionID    string      `json:"agentSessionId,omitempty"`
	TurnID            string      `json:"turnId,omitempty"`
	Status            string      `json:"status"`
	ActiveCandidateID string      `json:"activeCandidateId,omitempty"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
}

type OperationKind string

const (
	OperationAddNode             OperationKind = "add_node"
	OperationUpdateNode          OperationKind = "update_node"
	OperationDeleteNode          OperationKind = "delete_node"
	OperationAddEdge             OperationKind = "add_edge"
	OperationUpdateEdge          OperationKind = "update_edge"
	OperationDeleteEdge          OperationKind = "delete_edge"
	OperationUpdateWorkflow      OperationKind = "update_workflow"
	OperationUpdateLayout        OperationKind = "update_layout"
	OperationAttachManualBinding OperationKind = "attach_manual_binding"
)

type OperationValue interface{ Kind() OperationKind }

type WorkflowOperation struct {
	Value OperationValue `json:"-"`
}

func NewOperation(value OperationValue) WorkflowOperation { return WorkflowOperation{Value: value} }

type AddNodeOperation struct {
	Op        OperationKind `json:"op" jsonschema:"enum=add_node"`
	ClientRef string        `json:"clientRef,omitempty"`
	Node      EditableNode  `json:"node"`
}

func (*AddNodeOperation) Kind() OperationKind { return OperationAddNode }

type NodePatch struct {
	Type        *visual.NodeType               `json:"type,omitempty"`
	Position    *visual.Position               `json:"position,omitempty"`
	StepName    *string                        `json:"stepName,omitempty"`
	StepID      *string                        `json:"stepId,omitempty"`
	Step        *workflow.Step                 `json:"step,omitempty"`
	HandlerName *string                        `json:"handlerName,omitempty"`
	Handler     *workflow.Handler              `json:"handler,omitempty"`
	Approval    *visual.ApprovalSpec           `json:"approval,omitempty"`
	Condition   *visual.ConditionSpec          `json:"condition,omitempty"`
	Subflow     *visual.SubflowSpec            `json:"subflow,omitempty"`
	Aggregator  *visual.VariableAggregatorSpec `json:"aggregator,omitempty"`
	Join        *visual.JoinSpec               `json:"join,omitempty"`
	Loop        *visual.LoopSpec               `json:"loop,omitempty"`
	ParentID    *string                        `json:"parentId,omitempty"`
	Label       *string                        `json:"label,omitempty"`
	Collapsed   *bool                          `json:"collapsed,omitempty"`
	Ports       *[]visual.Port                 `json:"ports,omitempty"`
	Inputs      *[]visual.InputParamSpec       `json:"inputs,omitempty"`
	Outputs     *[]visual.OutputParamSpec      `json:"outputs,omitempty"`
}

type EditableNode struct {
	ID string `json:"id,omitempty"`
	NodePatch
}

type UpdateNodeOperation struct {
	Op     OperationKind `json:"op" jsonschema:"enum=update_node"`
	NodeID string        `json:"nodeId"`
	Set    NodePatch     `json:"set"`
}

func (*UpdateNodeOperation) Kind() OperationKind { return OperationUpdateNode }

type DeleteNodeOperation struct {
	Op           OperationKind `json:"op" jsonschema:"enum=delete_node"`
	NodeID       string        `json:"nodeId"`
	CascadeEdges bool          `json:"cascadeEdges"`
}

func (*DeleteNodeOperation) Kind() OperationKind { return OperationDeleteNode }

type AddEdgeOperation struct {
	Op        OperationKind `json:"op" jsonschema:"enum=add_edge"`
	ClientRef string        `json:"clientRef,omitempty"`
	Edge      EditableEdge  `json:"edge"`
}

func (*AddEdgeOperation) Kind() OperationKind { return OperationAddEdge }

type EdgePatch struct {
	Source     *string          `json:"source,omitempty"`
	SourcePort *string          `json:"sourcePort,omitempty"`
	Target     *string          `json:"target,omitempty"`
	TargetPort *string          `json:"targetPort,omitempty"`
	Kind       *visual.EdgeKind `json:"kind,omitempty"`
	Condition  *string          `json:"condition,omitempty"`
}

type EditableEdge struct {
	ID string `json:"id,omitempty"`
	EdgePatch
}

type UpdateEdgeOperation struct {
	Op     OperationKind `json:"op" jsonschema:"enum=update_edge"`
	EdgeID string        `json:"edgeId"`
	Set    EdgePatch     `json:"set"`
}

func (*UpdateEdgeOperation) Kind() OperationKind { return OperationUpdateEdge }

type DeleteEdgeOperation struct {
	Op     OperationKind `json:"op" jsonschema:"enum=delete_edge"`
	EdgeID string        `json:"edgeId"`
}

func (*DeleteEdgeOperation) Kind() OperationKind { return OperationDeleteEdge }

type WorkflowPatch struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	EnvPackages   *[]string       `json:"envPackages,omitempty"`
	ValidationEnv *string         `json:"validationEnv,omitempty"`
	Vars          *map[string]any `json:"vars,omitempty"`
	Plan          *workflow.Plan  `json:"plan,omitempty"`
}

type UpdateWorkflowOperation struct {
	Op  OperationKind `json:"op" jsonschema:"enum=update_workflow"`
	Set WorkflowPatch `json:"set"`
}

func (*UpdateWorkflowOperation) Kind() OperationKind { return OperationUpdateWorkflow }

type LayoutPatch struct {
	Nodes    []LayoutNodePatch `json:"nodes"`
	Viewport *visual.Viewport  `json:"viewport,omitempty"`
}

type LayoutNodePatch struct {
	NodeID   string          `json:"nodeId"`
	Position visual.Position `json:"position"`
}

type UpdateLayoutOperation struct {
	Op     OperationKind `json:"op" jsonschema:"enum=update_layout"`
	Layout LayoutPatch   `json:"layout"`
}

func (*UpdateLayoutOperation) Kind() OperationKind { return OperationUpdateLayout }

type AttachManualBindingOperation struct {
	Op             OperationKind   `json:"op" jsonschema:"enum=attach_manual_binding"`
	ManualRef      ManualReference `json:"manualRef"`
	WorkflowDigest string          `json:"workflowDigest"`
}

type ManualReference struct {
	ManualID    string `json:"manualId"`
	CandidateID string `json:"candidateId,omitempty"`
}

func (*AttachManualBindingOperation) Kind() OperationKind { return OperationAttachManualBinding }

func (o WorkflowOperation) MarshalJSON() ([]byte, error) {
	if o.Value == nil {
		return nil, fmt.Errorf("workflow operation is empty")
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(o.Value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	object["op"] = o.Value.Kind()
	return json.Marshal(object)
}

func (o *WorkflowOperation) UnmarshalJSON(data []byte) error {
	var header struct {
		Op OperationKind `json:"op"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	var value OperationValue
	switch header.Op {
	case OperationAddNode:
		value = &AddNodeOperation{}
	case OperationUpdateNode:
		value = &UpdateNodeOperation{}
	case OperationDeleteNode:
		value = &DeleteNodeOperation{}
	case OperationAddEdge:
		value = &AddEdgeOperation{}
	case OperationUpdateEdge:
		value = &UpdateEdgeOperation{}
	case OperationDeleteEdge:
		value = &DeleteEdgeOperation{}
	case OperationUpdateWorkflow:
		value = &UpdateWorkflowOperation{}
	case OperationUpdateLayout:
		value = &UpdateLayoutOperation{}
	case OperationAttachManualBinding:
		value = &AttachManualBindingOperation{}
	default:
		return fmt.Errorf("unknown workflow operation %q", header.Op)
	}
	if err := DecodeStrict(data, value); err != nil {
		return err
	}
	o.Value = value
	return o.Validate()
}

func (o WorkflowOperation) Validate() error {
	if o.Value == nil {
		return fmt.Errorf("workflow operation is empty")
	}
	required := func(name, value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	switch operation := o.Value.(type) {
	case *AddNodeOperation:
		if operation.ClientRef == "" {
			return required("node.id", operation.Node.ID)
		}
		if operation.Node.Type == nil || operation.Node.Position == nil {
			return fmt.Errorf("node.type and node.position are required")
		}
	case *UpdateNodeOperation:
		if err := required("nodeId", operation.NodeID); err != nil || operation.Set == (NodePatch{}) {
			return fmt.Errorf("nodeId and non-empty set are required")
		}
	case *DeleteNodeOperation:
		return required("nodeId", operation.NodeID)
	case *AddEdgeOperation:
		if operation.Edge.Source == nil || operation.Edge.Target == nil {
			return fmt.Errorf("edge.source and edge.target are required")
		}
		if err := required("edge.source", *operation.Edge.Source); err != nil {
			return err
		}
		return required("edge.target", *operation.Edge.Target)
	case *UpdateEdgeOperation:
		if err := required("edgeId", operation.EdgeID); err != nil || operation.Set == (EdgePatch{}) {
			return fmt.Errorf("edgeId and non-empty set are required")
		}
	case *DeleteEdgeOperation:
		return required("edgeId", operation.EdgeID)
	case *UpdateWorkflowOperation:
		if operation.Set == (WorkflowPatch{}) {
			return fmt.Errorf("non-empty workflow set is required")
		}
	case *UpdateLayoutOperation:
		if operation.Layout.Viewport == nil && len(operation.Layout.Nodes) == 0 {
			return fmt.Errorf("non-empty layout is required")
		}
	case *AttachManualBindingOperation:
		if err := required("manualRef.manualId", operation.ManualRef.ManualID); err != nil {
			return err
		}
		return required("workflowDigest", operation.WorkflowDigest)
	default:
		return fmt.Errorf("unsupported workflow operation %T", operation)
	}
	return nil
}

type ActorKind string
type CandidateState string

type WorkflowDiagnostic struct {
	Code       string          `json:"code"`
	Severity   string          `json:"severity"`
	Message    string          `json:"message"`
	NodeID     string          `json:"nodeId,omitempty"`
	EdgeID     string          `json:"edgeId,omitempty"`
	Field      string          `json:"field,omitempty"`
	Suggestion string          `json:"suggestion,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
}

type ValidationResult struct {
	Kind        string               `json:"kind" jsonschema:"enum=validation"`
	Basis       ValidationBasis      `json:"basis"`
	Valid       bool                 `json:"valid"`
	Diagnostics []WorkflowDiagnostic `json:"diagnostics"`
}

type WorkflowChangeSet struct {
	SchemaVersion     string              `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	ID                string              `json:"id"`
	Key               WorkflowKey         `json:"workflowKey"`
	CandidateID       string              `json:"candidateId"`
	CandidateRevision uint64              `json:"candidateRevision"`
	ParentChangeSetID string              `json:"parentChangeSetId,omitempty"`
	Actor             ActorKind           `json:"actor"`
	Intent            string              `json:"intent"`
	Base              Revision            `json:"base"`
	Operations        []WorkflowOperation `json:"operations"`
	PreviewHash       string              `json:"previewHash"`
	Validation        ValidationResult    `json:"validation"`
	State             CandidateState      `json:"state"`
	CreatedAt         time.Time           `json:"createdAt"`
	ExpiresAt         time.Time           `json:"expiresAt"`
}

type WorkflowCandidate struct {
	ID                 string      `json:"id"`
	Key                WorkflowKey `json:"workflowKey"`
	CurrentChangeSetID string      `json:"currentChangeSetId"`
	CurrentRevision    uint64      `json:"currentRevision"`
	CurrentPreviewHash string      `json:"currentPreviewHash"`
	State              string      `json:"state"`
	ExpiresAt          time.Time   `json:"expiresAt"`
}

type WorkflowEditGrant struct {
	ID                  string          `json:"id"`
	Key                 WorkflowKey     `json:"workflowKey"`
	ActorID             string          `json:"actorId"`
	PlanHash            string          `json:"planHash"`
	BaseSemanticHash    string          `json:"baseSemanticHash"`
	AllowedOps          []OperationKind `json:"allowedOps"`
	MaxOperations       int             `json:"maxOperations"`
	MaxAddedNodes       int             `json:"maxAddedNodes"`
	AllowDeleteExisting bool            `json:"allowDeleteExisting"`
	ExpiresAt           time.Time       `json:"expiresAt"`
}

type WorkflowApplyApproval struct {
	ID               string      `json:"id"`
	ActorID          string      `json:"actorId"`
	Key              WorkflowKey `json:"workflowKey"`
	CandidateID      string      `json:"candidateId"`
	ChangeSetID      string      `json:"changeSetId"`
	PreviewHash      string      `json:"previewHash"`
	ExpectedRevision string      `json:"expectedRevision"`
	ExpiresAt        time.Time   `json:"expiresAt"`
	ConsumedAt       *time.Time  `json:"consumedAt,omitempty"`
}

type WorkflowGraphDelta struct {
	AddedNodeIDs   []string `json:"addedNodeIds"`
	UpdatedNodeIDs []string `json:"updatedNodeIds"`
	DeletedNodeIDs []string `json:"deletedNodeIds"`
	AddedEdgeIDs   []string `json:"addedEdgeIds"`
	UpdatedEdgeIDs []string `json:"updatedEdgeIds"`
	DeletedEdgeIDs []string `json:"deletedEdgeIds"`
}

type WorkflowMutationReceipt struct {
	SchemaVersion        string             `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	Kind                 string             `json:"kind" jsonschema:"enum=receipt"`
	TransactionID        string             `json:"transactionId"`
	ChangeSetID          string             `json:"changeSetId"`
	Key                  WorkflowKey        `json:"workflowKey"`
	Status               string             `json:"status"`
	RevisionBefore       Revision           `json:"revisionBefore"`
	RevisionAfter        *Revision          `json:"revisionAfter,omitempty"`
	Delta                WorkflowGraphDelta `json:"delta"`
	Validation           ValidationResult   `json:"validation"`
	CheckpointID         string             `json:"checkpointId,omitempty"`
	InverseTransactionID string             `json:"inverseTransactionId,omitempty"`
	AuditID              string             `json:"auditId"`
}

type WorkflowConflict struct {
	Code             ErrorCode `json:"code"`
	Base             Revision  `json:"base"`
	Current          Revision  `json:"current"`
	ConflictingPaths []string  `json:"conflictingPaths"`
	SafeRebasePaths  []string  `json:"safeRebasePaths"`
	ChangedBy        ActorKind `json:"changedBy"`
}

type LifecycleGate struct {
	Kind     string          `json:"kind"`
	Basis    ValidationBasis `json:"basis"`
	Status   string          `json:"status"`
	IssuedAt time.Time       `json:"issuedAt"`
}

type WorkflowDomainEvent struct {
	SchemaVersion     string          `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	EventID           string          `json:"eventId"`
	Sequence          uint64          `json:"sequence"`
	Type              string          `json:"type"`
	Key               WorkflowKey     `json:"workflowKey"`
	Revision          Revision        `json:"revision"`
	Actor             ActorKind       `json:"actor"`
	TransactionID     string          `json:"transactionId,omitempty"`
	ChangeSetID       string          `json:"changeSetId,omitempty"`
	CandidateRevision uint64          `json:"candidateRevision,omitempty"`
	Payload           json.RawMessage `json:"payload"`
	OccurredAt        time.Time       `json:"occurredAt"`
}

type ToolTextContent struct {
	Type string `json:"type" jsonschema:"enum=text"`
	Text string `json:"text"`
}

type ToolResult[T any] struct {
	SchemaVersion     string            `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	Content           []ToolTextContent `json:"content"`
	StructuredContent T                 `json:"structuredContent"`
	IsError           bool              `json:"isError"`
	Meta              map[string]any    `json:"_meta,omitempty"`
}
