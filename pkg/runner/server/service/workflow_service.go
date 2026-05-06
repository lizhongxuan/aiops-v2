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
}

type WorkflowService struct {
	store       *workflowstore.Store
	metaPath    string
	historyPath string
	mu          sync.Mutex
}

const workflowBundleFormatVersion = "runner.workflow.bundle/v1"

func NewWorkflowService(dir string) *WorkflowService {
	return &WorkflowService{
		store:       workflowstore.New(dir),
		metaPath:    filepath.Join(dir, ".meta.json"),
		historyPath: filepath.Join(dir, ".history"),
	}
}

func (s *WorkflowService) List(_ context.Context, labels map[string]string) ([]*WorkflowRecord, error) {
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

func (s *WorkflowService) Get(_ context.Context, name string) (*WorkflowRecord, error) {
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

func (s *WorkflowService) Create(_ context.Context, record *WorkflowRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty workflow record", ErrInvalid)
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	raw := record.RawYAML
	if len(raw) == 0 {
		return fmt.Errorf("%w: yaml is required", ErrInvalid)
	}
	wf, err := workflow.Load(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if strings.TrimSpace(wf.Name) != name {
		return fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	if _, err := s.Get(context.Background(), name); err == nil {
		return ErrAlreadyExists
	} else if err != nil && err != ErrNotFound {
		return err
	}
	if _, err := s.store.Put(name, raw); err != nil {
		return err
	}
	if err := s.upsertMeta(name, record.Labels, strings.TrimSpace(record.SaveNote), WorkflowStatusDraft, time.Time{}, true); err != nil {
		return err
	}
	return s.saveCurrentVersion(context.Background(), name, "create")
}

func (s *WorkflowService) Update(_ context.Context, name string, record *WorkflowRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty workflow record", ErrInvalid)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	raw := record.RawYAML
	if len(raw) == 0 {
		return fmt.Errorf("%w: yaml is required", ErrInvalid)
	}
	wf, err := workflow.Load(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if strings.TrimSpace(wf.Name) != name {
		return fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	existing, err := s.Get(context.Background(), name)
	if err != nil {
		return err
	}
	if _, err := s.store.Put(name, raw); err != nil {
		return err
	}
	labels := record.Labels
	if labels == nil {
		labels = existing.Labels
	}
	saveNote := existing.SaveNote
	if record.SaveNoteSet || strings.TrimSpace(record.SaveNote) != "" {
		saveNote = strings.TrimSpace(record.SaveNote)
	}
	status := WorkflowStatusDraft
	validatedGraphHash := ""
	validatedLayoutHash := ""
	validatedAt := time.Time{}
	validatedBy := ""
	dryRunGraphHash := ""
	dryRunLayoutHash := ""
	dryRunAt := time.Time{}
	dryRunBy := ""
	publishedGraphHash := strings.TrimSpace(existing.PublishedGraphHash)
	if hashes, err := workflowYAMLGraphHashes(raw); err == nil && strings.TrimSpace(existing.ValidatedGraphHash) != "" && hashes.SemanticHash == existing.ValidatedGraphHash {
		status = normalizeWorkflowStatus(existing.Status)
		validatedGraphHash = existing.ValidatedGraphHash
		validatedLayoutHash = hashes.LayoutHash
		validatedAt = existing.ValidatedAt
		validatedBy = existing.ValidatedBy
		if strings.TrimSpace(existing.DryRunGraphHash) == hashes.SemanticHash {
			dryRunGraphHash = existing.DryRunGraphHash
			dryRunLayoutHash = hashes.LayoutHash
			dryRunAt = existing.DryRunAt
			dryRunBy = existing.DryRunBy
		}
		if status != WorkflowStatusPublished {
			publishedGraphHash = ""
		}
	}
	if err := s.upsertMetaWithHashes(name, labels, saveNote, status, existing.PublishedAt, false, workflowMetaHashFields{
		ValidatedGraphHash:  validatedGraphHash,
		ValidatedLayoutHash: validatedLayoutHash,
		ValidatedAt:         validatedAt,
		ValidatedBy:         validatedBy,
		DryRunGraphHash:     dryRunGraphHash,
		DryRunLayoutHash:    dryRunLayoutHash,
		DryRunAt:            dryRunAt,
		DryRunBy:            dryRunBy,
		PublishedGraphHash:  publishedGraphHash,
	}); err != nil {
		return err
	}
	return s.saveCurrentVersion(context.Background(), name, "update")
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
	if err := s.upsertMetaWithHashes(name, existing.Labels, saveNote, WorkflowStatusPublished, time.Now().UTC(), false, workflowMetaHashFields{
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
	record, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.saveVersionSnapshot(record, "publish"); err != nil {
		return nil, err
	}
	return record, nil
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
	if err := s.upsertMetaWithHashes(name, existing.Labels, existing.SaveNote, WorkflowStatusValidated, existing.PublishedAt, false, workflowMetaHashFields{
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
	if err := s.upsertMetaWithHashes(name, existing.Labels, existing.SaveNote, WorkflowStatusDryRunPassed, existing.PublishedAt, false, workflowMetaHashFields{
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

func (s *WorkflowService) ListVersions(_ context.Context, name string) ([]*WorkflowVersionRecord, error) {
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

func (s *WorkflowService) GetVersion(_ context.Context, name, versionID string) (*WorkflowVersionRecord, error) {
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
	name = strings.TrimSpace(name)
	version, err := s.GetVersion(ctx, name, versionID)
	if err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Put(name, version.RawYAML); err != nil {
		return nil, err
	}
	saveNote := strings.TrimSpace(opts.SaveNote)
	if saveNote == "" {
		saveNote = "rollback to " + version.ID
	}
	if err := s.upsertMeta(name, existing.Labels, saveNote, WorkflowStatusDraft, existing.PublishedAt, false); err != nil {
		return nil, err
	}
	record, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := s.saveVersionSnapshot(record, "rollback:"+version.ID); err != nil {
		return nil, err
	}
	return record, nil
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
	if bundle == nil {
		return nil, fmt.Errorf("%w: empty workflow bundle", ErrInvalid)
	}
	if bundle.BundleVersion != "" && bundle.BundleVersion != workflowBundleFormatVersion {
		return nil, fmt.Errorf("%w: unsupported workflow bundle version %q", ErrInvalid, bundle.BundleVersion)
	}
	if strings.TrimSpace(bundle.YAML) == "" {
		return nil, fmt.Errorf("%w: workflow bundle yaml is required", ErrInvalid)
	}
	raw := []byte(bundle.YAML)
	wf, err := workflow.Load(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	name := strings.TrimSpace(bundle.Name)
	if name == "" {
		name = strings.TrimSpace(wf.Name)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: workflow bundle name is required", ErrInvalid)
	}
	if strings.TrimSpace(wf.Name) != name {
		return nil, fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}

	existing, err := s.Get(ctx, name)
	exists := err == nil
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if exists && !opts.Overwrite {
		return nil, ErrAlreadyExists
	}

	if err := validateWorkflowBundleVersions(name, bundle.Versions); err != nil {
		return nil, err
	}
	if _, err := s.store.Put(name, raw); err != nil {
		return nil, err
	}

	saveNote := strings.TrimSpace(opts.SaveNote)
	if saveNote == "" {
		saveNote = strings.TrimSpace(bundle.SaveNote)
	}
	if saveNote == "" {
		saveNote = "imported workflow bundle"
	}
	labels := copyStringMap(bundle.Labels)
	if labels == nil && existing != nil {
		labels = copyStringMap(existing.Labels)
	}
	if err := s.upsertMeta(name, labels, saveNote, WorkflowStatusDraft, time.Time{}, !exists); err != nil {
		return nil, err
	}

	if err := s.replaceWorkflowHistoryFromBundle(name, bundle.Versions); err != nil {
		return nil, err
	}
	record, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(bundle.Versions) == 0 {
		if err := s.saveVersionSnapshot(record, "import"); err != nil {
			return nil, err
		}
	}
	return record, nil
}

func (s *WorkflowService) Delete(_ context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	path := filepath.Join(s.store.Dir, name+".yaml")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	metas, err := s.loadMetaNoLock()
	if err != nil {
		return err
	}
	delete(metas, name)
	return s.saveMetaNoLock(metas)
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

func (s *WorkflowService) saveCurrentVersion(ctx context.Context, name, reason string) error {
	record, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	return s.saveVersionSnapshot(record, reason)
}

func (s *WorkflowService) saveVersionSnapshot(record *WorkflowRecord, reason string) error {
	if record == nil {
		return fmt.Errorf("%w: empty workflow record", ErrInvalid)
	}
	now := time.Now().UTC()
	version := workflowVersionFile{
		ID:          workflowVersionID(now),
		Name:        record.Name,
		Description: record.Description,
		Version:     record.Version,
		Status:      normalizeWorkflowStatus(record.Status),
		SaveNote:    record.SaveNote,
		Reason:      strings.TrimSpace(reason),
		Checksum:    workflowYAMLChecksum(record.RawYAML),
		YAML:        string(record.RawYAML),
		PublishedAt: record.PublishedAt,
		CreatedAt:   now,
	}
	return s.writeVersionFile(record.Name, version)
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

func (s *WorkflowService) writeVersionFile(name string, version workflowVersionFile) error {
	version.ID = strings.TrimSpace(version.ID)
	if version.ID == "" {
		version.ID = workflowVersionID(time.Now().UTC())
	}
	version.Name = strings.TrimSpace(version.Name)
	if version.Name == "" {
		version.Name = strings.TrimSpace(name)
	}
	version.Status = normalizeWorkflowStatus(version.Status)
	if strings.TrimSpace(version.Checksum) == "" {
		version.Checksum = workflowYAMLChecksum([]byte(version.YAML))
	}
	if version.CreatedAt.IsZero() {
		version.CreatedAt = time.Now().UTC()
	}
	dir := s.historyDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(version, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "workflow-version-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.versionFilePath(name, version.ID))
}

func (s *WorkflowService) historyDir(name string) string {
	return filepath.Join(s.historyPath, url.PathEscape(strings.TrimSpace(name)))
}

func (s *WorkflowService) versionFilePath(name, versionID string) string {
	return filepath.Join(s.historyDir(name), url.PathEscape(strings.TrimSpace(versionID))+".json")
}

func workflowVersionID(now time.Time) string {
	return fmt.Sprintf("v%d", now.UnixNano())
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

func (s *WorkflowService) upsertMeta(name string, labels map[string]string, saveNote, status string, publishedAt time.Time, isCreate bool) error {
	return s.upsertMetaWithHashes(name, labels, saveNote, status, publishedAt, isCreate, workflowMetaHashFields{})
}

func (s *WorkflowService) upsertMetaWithHashes(name string, labels map[string]string, saveNote, status string, publishedAt time.Time, isCreate bool, hashes workflowMetaHashFields) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	metas, err := s.loadMetaNoLock()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	meta, ok := metas[name]
	if !ok {
		meta = workflowMeta{Name: name, CreatedAt: now}
	}
	if isCreate {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	meta.Labels = copyStringMap(labels)
	meta.SaveNote = saveNote
	meta.Status = normalizeWorkflowStatus(status)
	meta.PublishedAt = publishedAt
	meta.ValidatedGraphHash = strings.TrimSpace(hashes.ValidatedGraphHash)
	meta.ValidatedLayoutHash = strings.TrimSpace(hashes.ValidatedLayoutHash)
	meta.ValidatedAt = hashes.ValidatedAt
	meta.ValidatedBy = strings.TrimSpace(hashes.ValidatedBy)
	meta.DryRunGraphHash = strings.TrimSpace(hashes.DryRunGraphHash)
	meta.DryRunLayoutHash = strings.TrimSpace(hashes.DryRunLayoutHash)
	meta.DryRunAt = hashes.DryRunAt
	meta.DryRunBy = strings.TrimSpace(hashes.DryRunBy)
	meta.PublishedGraphHash = strings.TrimSpace(hashes.PublishedGraphHash)
	metas[name] = meta
	return s.saveMetaNoLock(metas)
}

func (s *WorkflowService) loadMeta() (map[string]workflowMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

func (s *WorkflowService) saveMetaNoLock(data map[string]workflowMeta) error {
	if err := os.MkdirAll(filepath.Dir(s.metaPath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.metaPath), "wf-meta-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.metaPath)
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

func (s *WorkflowService) replaceWorkflowHistoryFromBundle(name string, versions []WorkflowBundleVersion) error {
	dir := s.historyDir(name)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, version := range versions {
		file := workflowVersionFile{
			ID:          version.ID,
			Name:        version.Name,
			Description: version.Description,
			Version:     version.Version,
			Status:      version.Status,
			SaveNote:    version.SaveNote,
			Reason:      version.Reason,
			Checksum:    version.Checksum,
			YAML:        version.YAML,
			PublishedAt: version.PublishedAt,
			CreatedAt:   version.CreatedAt,
		}
		if err := s.writeVersionFile(name, file); err != nil {
			return err
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
