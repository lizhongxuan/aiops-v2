package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
	"runner/workflowstore"
)

// Workflow workspace projection DTOs are intentionally narrower than the
// transaction snapshot. They contain only facts needed by the browser and
// never expose Graph payload copies, approval records, audit payloads, or raw
// diagnostic/provider text.
type WorkflowRunGateProjection struct {
	CurrentBasis edit.ValidationBasis          `json:"currentBasis"`
	Environment  WorkflowEnvironmentProjection `json:"environment"`
	PostCheck    *WorkflowPostCheckProjection  `json:"postCheck,omitempty"`
}

const WorkflowEnvironmentCapabilitySchemaVersion = "runner.workflow.environment-capability.v1"

type WorkflowEnvironmentProjection struct {
	Name           string                                  `json:"name"`
	BindingVersion string                                  `json:"bindingVersion"`
	BindingStatus  string                                  `json:"bindingStatus"`
	Configuration  WorkflowEnvironmentCapabilityProjection `json:"configuration"`
}

// WorkflowEnvironmentCapabilityProjection describes only a trusted
// configuration mutation surface. A pre-existing binding is a separate fact
// and never makes this capability executable.
type WorkflowEnvironmentCapabilityProjection struct {
	SchemaVersion      string `json:"schemaVersion"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Permission         string `json:"permission"`
	HandlerAvailable   bool   `json:"handlerAvailable"`
	ConfigureAllowed   bool   `json:"configureAllowed"`
	PrimaryActionCount int    `json:"primaryActionCount"`
}

func disabledWorkflowEnvironmentCapability() WorkflowEnvironmentCapabilityProjection {
	return WorkflowEnvironmentCapabilityProjection{
		SchemaVersion: WorkflowEnvironmentCapabilitySchemaVersion,
		Status:        "off", Reason: "provider_unavailable", Permission: "unavailable",
	}
}

type WorkflowPostCheckProjection struct {
	Basis         edit.ValidationBasis              `json:"basis"`
	Mode          string                            `json:"mode"`
	Status        string                            `json:"status"`
	CheckedAt     time.Time                         `json:"checkedAt"`
	Diagnostics   []edit.WorkflowDiagnostic         `json:"diagnostics,omitempty"`
	Configuration WorkflowConfigurationRequirements `json:"configuration,omitempty"`
}

type WorkflowReceiptProjection struct {
	TransactionID  string                `json:"transactionId"`
	ChangeSetID    string                `json:"changeSetId"`
	WorkflowKey    edit.WorkflowKey      `json:"workflowKey"`
	Status         string                `json:"status"`
	RevisionBefore edit.Revision         `json:"revisionBefore"`
	RevisionAfter  *edit.Revision        `json:"revisionAfter,omitempty"`
	Validation     edit.ValidationResult `json:"validation"`
	ReceivedAt     time.Time             `json:"receivedAt"`
}

type WorkflowValidationArtifactProjection struct {
	ArtifactID  string                    `json:"artifactId"`
	Basis       edit.ValidationBasis      `json:"basis"`
	Status      string                    `json:"status"`
	Diagnostics []edit.WorkflowDiagnostic `json:"diagnostics"`
	IssuedAt    time.Time                 `json:"issuedAt"`
}

type WorkflowLifecycleArtifactProjection struct {
	ArtifactID string               `json:"artifactId"`
	Basis      edit.ValidationBasis `json:"basis"`
	Status     string               `json:"status"`
	IssuedAt   time.Time            `json:"issuedAt"`
}

type WorkflowDurableHistoryProjection struct {
	Source             string    `json:"source"`
	TransactionID      string    `json:"transactionId"`
	BusinessLabel      string    `json:"businessLabel"`
	Status             string    `json:"status"`
	OccurredAt         time.Time `json:"occurredAt"`
	ActorLabel         string    `json:"actorLabel,omitempty"`
	TouchedObjectCount int       `json:"touchedObjectCount"`
	Reversible         bool      `json:"reversible"`
	Redoable           bool      `json:"redoable"`
}

// workspaceProjectionSnapshot enriches the committed Graph with facts from
// the same durable transaction selected by CommitStore. Missing projection
// facts fail closed in the UI; they never fall back to page-local inference.
func (a *WorkflowEditApplication) workspaceProjectionSnapshot(ctx context.Context, key edit.WorkflowKey) (WorkflowEditSnapshot, error) {
	snapshot, err := a.readSnapshot(ctx, key)
	if err != nil {
		return WorkflowEditSnapshot{}, err
	}
	if a.commits == nil {
		return snapshot, nil
	}
	committed, err := a.commits.Snapshot(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return snapshot, nil
	}
	if err != nil {
		return WorkflowEditSnapshot{}, err
	}
	if !workflowProjectionBasisMatchesSnapshot(snapshot, committed.ValidationBasis) {
		return WorkflowEditSnapshot{}, ErrWorkflowEditSnapshot
	}

	receipt := projectWorkflowReceipt(committed.Receipt, committed.CommittedAt)
	validation := projectWorkflowValidationArtifact(receipt, committed.CommittedAt)
	environmentName := strings.TrimSpace(snapshot.Graph.Workflow.ValidationEnv)
	bindingStatus := "unbound"
	if environmentName != "" {
		bindingStatus = "bound"
	}
	snapshot.RunGateFacts = &WorkflowRunGateProjection{
		CurrentBasis: committed.ValidationBasis,
		Environment: WorkflowEnvironmentProjection{
			Name:           environmentName,
			BindingVersion: committed.ValidationBasis.EnvironmentBindingVersion,
			BindingStatus:  bindingStatus,
			Configuration:  disabledWorkflowEnvironmentCapability(),
		},
	}
	snapshot.Receipts = []WorkflowReceiptProjection{receipt}
	snapshot.ValidationArtifacts = []WorkflowValidationArtifactProjection{validation}
	if postCheck := latestWorkflowPostCheckProjection(committed.LifecycleArtifacts); postCheck != nil {
		snapshot.RunGateFacts.PostCheck = postCheck
	}

	// Lifecycle publication is deliberately not inferred from a Visual status
	// string. It is projected only from an explicit durable lifecycle envelope.
	for index := len(committed.LifecycleArtifacts) - 1; index >= 0; index-- {
		envelope := committed.LifecycleArtifacts[index]
		if envelope.Kind != "workflow.lifecycle" || envelope.SchemaVersion != edit.SchemaVersion {
			continue
		}
		var gate edit.LifecycleGate
		if json.Unmarshal(envelope.Payload, &gate) != nil || gate.Basis.Validate() != nil || gate.IssuedAt.IsZero() || (gate.Status != "draft" && gate.Status != "published") {
			continue
		}
		snapshot.LifecycleArtifacts = append(snapshot.LifecycleArtifacts, WorkflowLifecycleArtifactProjection{
			ArtifactID: "lifecycle:" + committed.TransactionID + ":" + gate.Status,
			Basis:      gate.Basis, Status: gate.Status, IssuedAt: gate.IssuedAt,
		})
		break
	}

	if a.candidates != nil {
		history, historyErr := a.workflowEditHistoryRecords(ctx, key)
		if historyErr == nil {
			snapshot.DurableHistory = projectWorkflowDurableHistory(history, snapshot.Graph, committed)
		}
	}
	return snapshot, nil
}

func workflowProjectionBasisMatchesSnapshot(snapshot WorkflowEditSnapshot, basis edit.ValidationBasis) bool {
	if basis.Validate() != nil || basis.ResourceVersion != snapshot.Revision.ResourceVersion {
		return false
	}
	if basis.SemanticHash == snapshot.Revision.SemanticHash {
		return true
	}
	// A semantic-hash field classification migration must not make the Graph
	// uneditable. Accept only the exact previous algorithm as a stale validation
	// basis; selectors still compare CurrentBasis with the new current revision,
	// so publish/run remain fail-closed until the next validated commit.
	legacyHash, err := legacyVisualWorkflowSemanticHash(snapshot.Graph)
	return err == nil && basis.SemanticHash == legacyHash
}

func projectWorkflowReceipt(receipt edit.WorkflowMutationReceipt, receivedAt time.Time) WorkflowReceiptProjection {
	validation := receipt.Validation
	validation.Diagnostics = projectWorkflowDiagnostics(validation.Diagnostics)
	status := receipt.Status
	switch status {
	case "applied":
		status = "completed"
	case "verifying", "completed", "completed_with_warnings", "configuration_required", "saved_with_issues", "no_effect", "conflict", "rejected":
	default:
		status = "verifying"
	}
	return WorkflowReceiptProjection{
		TransactionID: receipt.TransactionID, ChangeSetID: receipt.ChangeSetID, WorkflowKey: receipt.Key,
		Status: status, RevisionBefore: receipt.RevisionBefore, RevisionAfter: receipt.RevisionAfter,
		Validation: validation, ReceivedAt: receivedAt,
	}
}

func projectWorkflowValidationArtifact(receipt WorkflowReceiptProjection, issuedAt time.Time) WorkflowValidationArtifactProjection {
	status := "valid"
	switch receipt.Status {
	case "configuration_required", "saved_with_issues":
		status = receipt.Status
	default:
		if !receipt.Validation.Valid {
			status = "saved_with_issues"
		} else {
			for _, diagnostic := range receipt.Validation.Diagnostics {
				if strings.EqualFold(diagnostic.Severity, "warning") {
					status = "valid_with_warnings"
					break
				}
			}
		}
	}
	return WorkflowValidationArtifactProjection{
		ArtifactID: "validation:" + receipt.TransactionID,
		Basis:      receipt.Validation.Basis, Status: status,
		Diagnostics: append([]edit.WorkflowDiagnostic{}, receipt.Validation.Diagnostics...), IssuedAt: issuedAt,
	}
}

func latestWorkflowPostCheckProjection(envelopes []workflowstore.TypedEnvelope) *WorkflowPostCheckProjection {
	for index := len(envelopes) - 1; index >= 0; index-- {
		envelope := envelopes[index]
		if envelope.Kind != "workflow.postcheck" || envelope.SchemaVersion != edit.SchemaVersion {
			continue
		}
		var artifact workflowPostCheckArtifact
		if json.Unmarshal(envelope.Payload, &artifact) != nil || artifact.Basis.Validate() != nil || artifact.CheckedAt.IsZero() || artifact.Mode != WorkflowDryRunModeDraftNonProduction {
			continue
		}
		status, _ := workflowPostCheckStatus(artifact.Outcome, artifact.Diagnostics)
		if status == "completed" || status == "completed_with_warnings" {
			status = WorkflowDryRunStatusPassed
		}
		return &WorkflowPostCheckProjection{
			Basis: artifact.Basis, Mode: artifact.Mode, Status: status, CheckedAt: artifact.CheckedAt,
			Diagnostics: projectWorkflowDiagnostics(artifact.Diagnostics), Configuration: artifact.Configuration,
		}
	}
	return nil
}

func projectWorkflowDiagnostics(values []edit.WorkflowDiagnostic) []edit.WorkflowDiagnostic {
	result := make([]edit.WorkflowDiagnostic, 0, len(values))
	for _, value := range values {
		message := "检查项"
		if strings.EqualFold(value.Severity, "warning") {
			message = "检查提醒"
		} else if strings.EqualFold(value.Severity, "error") || strings.EqualFold(value.Severity, "fatal") || strings.EqualFold(value.Severity, "blocking") {
			message = "检查未通过"
		}
		result = append(result, edit.WorkflowDiagnostic{
			Code: value.Code, Severity: value.Severity, Message: message,
			NodeID: value.NodeID, EdgeID: value.EdgeID, Field: value.Field,
		})
	}
	return result
}

func projectWorkflowDurableHistory(records []workflowEditHistoryRecord, current visual.Graph, committed workflowstore.TransactionSnapshot) []WorkflowDurableHistoryProjection {
	// History is loaded under a separate durable-store lock from the workspace
	// snapshot. Anchor the projection to the exact committed generation so a
	// concurrent newer commit cannot be mixed into an older AppSnapshot.
	anchored := false
	for _, record := range records {
		if record.entry.Generation == committed.Generation &&
			record.entry.TransactionID == committed.TransactionID &&
			record.entry.RevisionAfter.ResourceVersion == committed.ResourceVersion {
			anchored = true
			break
		}
	}
	if !anchored {
		return []WorkflowDurableHistoryProjection{}
	}

	result := make([]WorkflowDurableHistoryProjection, 0, len(records))
	for _, record := range records {
		entry := record.entry
		if entry.Generation > committed.Generation {
			continue
		}
		status := entry.Status
		switch status {
		case "applied", "completed":
			status = "completed"
		case "completed_with_warnings", "conflict", "rejected":
		case "configuration_required", "saved_with_issues", "verifying":
			status = "completed_with_warnings"
		default:
			status = "rejected"
		}
		safeInverse := workflowHistoryRecordReversible(record, current)
		trustedUndo := record.actionValid && entry.ActionKind == WorkflowHistoryActionUndo
		trustedNonUndo := !record.actionPresent || (record.actionValid && entry.ActionKind != WorkflowHistoryActionUndo)
		result = append(result, WorkflowDurableHistoryProjection{
			Source: "durable_transaction", TransactionID: entry.TransactionID,
			BusinessLabel: workflowHistoryBusinessLabel(entry.TouchedPaths), Status: status,
			OccurredAt: entry.CommittedAt, ActorLabel: workflowHistoryActorLabel(entry.Actor),
			TouchedObjectCount: workflowHistoryTouchedObjectCount(entry.TouchedPaths),
			Reversible:         safeInverse && trustedNonUndo,
			Redoable:           safeInverse && trustedUndo,
		})
	}
	return result
}

func workflowHistoryRecordReversible(record workflowEditHistoryRecord, current visual.Graph) bool {
	if !record.entry.Reversible {
		return false
	}
	after, err := parseCommittedVisualGraph(record.snapshot.GraphPayload, record.snapshot.ResourceVersion)
	if err != nil {
		return false
	}
	return len(workflowEditHistoryConflicts(after, current, record.entry.TouchedPaths)) == 0
}

func workflowHistoryBusinessLabel(paths []string) string {
	if len(paths) > 0 {
		layoutOnly := true
		for _, path := range paths {
			if path != "/layout/viewport" && !strings.HasSuffix(path, "/position") {
				layoutOnly = false
				break
			}
		}
		if layoutOnly {
			return "调整工作流布局"
		}
	}
	return "更新工作流"
}

func workflowHistoryActorLabel(actor edit.ActorKind) string {
	switch actor {
	case "agent":
		return "AI 助手"
	case "user":
		return "用户"
	default:
		return "系统"
	}
}

func workflowHistoryTouchedObjectCount(paths []string) int {
	objects := map[string]struct{}{}
	for _, path := range paths {
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "nodes" || parts[0] == "edges") {
			objects[parts[0]+"/"+parts[1]] = struct{}{}
		} else if len(parts) > 0 && parts[0] != "" {
			objects[parts[0]] = struct{}{}
		}
	}
	return len(objects)
}
