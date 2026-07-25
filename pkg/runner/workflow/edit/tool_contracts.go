package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func DecodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("unexpected trailing JSON data")
	}
	return nil
}

func NewToolResult[T any](summary string, structured T) ToolResult[T] {
	return ToolResult[T]{
		SchemaVersion:     SchemaVersion,
		Content:           []ToolTextContent{{Type: "text", Text: summary}},
		StructuredContent: structured,
	}
}

func NewToolError(summary string, structured DomainError) ToolResult[DomainError] {
	structured.Kind = "error"
	result := NewToolResult(summary, structured)
	result.IsError = true
	return result
}

type GetContextRequest struct {
	SchemaVersion         string      `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	WorkflowKey           WorkflowKey `json:"workflowKey"`
	TurnContextArtifactID string      `json:"turnContextArtifactId,omitempty"`
	Revision              string      `json:"revision,omitempty"`
	View                  string      `json:"view"`
	NodeIDs               []string    `json:"nodeIds,omitempty"`
	EdgeDepth             int         `json:"edgeDepth,omitempty"`
	Include               []string    `json:"include,omitempty"`
	Cursor                string      `json:"cursor,omitempty"`
	Limit                 int         `json:"limit,omitempty"`
}

type WorkflowContext struct {
	Kind                  string           `json:"kind" jsonschema:"enum=context"`
	SchemaVersion         string           `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	WorkflowKey           WorkflowKey      `json:"workflowKey"`
	Revision              Revision         `json:"revision"`
	Graph                 json.RawMessage  `json:"graph,omitempty"`
	Validation            ValidationResult `json:"validation"`
	ResourceURI           string           `json:"resourceUri"`
	SelectionContextStale bool             `json:"selectionContextStale"`
	NextCursor            string           `json:"nextCursor,omitempty"`
}

type ActionCatalogRequest struct {
	SchemaVersion   string      `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	WorkflowKey     WorkflowKey `json:"workflowKey"`
	Names           []string    `json:"names,omitempty"`
	Query           string      `json:"query,omitempty"`
	Category        string      `json:"category,omitempty"`
	NodeType        string      `json:"nodeType,omitempty"`
	RiskMax         string      `json:"riskMax,omitempty"`
	IncludeSchema   bool        `json:"includeSchema,omitempty"`
	IncludeExamples bool        `json:"includeExamples,omitempty"`
	Cursor          string      `json:"cursor,omitempty"`
	Limit           int         `json:"limit,omitempty"`
}

type ActionCatalogEntry struct {
	Name   string          `json:"name"`
	Digest string          `json:"digest"`
	Spec   json.RawMessage `json:"spec"`
}

type ActionCatalogPage struct {
	Kind           string               `json:"kind" jsonschema:"enum=action_catalog"`
	SchemaVersion  string               `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	CatalogVersion string               `json:"catalogVersion"`
	Entries        []ActionCatalogEntry `json:"entries"`
	NextCursor     string               `json:"nextCursor,omitempty"`
}

type PreviewPatchRequest struct {
	SchemaVersion   string              `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	WorkflowKey     WorkflowKey         `json:"workflowKey"`
	Expected        Revision            `json:"expected"`
	ValidationBasis ValidationBasis     `json:"validationBasis"`
	Intent          string              `json:"intent"`
	Operations      []WorkflowOperation `json:"operations"`
}

type ChangeSetPreview struct {
	Kind      string            `json:"kind" jsonschema:"enum=preview"`
	ChangeSet WorkflowChangeSet `json:"changeSet"`
	Candidate WorkflowCandidate `json:"candidate"`
	Diff      json.RawMessage   `json:"diff"`
}

type ApplyPatchRequest struct {
	SchemaVersion    string      `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	WorkflowKey      WorkflowKey `json:"workflowKey"`
	ChangeSetID      string      `json:"changeSetId"`
	PreviewHash      string      `json:"previewHash"`
	ExpectedRevision string      `json:"expectedRevision"`
	IdempotencyKey   string      `json:"idempotencyKey"`
	CandidateGrantID string      `json:"candidateGrantId"`
	ApplyApprovalID  string      `json:"applyApprovalId"`
	Reason           string      `json:"reason"`
}

type ValidateRequest struct {
	SchemaVersion string      `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	WorkflowKey   WorkflowKey `json:"workflowKey"`
	Revision      string      `json:"revision,omitempty"`
	ChangeSetID   string      `json:"changeSetId,omitempty"`
}
