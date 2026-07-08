package workfloweditor

import (
	"time"

	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/workflowgen"
	"runner/workflow/visual"
)

type SessionIntent string

const (
	SessionIntentCreate SessionIntent = "create"
	SessionIntentEdit   SessionIntent = "edit"
)

type EffectStatus string

const (
	EffectChanged      EffectStatus = "changed"
	EffectNoEffect     EffectStatus = "no_effect"
	EffectDuplicate    EffectStatus = "duplicate"
	EffectMetadataOnly EffectStatus = "metadata_only"
)

type WorkflowAISession struct {
	SchemaVersion  string              `json:"schemaVersion"`
	ID             string              `json:"id"`
	WorkflowID     string              `json:"workflowId,omitempty"`
	BaseRevision   string              `json:"baseRevision,omitempty"`
	ActiveRevision string              `json:"activeRevision,omitempty"`
	Intent         SessionIntent       `json:"sessionIntent"`
	Status         string              `json:"status"`
	CurrentPlan    *WorkflowEditPlan   `json:"currentPlan,omitempty"`
	PatchQueue     []WorkflowPatch     `json:"patchQueue,omitempty"`
	UndoStack      []UndoCheckpointRef `json:"undoStack,omitempty"`
	StepBudget     StepBudget          `json:"stepBudget"`
	ToolLogRef     string              `json:"toolLogRef,omitempty"`
	CreatedAt      time.Time           `json:"createdAt,omitempty"`
	UpdatedAt      time.Time           `json:"updatedAt,omitempty"`
}

type StepBudget struct {
	MaxPatchReviewsPerTurn int `json:"maxPatchReviewsPerTurn"`
	UsedPatchReviews       int `json:"usedPatchReviews"`
	RemainingPlanItems     int `json:"remainingPlanItems"`
}

type WorkflowEditPlan struct {
	ID         string                 `json:"id"`
	WorkflowID string                 `json:"workflowId,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Items      []WorkflowEditPlanItem `json:"items"`
	CreatedAt  time.Time              `json:"createdAt,omitempty"`
}

type WorkflowVariableSpec struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
	Source   string `json:"source,omitempty"`
}

type WorkflowEditPlanItem struct {
	ID                string                 `json:"id"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description,omitempty"`
	Status            string                 `json:"status,omitempty"`
	Goal              string                 `json:"goal,omitempty"`
	Environment       string                 `json:"environment,omitempty"`
	NodeLabel         string                 `json:"nodeLabel,omitempty"`
	NodeType          string                 `json:"nodeType,omitempty"`
	NodeAction        string                 `json:"nodeAction,omitempty"`
	ScriptSummary     string                 `json:"scriptSummary,omitempty"`
	ValidationSummary string                 `json:"validationSummary,omitempty"`
	InputVariables    []WorkflowVariableSpec `json:"inputVariables,omitempty"`
	OutputVariables   []WorkflowVariableSpec `json:"outputVariables,omitempty"`
	Script            string                 `json:"script,omitempty"`
}

type PatchOperationType string

const (
	PatchAddNode                PatchOperationType = "add_node"
	PatchUpdateNode             PatchOperationType = "update_node"
	PatchDeleteNode             PatchOperationType = "delete_node"
	PatchAddEdge                PatchOperationType = "add_edge"
	PatchDeleteEdge             PatchOperationType = "delete_edge"
	PatchUpdateWorkflowMetadata PatchOperationType = "update_workflow_metadata"
	PatchUpdateInputs           PatchOperationType = "update_inputs"
	PatchUpdateOutputs          PatchOperationType = "update_outputs"
	PatchUpdateInventory        PatchOperationType = "update_inventory"
	PatchBindOpsManualCandidate PatchOperationType = "bind_ops_manual_candidate"
	PatchReplaceFullGraph       PatchOperationType = "replace_full_graph"
)

type WorkflowPatch struct {
	ID           string                   `json:"id"`
	WorkflowID   string                   `json:"workflowId,omitempty"`
	BaseRevision string                   `json:"baseRevision,omitempty"`
	Summary      string                   `json:"summary,omitempty"`
	Reason       string                   `json:"reason,omitempty"`
	Operations   []WorkflowPatchOperation `json:"operations"`
	CreatedAt    time.Time                `json:"createdAt,omitempty"`
}

type WorkflowPatchOperation struct {
	Op       PatchOperationType `json:"op"`
	NodeID   string             `json:"nodeId,omitempty"`
	EdgeID   string             `json:"edgeId,omitempty"`
	Node     *visual.Node       `json:"node,omitempty"`
	Edge     *visual.Edge       `json:"edge,omitempty"`
	Fields   map[string]any     `json:"fields,omitempty"`
	Metadata map[string]any     `json:"metadata,omitempty"`
	Graph    *visual.Graph      `json:"graph,omitempty"`
}

type WorkflowPatchValidation struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type PatchEffectResult struct {
	Status            EffectStatus `json:"status"`
	Summary           string       `json:"summary,omitempty"`
	AffectedNodes     []string     `json:"affectedNodes,omitempty"`
	AffectedEdges     []string     `json:"affectedEdges,omitempty"`
	AffectedVariables []string     `json:"affectedVariables,omitempty"`
}

type WorkflowPatchPreview struct {
	PatchID string            `json:"patchId,omitempty"`
	Graph   visual.Graph      `json:"graph"`
	Effect  PatchEffectResult `json:"effect"`
}

type WorkflowPatchResult struct {
	PatchID        string               `json:"patchId"`
	WorkflowID     string               `json:"workflowId"`
	RevisionBefore string               `json:"revisionBefore"`
	RevisionAfter  string               `json:"revisionAfter"`
	Effect         PatchEffectResult    `json:"effect"`
	Describe       DescribeResult       `json:"describe"`
	UndoCheckpoint UndoCheckpointRef    `json:"undoCheckpoint"`
	Audit          []WorkflowAuditEvent `json:"audit,omitempty"`
}

type UndoCheckpointRef struct {
	ID             string    `json:"id"`
	WorkflowID     string    `json:"workflowId"`
	PatchID        string    `json:"patchId,omitempty"`
	RevisionBefore string    `json:"revisionBefore"`
	RevisionAfter  string    `json:"revisionAfter"`
	CreatedAt      time.Time `json:"createdAt,omitempty"`
}

type UndoPatchResult struct {
	WorkflowID     string            `json:"workflowId"`
	RevisionBefore string            `json:"revisionBefore"`
	RevisionAfter  string            `json:"revisionAfter"`
	UndoCheckpoint UndoCheckpointRef `json:"undoCheckpoint"`
	Describe       DescribeResult    `json:"describe"`
}

type WorkflowAuditEvent struct {
	Type               string    `json:"type"`
	PatchID            string    `json:"patchId,omitempty"`
	UserConfirmationID string    `json:"userConfirmationId,omitempty"`
	DrawerSessionID    string    `json:"drawerSessionId,omitempty"`
	Reason             string    `json:"reason,omitempty"`
	CreatedAt          time.Time `json:"createdAt,omitempty"`
}

type WorkflowSnapshot struct {
	WorkflowID     string                  `json:"workflowId"`
	Revision       string                  `json:"revision"`
	RevisionDigest string                  `json:"revisionDigest"`
	Graph          visual.Graph            `json:"graph"`
	Validation     WorkflowPatchValidation `json:"validation"`
	ManualBinding  *ManualBindingSummary   `json:"manualBinding,omitempty"`
	Describe       DescribeResult          `json:"describe"`
}

type WorkflowStepSnapshot struct {
	WorkflowID string       `json:"workflowId"`
	Revision   string       `json:"revision"`
	Node       *visual.Node `json:"node,omitempty"`
}

type ManualBindingSummary struct {
	ManualID       string `json:"manualId,omitempty"`
	CandidateID    string `json:"candidateId,omitempty"`
	ReviewStatus   string `json:"reviewStatus,omitempty"`
	WorkflowDigest string `json:"workflowDigest,omitempty"`
}

type DescribeResult struct {
	WorkflowID string   `json:"workflowId,omitempty"`
	Revision   string   `json:"revision,omitempty"`
	Summary    string   `json:"summary"`
	NodeCount  int      `json:"nodeCount"`
	EdgeCount  int      `json:"edgeCount"`
	NodeIDs    []string `json:"nodeIds,omitempty"`
}

type GetSnapshotRequest struct {
	WorkflowID string `json:"workflowId"`
}

type GetStepRequest struct {
	WorkflowID string `json:"workflowId"`
	NodeID     string `json:"nodeId"`
}

type DescribeRequest struct {
	WorkflowID string        `json:"workflowId"`
	Graph      *visual.Graph `json:"graph,omitempty"`
}

type ProposeEditPlanRequest struct {
	WorkflowID      string `json:"workflowId,omitempty"`
	DrawerSessionID string `json:"drawerSessionId,omitempty"`
	Message         string `json:"message"`
}

type ProposePatchRequest struct {
	WorkflowID      string `json:"workflowId,omitempty"`
	BaseRevision    string `json:"baseRevision,omitempty"`
	DrawerSessionID string `json:"drawerSessionId,omitempty"`
	PlanID          string `json:"planId,omitempty"`
	ItemID          string `json:"itemId,omitempty"`
	Message         string `json:"message,omitempty"`
}

type ValidatePatchRequest struct {
	WorkflowID                string        `json:"workflowId,omitempty"`
	BaseRevision              string        `json:"baseRevision,omitempty"`
	Patch                     WorkflowPatch `json:"patch"`
	AllowFullGraphReplacement bool          `json:"allowFullGraphReplacement,omitempty"`
	SecondConfirmationID      string        `json:"secondConfirmationId,omitempty"`
}

type PreviewPatchRequest struct {
	WorkflowID   string        `json:"workflowId,omitempty"`
	BaseRevision string        `json:"baseRevision,omitempty"`
	Patch        WorkflowPatch `json:"patch"`
}

type DetectPatchEffectRequest struct {
	WorkflowID string        `json:"workflowId,omitempty"`
	Patch      WorkflowPatch `json:"patch"`
}

type ApplyPatchRequest struct {
	WorkflowID                string        `json:"workflowId"`
	BaseRevision              string        `json:"baseRevision"`
	PatchID                   string        `json:"patchId"`
	Patch                     WorkflowPatch `json:"patch"`
	UserConfirmationID        string        `json:"userConfirmationId"`
	DrawerSessionID           string        `json:"drawerSessionId"`
	Reason                    string        `json:"reason"`
	AllowFullGraphReplacement bool          `json:"allowFullGraphReplacement,omitempty"`
	SecondConfirmationID      string        `json:"secondConfirmationId,omitempty"`
}

type UndoLastAIPatchRequest struct {
	WorkflowID      string `json:"workflowId"`
	DrawerSessionID string `json:"drawerSessionId"`
	Reason          string `json:"reason,omitempty"`
}

type CreateSessionRequest struct {
	WorkflowID      string        `json:"workflowId,omitempty"`
	BaseRevision    string        `json:"baseRevision,omitempty"`
	Intent          SessionIntent `json:"sessionIntent"`
	DrawerSessionID string        `json:"drawerSessionId,omitempty"`
}

type WorkflowManualCandidateRequest struct {
	WorkflowID             string `json:"workflowId"`
	ManualID               string `json:"manualId,omitempty"`
	PreviousManualVersion  string `json:"previousManualVersion,omitempty"`
	ExpectedWorkflowDigest string `json:"expectedWorkflowDigest,omitempty"`
}

type WorkflowManualCandidateResult struct {
	Candidate    opsmanual.ManualCandidate `json:"candidate"`
	WorkflowRef  opsmanual.WorkflowRef     `json:"workflowRef"`
	StaleBinding bool                      `json:"staleBinding,omitempty"`
	StaleReason  string                    `json:"staleReason,omitempty"`
}

type WorkflowDraftFromPlanRequest struct {
	SessionID          string                             `json:"sessionId,omitempty"`
	DrawerSessionID    string                             `json:"drawerSessionId,omitempty"`
	UserConfirmationID string                             `json:"userConfirmationId,omitempty"`
	Plan               workflowgen.WorkflowGenerationPlan `json:"plan"`
}

type WorkflowDraftFromPlanResult struct {
	WorkflowID string                  `json:"workflowId"`
	Graph      visual.Graph            `json:"graph"`
	Revision   string                  `json:"revision"`
	Validation WorkflowPatchValidation `json:"validation"`
	Describe   DescribeResult          `json:"describe"`
	Published  bool                    `json:"published"`
	Executed   bool                    `json:"executed"`
}
