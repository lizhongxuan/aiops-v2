package experiencepack

import (
	"errors"
	"fmt"
	"strings"
)

type AssetID string

const (
	SchemaVersionGEP  = "gep-v1"
	SchemaVersionPack = "aiops-gep-pack-v1"

	PackStatusCandidate     = "candidate"
	PackStatusReviewPending = "review_pending"
	PackStatusApproved      = "approved"
	PackStatusEnabled       = "enabled"
	PackStatusPaused        = "paused"
	PackStatusRetired       = "retired"

	CategoryRepair   = "repair"
	CategoryOptimize = "optimize"
	CategoryInnovate = "innovate"

	UsageDiagnostic = "diagnostic"
	UsageGuided     = "guided"
	UsageExecutable = "executable"
)

var (
	ErrNotFound         = errors.New("experience pack not found")
	ErrConflict         = errors.New("asset_id conflict")
	ErrValidationFailed = errors.New("validation failed")
	ErrStateDenied      = errors.New("state transition denied")
)

type Metadata map[string]any

type AssetRef struct {
	Path    string  `json:"path,omitempty"`
	ID      string  `json:"id,omitempty"`
	AssetID AssetID `json:"asset_id,omitempty"`
}

type SkillAsset struct {
	Type    string  `json:"type,omitempty"`
	Path    string  `json:"path"`
	Title   string  `json:"title,omitempty"`
	Summary string  `json:"summary,omitempty"`
	Content string  `json:"content"`
	AssetID AssetID `json:"asset_id"`
}

type RequiredFileAsset struct {
	Path    string  `json:"path"`
	Kind    string  `json:"kind,omitempty"`
	Content string  `json:"content,omitempty"`
	AssetID AssetID `json:"asset_id"`
}

type AuthorizationScope struct {
	ID         string `json:"id,omitempty"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Searchable bool   `json:"searchable"`
	Reason     string `json:"reason,omitempty"`
}

type EnvironmentFingerprint struct {
	OS                 string            `json:"os,omitempty"`
	OSDistribution     string            `json:"os_distribution,omitempty"`
	OSVersion          string            `json:"os_version,omitempty"`
	Kernel             string            `json:"kernel,omitempty"`
	Architecture       string            `json:"architecture,omitempty"`
	PackageManager     string            `json:"package_manager,omitempty"`
	KubernetesVersion  string            `json:"kubernetes_version,omitempty"`
	ContainerRuntime   string            `json:"container_runtime,omitempty"`
	MiddlewareVersions map[string]string `json:"middleware_versions,omitempty"`
	CorootVersion      string            `json:"coroot_version,omitempty"`
	RunnerVersion      string            `json:"runner_version,omitempty"`
	AIOpsVersion       string            `json:"aiops_version,omitempty"`
	HostCount          int               `json:"host_count,omitempty"`
}

type BlastRadius struct {
	Files      int      `json:"files,omitempty"`
	Lines      int      `json:"lines,omitempty"`
	Hosts      int      `json:"hosts,omitempty"`
	Services   []string `json:"services,omitempty"`
	Pods       int      `json:"pods,omitempty"`
	Namespaces []string `json:"namespaces,omitempty"`
}

type Outcome struct {
	Status string  `json:"status"`
	Score  float64 `json:"score"`
	Note   string  `json:"note,omitempty"`
}

type GEPGene struct {
	Type            string         `json:"type"`
	SchemaVersion   string         `json:"schema_version"`
	ID              string         `json:"id"`
	Parent          string         `json:"parent,omitempty"`
	Category        string         `json:"category"`
	SignalsMatch    []string       `json:"signals_match"`
	Summary         string         `json:"summary"`
	Preconditions   []string       `json:"preconditions,omitempty"`
	Postconditions  []string       `json:"postconditions,omitempty"`
	Strategy        []string       `json:"strategy"`
	Constraints     map[string]any `json:"constraints"`
	Validation      []string       `json:"validation"`
	EpigeneticMarks []string       `json:"epigenetic_marks,omitempty"`
	Metadata        Metadata       `json:"metadata,omitempty"`
	ModelName       string         `json:"model_name,omitempty"`
	Domain          string         `json:"domain,omitempty"`
	EnvSelector     map[string]any `json:"env_selector,omitempty"`
	RunnerBindingID string         `json:"runner_binding_id,omitempty"`
	AssetID         AssetID        `json:"asset_id"`
}

type TriggerContext struct {
	Prompt         string   `json:"prompt,omitempty"`
	ReasoningTrace string   `json:"reasoning_trace,omitempty"`
	ContextSignals []string `json:"context_signals,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	AgentModel     string   `json:"agent_model,omitempty"`
	CaseID         string   `json:"case_id,omitempty"`
	RunnerRunID    string   `json:"runner_run_id,omitempty"`
	ProofID        string   `json:"proof_id,omitempty"`
}

type GEPCapsule struct {
	Type           string                 `json:"type"`
	SchemaVersion  string                 `json:"schema_version"`
	ID             string                 `json:"id"`
	Parent         string                 `json:"parent,omitempty"`
	Trigger        []string               `json:"trigger"`
	Gene           string                 `json:"gene"`
	GenesUsed      []string               `json:"genes_used,omitempty"`
	Summary        string                 `json:"summary"`
	Content        string                 `json:"content,omitempty"`
	Diff           string                 `json:"diff,omitempty"`
	CodeSnippet    string                 `json:"code_snippet,omitempty"`
	Strategy       []string               `json:"strategy,omitempty"`
	Confidence     float64                `json:"confidence"`
	BlastRadius    BlastRadius            `json:"blast_radius"`
	Outcome        Outcome                `json:"outcome"`
	SourceType     string                 `json:"source_type,omitempty"`
	ReusedAssetID  string                 `json:"reused_asset_id,omitempty"`
	SuccessStreak  int                    `json:"success_streak,omitempty"`
	EnvFingerprint EnvironmentFingerprint `json:"env_fingerprint"`
	TriggerContext TriggerContext         `json:"trigger_context,omitempty"`
	Metadata       Metadata               `json:"metadata,omitempty"`
	ModelName      string                 `json:"model_name,omitempty"`
	Domain         string                 `json:"domain,omitempty"`
	AssetID        AssetID                `json:"asset_id"`
}

type EvolutionEvent struct {
	Type               string                 `json:"type"`
	SchemaVersion      string                 `json:"schema_version"`
	ID                 string                 `json:"id"`
	Parent             string                 `json:"parent,omitempty"`
	Intent             string                 `json:"intent"`
	Signals            []string               `json:"signals"`
	GenesUsed          []string               `json:"genes_used"`
	MutationID         string                 `json:"mutation_id"`
	PersonalityState   map[string]any         `json:"personality_state,omitempty"`
	BlastRadius        map[string]any         `json:"blast_radius"`
	Outcome            Outcome                `json:"outcome"`
	CapsuleID          string                 `json:"capsule_id,omitempty"`
	SourceType         string                 `json:"source_type"`
	ReusedAssetID      string                 `json:"reused_asset_id,omitempty"`
	EnvFingerprint     EnvironmentFingerprint `json:"env_fingerprint,omitempty"`
	ValidationReportID string                 `json:"validation_report_id,omitempty"`
	TriggerContext     TriggerContext         `json:"trigger_context,omitempty"`
	ExecutionTrace     map[string]any         `json:"execution_trace,omitempty"`
	Meta               map[string]any         `json:"meta,omitempty"`
	ModelName          string                 `json:"model_name,omitempty"`
	AssetID            AssetID                `json:"asset_id"`
}

type MemoryGraphEvent struct {
	Type       string         `json:"type"`
	Kind       string         `json:"kind"`
	ID         string         `json:"id"`
	Timestamp  string         `json:"ts"`
	Signal     map[string]any `json:"signal,omitempty"`
	Gene       map[string]any `json:"gene,omitempty"`
	Outcome    *Outcome       `json:"outcome,omitempty"`
	Hypothesis map[string]any `json:"hypothesis,omitempty"`
	AssetID    AssetID        `json:"asset_id,omitempty"`
}

type AvoidCue struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	GeneID   string   `json:"gene_id"`
	Signals  []string `json:"signals"`
	Warning  string   `json:"warning"`
	Evidence string   `json:"evidence,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Blocking bool     `json:"blocking,omitempty"`
	AssetID  AssetID  `json:"asset_id"`
}

type RunnerBinding struct {
	Type              string         `json:"type"`
	SchemaVersion     string         `json:"schema_version"`
	ID                string         `json:"id"`
	WorkflowID        string         `json:"workflow_id"`
	WorkflowVersion   string         `json:"workflow_version,omitempty"`
	WorkflowName      string         `json:"workflow_name,omitempty"`
	Status            string         `json:"status,omitempty"`
	GeneID            string         `json:"gene_id,omitempty"`
	InputSchemaPath   string         `json:"input_schema_path,omitempty"`
	DryRunRequired    bool           `json:"dry_run_required"`
	ApprovalRequired  bool           `json:"approval_required"`
	HostLeaseRequired bool           `json:"host_lease_required"`
	ValidationPath    string         `json:"validation_path,omitempty"`
	RollbackPath      string         `json:"rollback_path,omitempty"`
	AllowedParams     []string       `json:"allowed_params,omitempty"`
	ForbiddenParams   []string       `json:"forbidden_params,omitempty"`
	EnvSelector       map[string]any `json:"env_selector,omitempty"`
	Published         bool           `json:"published,omitempty"`
	AssetID           AssetID        `json:"asset_id"`
}

type ExperiencePackManifest struct {
	Type                string               `json:"type"`
	SchemaVersion       string               `json:"schema_version"`
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Title               string               `json:"title,omitempty"`
	Summary             string               `json:"summary"`
	Domain              string               `json:"domain,omitempty"`
	Category            string               `json:"category"`
	UsageMode           string               `json:"usage_mode,omitempty"`
	Status              string               `json:"status"`
	ReviewStatus        string               `json:"review_status,omitempty"`
	Enabled             bool                 `json:"enabled"`
	Skill               SkillAsset           `json:"skill"`
	RequiredFiles       []RequiredFileAsset  `json:"required_files,omitempty"`
	Genes               []AssetRef           `json:"genes"`
	Capsules            []AssetRef           `json:"capsules,omitempty"`
	RunnerBinding       *AssetRef            `json:"runner_binding,omitempty"`
	RunnerBindings      []AssetRef           `json:"runner_bindings,omitempty"`
	Events              AssetRef             `json:"events,omitempty"`
	MemoryGraph         AssetRef             `json:"memory_graph,omitempty"`
	Lineage             map[string]string    `json:"lineage,omitempty"`
	AuthorizationScopes []AuthorizationScope `json:"authorization_scopes,omitempty"`
	Metadata            Metadata             `json:"metadata,omitempty"`
	AssetID             AssetID              `json:"asset_id"`
}

type ValidationReport struct {
	Passed         bool             `json:"passed"`
	BlockedReasons []string         `json:"blockedReasons"`
	CompiledTasks  []ValidationTask `json:"compiledTasks"`
	Redacted       bool             `json:"redacted"`
}

type RunnerWorkflowCandidate struct {
	ID              string         `json:"id"`
	WorkflowName    string         `json:"workflow_name"`
	Status          string         `json:"status"`
	Steps           []RunnerStep   `json:"steps"`
	Parameters      []string       `json:"parameters"`
	Guards          map[string]any `json:"guards"`
	StudioDraftLink string         `json:"studio_draft_link,omitempty"`
	AssetID         AssetID        `json:"asset_id,omitempty"`
}

type RunnerStep struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
}

func (g GEPGene) Validate() error {
	var missing []string
	if strings.TrimSpace(g.Type) != "Gene" {
		missing = append(missing, "type")
	}
	if strings.TrimSpace(g.SchemaVersion) == "" {
		missing = append(missing, "schema_version")
	}
	if strings.TrimSpace(g.ID) == "" {
		missing = append(missing, "id")
	}
	if !validCategory(g.Category) {
		missing = append(missing, "category")
	}
	if len(g.SignalsMatch) == 0 {
		missing = append(missing, "signals_match")
	}
	if len(strings.TrimSpace(g.Summary)) < 10 {
		missing = append(missing, "summary")
	}
	if len(g.Strategy) == 0 {
		missing = append(missing, "strategy")
	}
	if len(g.Constraints) == 0 {
		missing = append(missing, "constraints")
	}
	if len(g.Validation) == 0 {
		missing = append(missing, "validation")
	}
	if !ValidAssetID(g.AssetID) {
		missing = append(missing, "asset_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: gene missing %s", ErrValidationFailed, strings.Join(missing, ", "))
	}
	return nil
}

func (c GEPCapsule) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Type) != "Capsule" {
		missing = append(missing, "type")
	}
	if strings.TrimSpace(c.ID) == "" {
		missing = append(missing, "id")
	}
	if len(c.Trigger) == 0 {
		missing = append(missing, "trigger")
	}
	if strings.TrimSpace(c.Gene) == "" {
		missing = append(missing, "gene")
	}
	if strings.TrimSpace(c.Summary) == "" {
		missing = append(missing, "summary")
	}
	if !capsuleHasSubstance(c) {
		missing = append(missing, "content/diff/strategy/code_snippet")
	}
	if c.Outcome.Status == "" {
		missing = append(missing, "outcome")
	}
	if envFingerprintEmpty(c.EnvFingerprint) {
		missing = append(missing, "env_fingerprint")
	}
	if !ValidAssetID(c.AssetID) {
		missing = append(missing, "asset_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: capsule missing %s", ErrValidationFailed, strings.Join(missing, ", "))
	}
	return nil
}

func (m ExperiencePackManifest) Validate() error {
	var missing []string
	if strings.TrimSpace(m.ID) == "" {
		missing = append(missing, "id")
	}
	if strings.TrimSpace(m.Skill.Path) != "skills/SKILL.md" && strings.TrimSpace(m.Skill.Content) == "" {
		missing = append(missing, "skills/SKILL.md")
	}
	if len(m.Genes) == 0 {
		missing = append(missing, "genes")
	}
	if !ValidAssetID(m.AssetID) {
		missing = append(missing, "asset_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: manifest missing %s", ErrValidationFailed, strings.Join(missing, ", "))
	}
	return nil
}

func (m ExperiencePackManifest) CanEnable() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if !m.hasRequiredFile("files/validation.md") || !m.hasRequiredFile("files/rollback.md") {
		return fmt.Errorf("%w: validation and rollback files are required", ErrValidationFailed)
	}
	if len(m.AuthorizationScopes) == 0 {
		return fmt.Errorf("%w: authorization scope is required", ErrValidationFailed)
	}
	return nil
}

func (m ExperiencePackManifest) hasRequiredFile(path string) bool {
	for _, file := range m.RequiredFiles {
		if file.Path == path {
			return true
		}
	}
	return false
}

func validCategory(category string) bool {
	return category == CategoryRepair || category == CategoryOptimize || category == CategoryInnovate
}

func capsuleHasSubstance(c GEPCapsule) bool {
	return len(strings.TrimSpace(c.Content)) >= 50 ||
		len(strings.TrimSpace(c.Diff)) >= 50 ||
		len(strings.TrimSpace(c.CodeSnippet)) >= 50 ||
		len(c.Strategy) > 0
}

func envFingerprintEmpty(env EnvironmentFingerprint) bool {
	return strings.TrimSpace(env.OS) == "" &&
		strings.TrimSpace(env.OSDistribution) == "" &&
		strings.TrimSpace(env.PackageManager) == "" &&
		len(env.MiddlewareVersions) == 0
}

func ValidAssetID(id AssetID) bool {
	value := string(id)
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	hex := strings.TrimPrefix(value, "sha256:")
	if len(hex) != 64 {
		return false
	}
	for _, ch := range hex {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	return true
}
