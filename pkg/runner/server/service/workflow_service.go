package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"runner/workflow"
	"runner/workflowstore"
)

type workflowMeta struct {
	Name                string            `json:"name"`
	Labels              map[string]string `json:"labels,omitempty"`
	SaveNote            string            `json:"save_note,omitempty"`
	Status              string            `json:"status,omitempty"`
	ValidatedGraphHash  string            `json:"validated_graph_hash,omitempty"`
	ValidatedLayoutHash string            `json:"validated_layout_hash,omitempty"`
	ValidatedAt         time.Time         `json:"validated_at,omitempty"`
	ValidatedBy         string            `json:"validated_by,omitempty"`
	DryRunGraphHash     string            `json:"dry_run_graph_hash,omitempty"`
	DryRunLayoutHash    string            `json:"dry_run_layout_hash,omitempty"`
	DryRunAt            time.Time         `json:"dry_run_at,omitempty"`
	DryRunBy            string            `json:"dry_run_by,omitempty"`
	PublishedGraphHash  string            `json:"published_graph_hash,omitempty"`
	PublishedAt         time.Time         `json:"published_at,omitempty"`
	CreatedAt           time.Time         `json:"created_at,omitempty"`
	UpdatedAt           time.Time         `json:"updated_at,omitempty"`
	Deleted             bool              `json:"deleted,omitempty"`
}

type WorkflowService struct {
	store              *workflowstore.Store
	commitStore        *WorkflowCommitStore
	tenantID           string
	metaPath           string
	historyPath        string
	referenceChecker   WorkflowReferenceChecker
	referenceGuardMode WorkflowReferenceGuardMode
	mu                 sync.Mutex
}

const workflowBundleFormatVersion = "runner.workflow.bundle/v1"

func NewWorkflowService(dir string) *WorkflowService {
	var commitStore *WorkflowCommitStore
	if strings.TrimSpace(dir) != "" {
		commitStore, _ = NewWorkflowCommitStore(filepath.Join(dir, ".transactions"))
	}
	return &WorkflowService{
		store:              workflowstore.New(dir),
		commitStore:        commitStore,
		tenantID:           VisualWorkflowDefaultTenantID,
		metaPath:           filepath.Join(dir, ".meta.json"),
		historyPath:        filepath.Join(dir, ".history"),
		referenceGuardMode: WorkflowReferenceGuardModeEnforce,
	}
}

func (s *WorkflowService) List(ctx context.Context, labels map[string]string) ([]*WorkflowRecord, error) {
	return s.listWorkflows(ctx, labels)
}

func (s *WorkflowService) listLegacyWorkflows(_ context.Context, labels map[string]string) ([]*WorkflowRecord, error) {
	items, err := s.store.List()
	if err != nil {
		return nil, err
	}
	metas, err := s.loadMeta()
	if err != nil {
		return nil, err
	}

	out := make([]*WorkflowRecord, 0, len(items))
	for _, item := range items {
		record := &WorkflowRecord{
			Name:        item.Name,
			Description: item.Description,
			UpdatedAt:   item.UpdatedAt,
			Status:      WorkflowStatusDraft,
		}
		if meta, ok := metas[item.Name]; ok {
			record.Labels = copyStringMap(meta.Labels)
			record.SaveNote = meta.SaveNote
			record.Status = normalizeWorkflowStatus(meta.Status)
			record.ValidatedGraphHash = strings.TrimSpace(meta.ValidatedGraphHash)
			record.ValidatedLayoutHash = strings.TrimSpace(meta.ValidatedLayoutHash)
			record.ValidatedAt = meta.ValidatedAt
			record.ValidatedBy = strings.TrimSpace(meta.ValidatedBy)
			record.DryRunGraphHash = strings.TrimSpace(meta.DryRunGraphHash)
			record.DryRunLayoutHash = strings.TrimSpace(meta.DryRunLayoutHash)
			record.DryRunAt = meta.DryRunAt
			record.DryRunBy = strings.TrimSpace(meta.DryRunBy)
			record.PublishedGraphHash = strings.TrimSpace(meta.PublishedGraphHash)
			record.PublishedAt = meta.PublishedAt
			record.CreatedAt = meta.CreatedAt
			if !meta.UpdatedAt.IsZero() {
				record.UpdatedAt = meta.UpdatedAt
			}
		}
		if !matchLabels(record.Labels, labels) {
			continue
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *WorkflowService) Get(ctx context.Context, name string) (*WorkflowRecord, error) {
	return s.getWorkflow(ctx, name)
}

func (s *WorkflowService) getLegacyWorkflow(_ context.Context, name string) (*WorkflowRecord, error) {
	wf, raw, err := s.store.Get(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	metas, err := s.loadMeta()
	if err != nil {
		return nil, err
	}
	record := &WorkflowRecord{
		Name:        wf.Name,
		Description: wf.Description,
		Version:     wf.Version,
		RawYAML:     append([]byte{}, raw...),
		Status:      WorkflowStatusDraft,
	}
	if meta, ok := metas[wf.Name]; ok {
		record.Labels = copyStringMap(meta.Labels)
		record.SaveNote = meta.SaveNote
		record.Status = normalizeWorkflowStatus(meta.Status)
		record.ValidatedGraphHash = strings.TrimSpace(meta.ValidatedGraphHash)
		record.ValidatedLayoutHash = strings.TrimSpace(meta.ValidatedLayoutHash)
		record.ValidatedAt = meta.ValidatedAt
		record.ValidatedBy = strings.TrimSpace(meta.ValidatedBy)
		record.DryRunGraphHash = strings.TrimSpace(meta.DryRunGraphHash)
		record.DryRunLayoutHash = strings.TrimSpace(meta.DryRunLayoutHash)
		record.DryRunAt = meta.DryRunAt
		record.DryRunBy = strings.TrimSpace(meta.DryRunBy)
		record.PublishedGraphHash = strings.TrimSpace(meta.PublishedGraphHash)
		record.PublishedAt = meta.PublishedAt
		record.CreatedAt = meta.CreatedAt
		record.UpdatedAt = meta.UpdatedAt
	}
	if record.UpdatedAt.IsZero() {
		if summaries, err := s.store.List(); err == nil {
			for _, item := range summaries {
				if item.Name == wf.Name {
					record.UpdatedAt = item.UpdatedAt
					break
				}
			}
		}
	}
	return record, nil
}

func (s *WorkflowService) Create(ctx context.Context, record *WorkflowRecord) error {
	return s.createWorkflow(ctx, record, "create", nil, false)
}

func (s *WorkflowService) Update(ctx context.Context, name string, record *WorkflowRecord) error {
	return s.updateWorkflow(ctx, name, record, "update", nil, false)
}

func (s *WorkflowService) Publish(ctx context.Context, name string, opts WorkflowPublishOptions) (*WorkflowRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	existing, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(ctx, existing.RawYAML); err != nil {
		return nil, err
	}
	hashes, err := workflowYAMLGraphHashes(existing.RawYAML)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(existing.ValidatedGraphHash) == "" || existing.ValidatedGraphHash != hashes.SemanticHash || existing.ValidatedAt.IsZero() {
		return nil, fmt.Errorf("%w: publish requires current validated_graph_hash; validate workflow before publishing", ErrInvalid)
	}
	if expected := strings.TrimSpace(opts.ValidatedGraphHash); expected != "" && expected != existing.ValidatedGraphHash {
		return nil, fmt.Errorf("%w: publish validated_graph_hash does not match current graph", ErrInvalid)
	}
	if strings.TrimSpace(existing.DryRunGraphHash) == "" || existing.DryRunGraphHash != hashes.SemanticHash || existing.DryRunAt.IsZero() {
		return nil, fmt.Errorf("%w: publish requires current dry_run_passed graph hash; run Dry Run before publishing", ErrInvalid)
	}
	if expected := strings.TrimSpace(opts.DryRunGraphHash); expected != "" && expected != existing.DryRunGraphHash {
		return nil, fmt.Errorf("%w: publish dry_run_graph_hash does not match current graph", ErrInvalid)
	}
	wf, err := workflow.Load(existing.RawYAML)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if risky := collectWorkflowHighRiskActions(wf); len(risky) > 0 && !opts.RiskAcknowledged {
		return nil, fmt.Errorf("%w: high risk actions require risk_acknowledged=true before publishing: %s", ErrInvalid, strings.Join(risky, ", "))
	}
	if capabilityWarnings := collectCapabilityPrecheckWarnings(wf); len(capabilityWarnings) > 0 {
		return nil, fmt.Errorf("%w: agent capability constraints failed before publishing: %s", ErrInvalid, firstIssueMessage(capabilityWarnings))
	}
	dryRunWarnings := collectWorkflowPublishWarnings(wf)
	if len(dryRunWarnings) > 0 && !opts.WarningAcknowledged {
		return nil, fmt.Errorf("%w: dry-run warnings require warning_acknowledged=true before publishing: %s", ErrInvalid, firstIssueMessage(dryRunWarnings))
	}
	saveNote := existing.SaveNote
	if opts.SaveNoteSet || strings.TrimSpace(opts.SaveNote) != "" {
		saveNote = strings.TrimSpace(opts.SaveNote)
	}
	if err := s.upsertCommittedMeta(ctx, name, existing.Labels, saveNote, WorkflowStatusPublished, time.Now().UTC(), workflowMetaHashFields{
		ValidatedGraphHash:  hashes.SemanticHash,
		ValidatedLayoutHash: hashes.LayoutHash,
		ValidatedAt:         existing.ValidatedAt,
		ValidatedBy:         existing.ValidatedBy,
		DryRunGraphHash:     hashes.SemanticHash,
		DryRunLayoutHash:    hashes.LayoutHash,
		DryRunAt:            existing.DryRunAt,
		DryRunBy:            existing.DryRunBy,
		PublishedGraphHash:  hashes.SemanticHash,
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

func (s *WorkflowService) ValidateWorkflow(ctx context.Context, name string, opts WorkflowValidateOptions) (*WorkflowRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	existing, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.Validate(ctx, existing.RawYAML); err != nil {
		return nil, err
	}
	hashes, err := workflowYAMLGraphHashes(existing.RawYAML)
	if err != nil {
		return nil, err
	}
	if err := s.upsertCommittedMeta(ctx, name, existing.Labels, existing.SaveNote, WorkflowStatusValidated, existing.PublishedAt, workflowMetaHashFields{
		ValidatedGraphHash:  hashes.SemanticHash,
		ValidatedLayoutHash: hashes.LayoutHash,
		ValidatedAt:         time.Now().UTC(),
		ValidatedBy:         strings.TrimSpace(opts.Actor),
		DryRunGraphHash:     "",
		DryRunLayoutHash:    "",
		DryRunAt:            time.Time{},
		DryRunBy:            "",
		PublishedGraphHash:  "",
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

func (s *WorkflowService) MarkDryRunPassed(ctx context.Context, name string, opts WorkflowDryRunOptions) (*WorkflowRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	existing, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	hashes, err := workflowYAMLGraphHashes(existing.RawYAML)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(existing.ValidatedGraphHash) == "" || existing.ValidatedGraphHash != hashes.SemanticHash || existing.ValidatedAt.IsZero() {
		return nil, fmt.Errorf("%w: dry_run_passed requires current validated_graph_hash; validate workflow before dry run", ErrInvalid)
	}
	if expected := strings.TrimSpace(opts.ExpectedGraphHash); expected != "" && expected != hashes.SemanticHash {
		return nil, fmt.Errorf("%w: dry-run graph hash does not match saved workflow", ErrConflict)
	}
	if err := s.upsertCommittedMeta(ctx, name, existing.Labels, existing.SaveNote, WorkflowStatusDryRunPassed, existing.PublishedAt, workflowMetaHashFields{
		ValidatedGraphHash:  hashes.SemanticHash,
		ValidatedLayoutHash: hashes.LayoutHash,
		ValidatedAt:         existing.ValidatedAt,
		ValidatedBy:         existing.ValidatedBy,
		DryRunGraphHash:     hashes.SemanticHash,
		DryRunLayoutHash:    hashes.LayoutHash,
		DryRunAt:            time.Now().UTC(),
		DryRunBy:            strings.TrimSpace(opts.Actor),
		PublishedGraphHash:  "",
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

func (s *WorkflowService) ListVersions(ctx context.Context, name string) ([]*WorkflowVersionRecord, error) {
	return s.listWorkflowVersions(ctx, name)
}

func (s *WorkflowService) listLegacyWorkflowVersions(_ context.Context, name string) ([]*WorkflowVersionRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	entries, err := os.ReadDir(s.historyDir(name))
	if err != nil {
		if os.IsNotExist(err) {
			return []*WorkflowVersionRecord{}, nil
		}
		return nil, err
	}
	out := make([]*WorkflowVersionRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		version, err := s.readVersionFile(filepath.Join(s.historyDir(name), entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *WorkflowService) GetVersion(ctx context.Context, name, versionID string) (*WorkflowVersionRecord, error) {
	return s.getWorkflowVersion(ctx, name, versionID)
}

func (s *WorkflowService) getLegacyWorkflowVersion(_ context.Context, name, versionID string) (*WorkflowVersionRecord, error) {
	name = strings.TrimSpace(name)
	versionID = strings.TrimSpace(versionID)
	if name == "" || versionID == "" {
		return nil, fmt.Errorf("%w: workflow name and version id are required", ErrInvalid)
	}
	version, err := s.readVersionFile(s.versionFilePath(name, versionID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return version, nil
}

func (s *WorkflowService) Rollback(ctx context.Context, name, versionID string, opts WorkflowRollbackOptions) (*WorkflowRecord, error) {
	return s.rollbackWorkflow(ctx, name, versionID, opts)
}

func (s *WorkflowService) ExportBundle(ctx context.Context, name string) (*WorkflowBundle, error) {
	record, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	versions, err := s.ListVersions(ctx, record.Name)
	if err != nil {
		return nil, err
	}
	bundle := &WorkflowBundle{
		BundleVersion: workflowBundleFormatVersion,
		ExportedAt:    time.Now().UTC(),
		Name:          record.Name,
		Description:   record.Description,
		Version:       record.Version,
		YAML:          string(record.RawYAML),
		Labels:        copyStringMap(record.Labels),
		SaveNote:      record.SaveNote,
		Status:        normalizeWorkflowStatus(record.Status),
		PublishedAt:   record.PublishedAt,
		Versions:      make([]WorkflowBundleVersion, 0, len(versions)),
	}
	for _, version := range versions {
		bundle.Versions = append(bundle.Versions, workflowBundleVersionFromRecord(version))
	}
	return bundle, nil
}

func (s *WorkflowService) ImportBundle(ctx context.Context, bundle *WorkflowBundle, opts WorkflowImportOptions) (*WorkflowRecord, error) {
	return s.importWorkflowBundle(ctx, bundle, opts)
}

func (s *WorkflowService) Delete(ctx context.Context, name string) error {
	return s.deleteWorkflow(ctx, name)
}

func (s *WorkflowService) Validate(_ context.Context, yamlContent []byte) error {
	wf, err := workflow.Load(yamlContent)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

type workflowVersionFile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Version     string    `json:"version,omitempty"`
	Status      string    `json:"status,omitempty"`
	SaveNote    string    `json:"save_note,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Checksum    string    `json:"checksum,omitempty"`
	YAML        string    `json:"yaml"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

func (s *WorkflowService) readVersionFile(path string) (*WorkflowVersionRecord, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file workflowVersionFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	return &WorkflowVersionRecord{
		ID:          file.ID,
		Name:        file.Name,
		Description: file.Description,
		Version:     file.Version,
		Status:      normalizeWorkflowStatus(file.Status),
		SaveNote:    file.SaveNote,
		Reason:      file.Reason,
		Checksum:    file.Checksum,
		RawYAML:     []byte(file.YAML),
		PublishedAt: file.PublishedAt,
		CreatedAt:   file.CreatedAt,
	}, nil
}

func (s *WorkflowService) historyDir(name string) string {
	return filepath.Join(s.historyPath, url.PathEscape(strings.TrimSpace(name)))
}

func (s *WorkflowService) versionFilePath(name, versionID string) string {
	return filepath.Join(s.historyDir(name), url.PathEscape(strings.TrimSpace(versionID))+".json")
}

func workflowYAMLChecksum(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type workflowMetaHashFields struct {
	ValidatedGraphHash  string
	ValidatedLayoutHash string
	ValidatedAt         time.Time
	ValidatedBy         string
	DryRunGraphHash     string
	DryRunLayoutHash    string
	DryRunAt            time.Time
	DryRunBy            string
	PublishedGraphHash  string
}

func (s *WorkflowService) loadMeta() (map[string]workflowMeta, error) {
	return s.loadMetaNoLock()
}

func (s *WorkflowService) loadMetaNoLock() (map[string]workflowMeta, error) {
	raw, err := os.ReadFile(s.metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]workflowMeta{}, nil
		}
		return nil, err
	}
	data := map[string]workflowMeta{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func matchLabels(actual map[string]string, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}
	for k, v := range filter {
		if strings.TrimSpace(actual[k]) != strings.TrimSpace(v) {
			return false
		}
	}
	return true
}

func copyStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func normalizeWorkflowStatus(status string) string {
	switch strings.TrimSpace(status) {
	case WorkflowStatusPublished:
		return WorkflowStatusPublished
	case WorkflowStatusDryRunPassed:
		return WorkflowStatusDryRunPassed
	case WorkflowStatusValidated:
		return WorkflowStatusValidated
	default:
		return WorkflowStatusDraft
	}
}

func workflowBundleVersionFromRecord(item *WorkflowVersionRecord) WorkflowBundleVersion {
	if item == nil {
		return WorkflowBundleVersion{}
	}
	return WorkflowBundleVersion{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		Version:     item.Version,
		Status:      item.Status,
		SaveNote:    item.SaveNote,
		Reason:      item.Reason,
		Checksum:    item.Checksum,
		YAML:        string(item.RawYAML),
		PublishedAt: item.PublishedAt,
		CreatedAt:   item.CreatedAt,
	}
}

func validateWorkflowBundleVersions(workflowName string, versions []WorkflowBundleVersion) error {
	seen := map[string]struct{}{}
	for _, version := range versions {
		id := strings.TrimSpace(version.ID)
		if id == "" {
			return fmt.Errorf("%w: workflow bundle version id is required", ErrInvalid)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate workflow bundle version %q", ErrInvalid, id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(version.YAML) == "" {
			return fmt.Errorf("%w: workflow bundle version yaml is required", ErrInvalid)
		}
		wf, err := workflow.Load([]byte(version.YAML))
		if err != nil {
			return fmt.Errorf("%w: invalid workflow bundle version %q: %v", ErrInvalid, id, err)
		}
		if err := wf.Validate(); err != nil {
			return fmt.Errorf("%w: invalid workflow bundle version %q: %v", ErrInvalid, id, err)
		}
		if strings.TrimSpace(wf.Name) != workflowName {
			return fmt.Errorf("%w: workflow bundle version %q name mismatch", ErrInvalid, id)
		}
	}
	return nil
}

func collectWorkflowHighRiskActions(wf workflow.Workflow) []string {
	catalog := NewActionCatalog()
	var risky []string
	for _, step := range wf.Steps {
		if isHighRiskAction(catalog, step.Action) {
			risky = append(risky, fmt.Sprintf("step %q uses %q", step.Name, step.Action))
		}
	}
	for _, handler := range wf.Handlers {
		if isHighRiskAction(catalog, handler.Action) {
			risky = append(risky, fmt.Sprintf("handler %q uses %q", handler.Name, handler.Action))
		}
	}
	return risky
}

func collectWorkflowPublishWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	var warnings []VisualWorkflowIssue
	warnings = append(warnings, collectUndefinedVariableWarnings(wf)...)
	warnings = append(warnings, collectScriptSecurityScanWarnings(wf)...)
	return dedupeIssues(warnings)
}

func isHighRiskAction(catalog *ActionCatalog, action string) bool {
	spec, ok := catalog.Get(context.Background(), strings.TrimSpace(action))
	return ok && strings.TrimSpace(spec.Risk) == "high"
}
