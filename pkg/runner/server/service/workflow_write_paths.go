package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"runner/workflow"
	"runner/workflow/edit"
	"runner/workflow/visual"
	"runner/workflowstore"
)

type workflowHistoryPayload struct {
	Versions []workflowVersionFile `json:"versions"`
}

func (s *WorkflowService) WorkflowCommitStore() *WorkflowCommitStore {
	return s.commitStore
}

func (s *WorkflowService) SetWorkflowTenantID(tenantID string) {
	if s != nil {
		s.tenantID = strings.TrimSpace(tenantID)
	}
}

func (s *WorkflowService) workflowKey(name string) edit.WorkflowKey {
	return edit.WorkflowKey{TenantID: strings.TrimSpace(s.tenantID), WorkflowID: strings.TrimSpace(name)}
}

func (s *WorkflowService) visualWriter() *VisualWorkflowService {
	return NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: s, CommitStore: s.commitStore, TenantID: s.tenantID})
}

func (s *WorkflowService) currentWorkflowSnapshot(ctx context.Context, name string) (workflowstore.TransactionSnapshot, bool, error) {
	if s == nil || s.commitStore == nil {
		return workflowstore.TransactionSnapshot{}, false, nil
	}
	snapshot, err := s.commitStore.Snapshot(ctx, s.workflowKey(name))
	if errors.Is(err, ErrNotFound) {
		return workflowstore.TransactionSnapshot{}, false, nil
	}
	return snapshot, err == nil, err
}

func (s *WorkflowService) getWorkflow(ctx context.Context, name string) (*WorkflowRecord, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if snapshot, found, err := s.currentWorkflowSnapshot(ctx, name); err != nil {
		return nil, err
	} else if found {
		return workflowRecordFromSnapshot(snapshot)
	}
	return s.getLegacyWorkflow(ctx, name)
}

func workflowRecordFromSnapshot(snapshot workflowstore.TransactionSnapshot) (*WorkflowRecord, error) {
	wf, err := workflow.Load(snapshot.GraphPayload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if wf.Name != snapshot.Key.WorkflowID {
		return nil, fmt.Errorf("%w: workflow snapshot key mismatch", ErrInvalid)
	}
	meta := workflowMeta{Name: wf.Name, Status: WorkflowStatusDraft}
	if err := json.Unmarshal(snapshot.Meta.Payload, &meta); err != nil {
		return nil, fmt.Errorf("decode workflow transaction metadata: %w", err)
	}
	if meta.Deleted {
		return nil, ErrNotFound
	}
	record := &WorkflowRecord{Name: wf.Name, Description: wf.Description, Version: wf.Version, RawYAML: append([]byte(nil), snapshot.GraphPayload...)}
	applyWorkflowMeta(record, meta)
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = snapshot.CommittedAt
	}
	return record, nil
}

func applyWorkflowMeta(record *WorkflowRecord, meta workflowMeta) {
	record.Labels, record.SaveNote, record.Status = copyStringMap(meta.Labels), meta.SaveNote, normalizeWorkflowStatus(meta.Status)
	record.ValidatedGraphHash, record.ValidatedLayoutHash, record.ValidatedAt, record.ValidatedBy = strings.TrimSpace(meta.ValidatedGraphHash), strings.TrimSpace(meta.ValidatedLayoutHash), meta.ValidatedAt, strings.TrimSpace(meta.ValidatedBy)
	record.DryRunGraphHash, record.DryRunLayoutHash, record.DryRunAt, record.DryRunBy = strings.TrimSpace(meta.DryRunGraphHash), strings.TrimSpace(meta.DryRunLayoutHash), meta.DryRunAt, strings.TrimSpace(meta.DryRunBy)
	record.PublishedGraphHash, record.PublishedAt, record.CreatedAt, record.UpdatedAt = strings.TrimSpace(meta.PublishedGraphHash), meta.PublishedAt, meta.CreatedAt, meta.UpdatedAt
}

func (s *WorkflowService) listWorkflows(ctx context.Context, labels map[string]string) ([]*WorkflowRecord, error) {
	legacy, err := s.listLegacyWorkflows(ctx, labels)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*WorkflowRecord, len(legacy))
	for _, item := range legacy {
		byName[item.Name] = item
	}
	if s.commitStore != nil {
		snapshots, err := workflowstore.ListCurrentTransactionSnapshots(s.commitStore.root)
		if err != nil {
			return nil, err
		}
		for _, snapshot := range snapshots {
			if snapshot.Key.TenantID != s.tenantID {
				continue
			}
			record, err := workflowRecordFromSnapshot(snapshot)
			if errors.Is(err, ErrNotFound) {
				delete(byName, snapshot.Key.WorkflowID)
				continue
			}
			if err != nil {
				return nil, err
			}
			if matchLabels(record.Labels, labels) {
				byName[record.Name] = record
			} else {
				delete(byName, record.Name)
			}
		}
	}
	out := make([]*WorkflowRecord, 0, len(byName))
	for _, record := range byName {
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *WorkflowService) createWorkflow(ctx context.Context, record *WorkflowRecord, operation string, versions []WorkflowBundleVersion, replaceHistory bool) error {
	_, graph, err := validateWorkflowWriteRecord(record, "")
	if err != nil {
		return err
	}
	_, err = s.visualWriter().CreateGraph(ctx, graph, VisualWorkflowCreateOptions{
		Labels: record.Labels, SaveNote: record.SaveNote, SaveNoteSet: record.SaveNoteSet || strings.TrimSpace(record.SaveNote) != "",
		operation: operation, versions: versions, replaceHistory: replaceHistory, rawYAML: record.RawYAML,
	})
	return err
}

func (s *WorkflowService) updateWorkflow(ctx context.Context, name string, record *WorkflowRecord, operation string, versions []WorkflowBundleVersion, replaceHistory bool) error {
	name, graph, err := validateWorkflowWriteRecord(record, name)
	if err != nil {
		return err
	}
	if err := s.ensureWorkflowVersionMutable(ctx, name); err != nil {
		return err
	}
	current, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	expected := graphResourceVersion(current.RawYAML)
	if snapshot, found, err := s.currentWorkflowSnapshot(ctx, name); err != nil {
		return err
	} else if found {
		expected = snapshot.ResourceVersion
	}
	graph.UI = withGraphResourceVersion(graph.UI, expected)
	_, err = s.visualWriter().SaveGraphWithOptions(ctx, name, graph, VisualWorkflowSaveOptions{
		SaveNote: record.SaveNote, SaveNoteSet: record.SaveNoteSet || strings.TrimSpace(record.SaveNote) != "",
		labels: record.Labels, labelsSet: record.Labels != nil, operation: operation, versions: versions, replaceHistory: replaceHistory, rawYAML: record.RawYAML,
	})
	return err
}

func validateWorkflowWriteRecord(record *WorkflowRecord, requestedName string) (string, visual.Graph, error) {
	if record == nil {
		return "", visual.Graph{}, fmt.Errorf("%w: empty workflow record", ErrInvalid)
	}
	if len(record.RawYAML) == 0 {
		return "", visual.Graph{}, fmt.Errorf("%w: yaml is required", ErrInvalid)
	}
	wf, err := workflow.Load(record.RawYAML)
	if err != nil {
		return "", visual.Graph{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return "", visual.Graph{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	name := strings.TrimSpace(requestedName)
	if name == "" {
		name = strings.TrimSpace(record.Name)
	}
	if name == "" {
		return "", visual.Graph{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if strings.TrimSpace(wf.Name) != name {
		return "", visual.Graph{}, fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	graph, err := visual.ParseYAMLToGraph(record.RawYAML)
	if err != nil {
		return "", visual.Graph{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return name, graph, nil
}

func (s *WorkflowService) commitWorkflowMeta(ctx context.Context, name string, meta workflowMeta, operation string) error {
	record, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	graph, err := visual.ParseYAMLToGraph(record.RawYAML)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	writer := s.visualWriter()
	compiled, err := writer.Compile(ctx, graph)
	if err != nil {
		return err
	}
	clientExpected := graphResourceVersion(record.RawYAML)
	if snapshot, found, err := s.currentWorkflowSnapshot(ctx, name); err != nil {
		return err
	} else if found {
		clientExpected = snapshot.ResourceVersion
	}
	state, err := writer.visualWorkflowWriteState(ctx, s.workflowKey(name), name, clientExpected)
	if err != nil {
		return err
	}
	if err := writer.commitVisualGraph(ctx, s.workflowKey(name), state.expectedVersion, clientExpected, state.previousGraph, meta, compiled, operation, nil, false, record.RawYAML); err != nil {
		if isStaleRevision(err) {
			return fmt.Errorf("%w: workflow graph changed", ErrConflict)
		}
		return err
	}
	return nil
}

func (s *WorkflowService) upsertCommittedMeta(ctx context.Context, name string, labels map[string]string, saveNote, status string, publishedAt time.Time, hashes workflowMetaHashFields) error {
	existing, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	meta := workflowMetaFromRecord(existing)
	meta.Labels, meta.SaveNote, meta.Status, meta.PublishedAt, meta.UpdatedAt = copyStringMap(labels), saveNote, normalizeWorkflowStatus(status), publishedAt, time.Now().UTC()
	meta.ValidatedGraphHash, meta.ValidatedLayoutHash, meta.ValidatedAt, meta.ValidatedBy = strings.TrimSpace(hashes.ValidatedGraphHash), strings.TrimSpace(hashes.ValidatedLayoutHash), hashes.ValidatedAt, strings.TrimSpace(hashes.ValidatedBy)
	meta.DryRunGraphHash, meta.DryRunLayoutHash, meta.DryRunAt, meta.DryRunBy = strings.TrimSpace(hashes.DryRunGraphHash), strings.TrimSpace(hashes.DryRunLayoutHash), hashes.DryRunAt, strings.TrimSpace(hashes.DryRunBy)
	meta.PublishedGraphHash = strings.TrimSpace(hashes.PublishedGraphHash)
	operation := map[string]string{WorkflowStatusValidated: "validate", WorkflowStatusDryRunPassed: "dry_run", WorkflowStatusPublished: "publish"}[meta.Status]
	return s.commitWorkflowMeta(ctx, name, meta, operation)
}

func (s *WorkflowService) rollbackWorkflow(ctx context.Context, name, versionID string, opts WorkflowRollbackOptions) (*WorkflowRecord, error) {
	version, err := s.GetVersion(ctx, name, versionID)
	if err != nil {
		return nil, err
	}
	note := strings.TrimSpace(opts.SaveNote)
	if note == "" {
		note = "rollback to " + version.ID
	}
	if err := s.updateWorkflow(ctx, name, &WorkflowRecord{Name: name, RawYAML: version.RawYAML, SaveNote: note, SaveNoteSet: true}, "rollback:"+version.ID, nil, false); err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

func (s *WorkflowService) importWorkflowBundle(ctx context.Context, bundle *WorkflowBundle, opts WorkflowImportOptions) (*WorkflowRecord, error) {
	if bundle == nil || strings.TrimSpace(bundle.YAML) == "" {
		return nil, fmt.Errorf("%w: empty workflow bundle or yaml", ErrInvalid)
	}
	if bundle.BundleVersion != "" && bundle.BundleVersion != workflowBundleFormatVersion {
		return nil, fmt.Errorf("%w: unsupported workflow bundle version %q", ErrInvalid, bundle.BundleVersion)
	}
	wf, err := workflow.Load([]byte(bundle.YAML))
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
	if name == "" || strings.TrimSpace(wf.Name) != name {
		return nil, fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	if err := validateWorkflowBundleVersions(name, bundle.Versions); err != nil {
		return nil, err
	}
	existing, err := s.Get(ctx, name)
	exists := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if exists && !opts.Overwrite {
		return nil, ErrAlreadyExists
	}
	note := strings.TrimSpace(opts.SaveNote)
	if note == "" {
		note = strings.TrimSpace(bundle.SaveNote)
	}
	if note == "" {
		note = "imported workflow bundle"
	}
	labels := copyStringMap(bundle.Labels)
	if labels == nil && existing != nil {
		labels = copyStringMap(existing.Labels)
	}
	record := &WorkflowRecord{Name: name, RawYAML: []byte(bundle.YAML), Labels: labels, SaveNote: note, SaveNoteSet: true}
	if exists {
		err = s.updateWorkflow(ctx, name, record, "import", bundle.Versions, true)
	} else {
		err = s.createWorkflow(ctx, record, "import", bundle.Versions, true)
	}
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, name)
}

func (s *WorkflowService) deleteWorkflow(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if _, err := s.Get(ctx, name); err != nil {
		return err
	}
	if err := s.ensureWorkflowDeleteAllowed(ctx, name); err != nil {
		return err
	}
	record, _ := s.Get(ctx, name)
	meta := workflowMetaFromRecord(record)
	meta.Deleted, meta.UpdatedAt = true, time.Now().UTC()
	return s.commitWorkflowMeta(ctx, name, meta, "delete")
}

func (s *VisualWorkflowService) nextWorkflowHistory(ctx context.Context, key edit.WorkflowKey, yaml string, meta workflowMeta, operation, transactionID string, imported []WorkflowBundleVersion, replace bool) (workflowHistoryPayload, error) {
	history := workflowHistoryPayload{Versions: []workflowVersionFile{}}
	if snapshot, err := s.commitStore.Snapshot(ctx, key); err == nil {
		_ = json.Unmarshal(snapshot.History.Payload, &history)
	} else if !errors.Is(err, ErrNotFound) {
		return history, err
	} else if s.workflowSvc != nil {
		legacy, err := s.workflowSvc.listLegacyWorkflowVersions(ctx, key.WorkflowID)
		if err != nil {
			return history, err
		}
		for _, version := range legacy {
			history.Versions = append(history.Versions, versionFileFromRecord(version))
		}
	}
	if replace {
		history.Versions = history.Versions[:0]
		for _, version := range imported {
			history.Versions = append(history.Versions, versionFileFromBundle(key.WorkflowID, version))
		}
	}
	if operation == "validate" || operation == "dry_run" || operation == "delete" || (operation == "import" && len(imported) > 0) {
		return history, nil
	}
	wf, err := workflow.Load([]byte(yaml))
	if err != nil {
		return history, err
	}
	history.Versions = append(history.Versions, workflowVersionFile{ID: transactionID, Name: wf.Name, Description: wf.Description, Version: wf.Version, Status: normalizeWorkflowStatus(meta.Status), SaveNote: meta.SaveNote, Reason: operation, Checksum: workflowYAMLChecksum([]byte(yaml)), YAML: yaml, PublishedAt: meta.PublishedAt, CreatedAt: time.Now().UTC()})
	return history, nil
}

func (s *WorkflowService) listWorkflowVersions(ctx context.Context, name string) ([]*WorkflowVersionRecord, error) {
	snapshot, found, err := s.currentWorkflowSnapshot(ctx, strings.TrimSpace(name))
	if err != nil {
		return nil, err
	}
	if !found {
		return s.listLegacyWorkflowVersions(ctx, name)
	}
	history := workflowHistoryPayload{}
	if err := json.Unmarshal(snapshot.History.Payload, &history); err != nil {
		return nil, err
	}
	out := make([]*WorkflowVersionRecord, 0, len(history.Versions))
	for i := range history.Versions {
		out = append(out, recordFromVersionFile(history.Versions[i]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *WorkflowService) getWorkflowVersion(ctx context.Context, name, versionID string) (*WorkflowVersionRecord, error) {
	versions, err := s.listWorkflowVersions(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if version.ID == strings.TrimSpace(versionID) {
			return version, nil
		}
	}
	return nil, ErrNotFound
}

func versionFileFromRecord(item *WorkflowVersionRecord) workflowVersionFile {
	return workflowVersionFile{ID: item.ID, Name: item.Name, Description: item.Description, Version: item.Version, Status: item.Status, SaveNote: item.SaveNote, Reason: item.Reason, Checksum: item.Checksum, YAML: string(item.RawYAML), PublishedAt: item.PublishedAt, CreatedAt: item.CreatedAt}
}

func versionFileFromBundle(name string, item WorkflowBundleVersion) workflowVersionFile {
	if strings.TrimSpace(item.Name) == "" {
		item.Name = name
	}
	return workflowVersionFile{ID: item.ID, Name: item.Name, Description: item.Description, Version: item.Version, Status: item.Status, SaveNote: item.SaveNote, Reason: item.Reason, Checksum: item.Checksum, YAML: item.YAML, PublishedAt: item.PublishedAt, CreatedAt: item.CreatedAt}
}

func recordFromVersionFile(file workflowVersionFile) *WorkflowVersionRecord {
	return &WorkflowVersionRecord{ID: file.ID, Name: file.Name, Description: file.Description, Version: file.Version, Status: normalizeWorkflowStatus(file.Status), SaveNote: file.SaveNote, Reason: file.Reason, Checksum: file.Checksum, RawYAML: []byte(file.YAML), PublishedAt: file.PublishedAt, CreatedAt: file.CreatedAt}
}
