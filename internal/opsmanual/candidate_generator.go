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
	parsed := workflowDraftYAML{}
	if strings.TrimSpace(input.YAML) != "" {
		if err := yaml.Unmarshal([]byte(input.YAML), &parsed); err != nil {
			return ManualCandidate{}, err
		}
	}
	meta := mergeMetadata(parsed.XOpsManual, input.Metadata)
	version := firstNonEmpty(input.WorkflowVersion, parsed.Version)
	digest := firstNonEmpty(input.WorkflowDigest, digestIfRaw(input.YAML))
	title := firstNonEmpty(metadataString(meta, "title"), parsed.Name, workflowID)
	now := time.Now().UTC().Format(time.RFC3339)
	manual := OpsManual{
		ID:             "manual-candidate-" + slug(workflowID),
		ManualFamilyID: firstNonEmpty(metadataString(meta, "manual_family_id"), slug(workflowID)),
		Title:          title,
		Status:         ManualStatusDraft,
		Version:        version,
		Owner:          metadataString(meta, "owner"),
		WorkflowRef: WorkflowRef{
			WorkflowID:      workflowID,
			WorkflowVersion: version,
			WorkflowDigest:  digest,
			StorageURI:      strings.TrimSpace(input.StorageURI),
		},
		Operation: OperationProfile{
			TargetType: metadataString(meta, "target_type"),
			Action:     metadataString(meta, "action"),
			RiskLevel:  metadataString(meta, "risk_level"),
		},
		Applicability: ApplicabilityProfile{
			Middleware:       metadataString(meta, "middleware"),
			ExecutionSurface: metadataStringSlice(meta, "execution_surface"),
			Platform:         metadataStringSlice(meta, "platform"),
			OS:               metadataStringSlice(meta, "os"),
		},
		RequiredContext: RequiredContext{
			RequiredInputs:   metadataStringSlice(meta, "required_inputs"),
			RequiredEvidence: metadataStringSlice(meta, "required_evidence"),
		},
		ParameterRules:   parameterRulesFromVars(parsed.Vars),
		Validation:       metadataStringSlice(meta, "validation"),
		CannotUseWhen:    metadataStringSlice(meta, "cannot_use_when"),
		RiskNotes:        metadataStringSlice(meta, "risk_notes"),
		DocumentMarkdown: workflowDraftMarkdown(title, parsed.Description),
		SearchDoc:        strings.TrimSpace(title + " " + parsed.Description),
		Metadata:         cloneMap(meta),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if len(manual.RequiredContext.RequiredInputs) == 0 {
		for name, rule := range manual.ParameterRules {
			if rule.Required {
				manual.RequiredContext.RequiredInputs = append(manual.RequiredContext.RequiredInputs, name)
			}
		}
	}
	return ManualCandidate{
		ID:             "candidate-" + slug(workflowID),
		SourceType:     "workflow_draft",
		SourceRefs:     []string{workflowID},
		ProposedManual: manual,
		ReviewStatus:   "pending",
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
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
