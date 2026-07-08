package opsmanual

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type WorkflowDraftInput struct {
	WorkflowID      string         `json:"workflow_id"`
	WorkflowVersion string         `json:"workflow_version,omitempty"`
	WorkflowDigest  string         `json:"workflow_digest,omitempty"`
	StorageURI      string         `json:"storage_uri,omitempty"`
	YAML            string         `json:"yaml,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type AIChatCandidateRequest struct {
	Message    string             `json:"message,omitempty"`
	WorkflowID string             `json:"workflow_id,omitempty"`
	Workflow   WorkflowDraftInput `json:"workflow,omitempty"`
	Frame      OperationFrame     `json:"frame"`
}

type AIChatCandidateResult struct {
	RequiresWorkflowDraft bool            `json:"requires_workflow_draft,omitempty"`
	Reason                string          `json:"reason,omitempty"`
	Candidate             ManualCandidate `json:"candidate,omitempty"`
}

type AdaptationCandidateRequest struct {
	VariantID     string               `json:"variant_id,omitempty"`
	TitleSuffix   string               `json:"title_suffix,omitempty"`
	WorkflowRef   WorkflowRef          `json:"workflow_ref,omitempty"`
	Applicability ApplicabilityProfile `json:"applicability,omitempty"`
}

type ScriptImportRequest struct {
	ScriptName string         `json:"script_name"`
	Script     string         `json:"script"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type workflowDraftYAML struct {
	Version     string         `yaml:"version"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Vars        map[string]any `yaml:"vars"`
	XOpsManual  map[string]any `yaml:"x_ops_manual"`
}

func GenerateCandidateFromWorkflowDraft(input WorkflowDraftInput) (ManualCandidate, error) {
	workflowID := strings.TrimSpace(input.WorkflowID)
	if workflowID == "" {
		return ManualCandidate{}, fmt.Errorf("workflow_id is required")
	}
	raw := strings.TrimSpace(input.YAML)
	if raw == "" {
		raw = workflowDraftFallbackYAML(workflowID, input.WorkflowVersion)
	}
	analysis, err := AnalyzeWorkflowForManual(WorkflowManualGenerationRequest{
		WorkflowID:      workflowID,
		WorkflowVersion: input.WorkflowVersion,
		WorkflowDigest:  input.WorkflowDigest,
		StorageURI:      input.StorageURI,
		RawYAML:         []byte(raw),
	})
	if err != nil {
		return ManualCandidate{}, err
	}
	applyWorkflowDraftMetadataOverrides(&analysis, input.Metadata)
	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		return ManualCandidate{}, err
	}
	sourceType := metadataString(input.Metadata, "source_type")
	if sourceType == "" {
		sourceType = metadataString(input.Metadata, "source")
	}
	if sourceType == "" {
		sourceType = "workflow_draft"
	}
	candidate.SourceType = sourceType
	candidate.ProposedManual.Metadata["source_type"] = sourceType
	if values := metadataStringSlice(analysis.XOpsManual, "cannot_use_when"); len(values) > 0 {
		candidate.ProposedManual.CannotUseWhen = values
	}
	return candidate, nil
}

func GenerateCandidateFromAIChat(req AIChatCandidateRequest) (AIChatCandidateResult, error) {
	workflowID := firstNonEmpty(req.WorkflowID, req.Workflow.WorkflowID)
	if strings.TrimSpace(workflowID) == "" {
		return AIChatCandidateResult{RequiresWorkflowDraft: true, Reason: "requires_workflow_draft"}, nil
	}
	workflow := req.Workflow
	workflow.WorkflowID = workflowID
	candidate, err := GenerateCandidateFromWorkflowDraft(workflow)
	if err != nil {
		return AIChatCandidateResult{}, err
	}
	if candidate.ProposedManual.Operation.TargetType == "" {
		candidate.ProposedManual.Operation.TargetType = req.Frame.Target.Type
	}
	if candidate.ProposedManual.Operation.Action == "" {
		candidate.ProposedManual.Operation.Action = req.Frame.Operation.Action
	}
	return AIChatCandidateResult{Candidate: candidate}, nil
}

func GenerateAdaptationCandidate(base OpsManual, req AdaptationCandidateRequest) (ManualCandidate, error) {
	if strings.TrimSpace(base.ID) == "" {
		return ManualCandidate{}, fmt.Errorf("base manual id is required")
	}
	variant := slug(firstNonEmpty(req.VariantID, "variant"))
	now := time.Now().UTC().Format(time.RFC3339)
	manual := cloneManual(base)
	manual.ID = base.ID + "-" + variant
	manual.Title = strings.TrimSpace(base.Title + " " + req.TitleSuffix)
	manual.Status = ManualStatusDraft
	manual.WorkflowRef = req.WorkflowRef
	if manual.WorkflowRef.WorkflowID == "" {
		manual.WorkflowRef = base.WorkflowRef
	}
	manual.Applicability = req.Applicability
	manual.CreatedAt = now
	manual.UpdatedAt = now
	return ManualCandidate{
		ID:             "candidate-" + slug(manual.ID),
		SourceType:     "adaptation",
		SourceRefs:     []string{base.ID},
		ProposedManual: manual,
		ReviewStatus:   "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func ConvertScriptImportToWorkflowDraft(req ScriptImportRequest) (WorkflowDraftInput, error) {
	name := strings.TrimSpace(req.ScriptName)
	if name == "" {
		return WorkflowDraftInput{}, fmt.Errorf("script_name is required")
	}
	script := strings.TrimSpace(req.Script)
	if script == "" {
		return WorkflowDraftInput{}, fmt.Errorf("script is required")
	}
	id := "wf-" + slug(strings.TrimSuffix(name, ".sh"))
	meta := cloneMap(req.Metadata)
	if meta == nil {
		meta = map[string]any{}
	}
	meta["source_type"] = "script_import"
	raw := map[string]any{
		"version":     "draft",
		"name":        strings.TrimSuffix(name, ".sh"),
		"description": "Imported script workflow draft.",
		"vars": map[string]any{
			"target_instance": map[string]any{"required": true},
		},
		"steps": []map[string]any{{
			"name": "run_imported_script",
			"run":  script,
		}},
		"x_ops_manual": req.Metadata,
	}
	encoded, err := yaml.Marshal(raw)
	if err != nil {
		return WorkflowDraftInput{}, err
	}
	return WorkflowDraftInput{WorkflowID: id, WorkflowVersion: "draft", YAML: string(encoded), Metadata: meta}, nil
}

func workflowDraftFallbackYAML(workflowID string, version string) string {
	return fmt.Sprintf("version: %s\nname: %s\n", firstNonEmpty(version, "draft"), workflowID)
}

func applyWorkflowDraftMetadataOverrides(analysis *WorkflowManualAnalysis, metadata map[string]any) {
	if analysis == nil || len(metadata) == 0 {
		return
	}
	analysis.XOpsManual = mergeMetadata(analysis.XOpsManual, metadata)
	if value := metadataString(metadata, "target_type"); value != "" {
		analysis.Operation.TargetType = value
	}
	if value := metadataString(metadata, "action"); value != "" {
		analysis.Operation.Action = value
	}
	if value := metadataString(metadata, "risk_level"); value != "" {
		analysis.Operation.RiskLevel = value
	}
	if value := metadataString(metadata, "middleware"); value != "" {
		analysis.Applicability.Middleware = value
	}
	if values := metadataStringSlice(metadata, "execution_surface"); len(values) > 0 {
		analysis.Applicability.ExecutionSurface = values
	}
	if values := metadataStringSlice(metadata, "platform"); len(values) > 0 {
		analysis.Applicability.Platform = values
	}
	if values := metadataStringSlice(metadata, "os"); len(values) > 0 {
		analysis.Applicability.OS = values
	}
	for _, input := range metadataStringSlice(metadata, "required_inputs") {
		analysis.RequiredContext.RequiredInputs = appendUnique(analysis.RequiredContext.RequiredInputs, input)
	}
	for _, evidence := range metadataStringSlice(metadata, "required_evidence") {
		analysis.RequiredContext.RequiredEvidence = appendUnique(analysis.RequiredContext.RequiredEvidence, evidence)
	}
	for _, hint := range metadataStringSlice(metadata, "validation") {
		analysis.ValidationHints = appendUnique(analysis.ValidationHints, hint)
	}
	for _, hint := range metadataStringSlice(metadata, "cannot_use_when") {
		analysis.CannotUseHints = appendUnique(analysis.CannotUseHints, hint)
	}
}

func mergeMetadata(base, override map[string]any) map[string]any {
	out := cloneMap(base)
	if out == nil {
		out = map[string]any{}
	}
	for key, value := range override {
		out[key] = value
	}
	return out
}

func parameterRulesFromVars(vars map[string]any) map[string]ParameterRule {
	if len(vars) == 0 {
		return nil
	}
	rules := make(map[string]ParameterRule, len(vars))
	for key, value := range vars {
		rule := ParameterRule{Source: "workflow_var"}
		if valueMap, ok := value.(map[string]any); ok {
			if required, ok := valueMap["required"].(bool); ok {
				rule.Required = required
			}
			if validation, ok := valueMap["validation"].(string); ok {
				rule.Validation = validation
			}
		}
		rules[key] = rule
	}
	return rules
}

func metadataStringSlice(meta map[string]any, key string) []string {
	value, ok := meta[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return cloneStrings(typed)
	case []any:
		out := []string{}
		for _, item := range typed {
			out = appendUnique(out, fmt.Sprint(item))
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func digestIfRaw(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return DigestWorkflowYAML(raw)
}

func workflowDraftMarkdown(title, description string) string {
	if strings.TrimSpace(description) == "" {
		return "# " + strings.TrimSpace(title)
	}
	return "# " + strings.TrimSpace(title) + "\n\n" + strings.TrimSpace(description)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func slug(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	lower = strings.TrimSuffix(lower, ".yaml")
	lower = strings.TrimSuffix(lower, ".yml")
	lower = strings.TrimSuffix(lower, ".sh")
	re := regexp.MustCompile(`[^a-z0-9]+`)
	out := strings.Trim(re.ReplaceAllString(lower, "-"), "-")
	if out == "" {
		return "draft"
	}
	return out
}
