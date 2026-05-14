package appui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type fileExperiencePackRepository struct {
	mu   sync.Mutex
	path string
}

type experiencePackRepositorySnapshot struct {
	Packs      []ExperiencePack                       `json:"packs"`
	Candidates []ExperiencePackCandidate              `json:"candidates"`
	Reuse      map[string][]ExperiencePackReuseRecord `json:"reuse,omitempty"`
}

func NewFileExperiencePackRepository(path string) ExperiencePackRepository {
	return &fileExperiencePackRepository{path: strings.TrimSpace(path)}
}

func (r *fileExperiencePackRepository) ListExperiencePacks(req ListExperiencePacksRequest) (ExperiencePackLibraryList, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return ExperiencePackLibraryList{}, err
	}
	items := make([]ExperiencePack, 0, len(snapshot.Packs))
	for _, pack := range snapshot.Packs {
		pack = normalizeExperiencePack(pack)
		if req.Status != "" && pack.Status != req.Status {
			continue
		}
		if req.Category != "" && pack.Category != req.Category {
			continue
		}
		if req.UsageShape != "" && pack.UsageShape != req.UsageShape {
			continue
		}
		if req.Middleware != "" && !strings.Contains(strings.ToLower(pack.Middleware+" "+strings.Join(pack.Tags, " ")), strings.ToLower(req.Middleware)) {
			continue
		}
		if req.HasRunnerBinding == "true" && !packHasExecutableRunnerBinding(pack) {
			continue
		}
		items = append(items, cloneExperiencePack(pack))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return ExperiencePackLibraryList{Items: items, Total: len(items)}, nil
}

func (r *fileExperiencePackRepository) ListExperiencePackCandidates(req ListExperiencePackCandidatesRequest) (ExperiencePackCandidateList, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return ExperiencePackCandidateList{}, err
	}
	items := make([]ExperiencePackCandidate, 0, len(snapshot.Candidates))
	for _, candidate := range snapshot.Candidates {
		items = append(items, cloneExperiencePackCandidate(candidate))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return ExperiencePackCandidateList{Items: items, Total: len(items)}, nil
}

func (r *fileExperiencePackRepository) SaveExperiencePackCandidate(candidate ExperiencePackCandidate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return err
	}
	candidate = cloneExperiencePackCandidate(candidate)
	candidate.ExperiencePack = nil
	replaced := false
	for i := range snapshot.Candidates {
		if snapshot.Candidates[i].ID == candidate.ID || snapshot.Candidates[i].CandidateID == candidate.CandidateID {
			snapshot.Candidates[i] = candidate
			replaced = true
			break
		}
	}
	if !replaced {
		snapshot.Candidates = append(snapshot.Candidates, candidate)
	}
	return r.saveLocked(snapshot)
}

func (r *fileExperiencePackRepository) GetExperiencePackCandidate(candidateID string) (ExperiencePackCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return ExperiencePackCandidate{}, err
	}
	target := strings.TrimSpace(candidateID)
	for _, candidate := range snapshot.Candidates {
		if candidate.ID == target || candidate.CandidateID == target || candidate.PackID == target {
			return cloneExperiencePackCandidate(candidate), nil
		}
	}
	return ExperiencePackCandidate{}, ErrExperiencePackCandidateNotFound
}

func (r *fileExperiencePackRepository) GetExperiencePack(packID string) (ExperiencePack, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return ExperiencePack{}, err
	}
	target := strings.TrimSpace(packID)
	for _, pack := range snapshot.Packs {
		if pack.ID == target || pack.PackID == target {
			return cloneExperiencePack(pack), nil
		}
	}
	return ExperiencePack{}, ErrExperiencePackNotFound
}

func (r *fileExperiencePackRepository) SaveExperiencePack(pack ExperiencePack) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return err
	}
	pack = normalizeExperiencePack(pack)
	replaced := false
	for i := range snapshot.Packs {
		if snapshot.Packs[i].ID == pack.ID || snapshot.Packs[i].PackID == pack.PackID {
			snapshot.Packs[i] = pack
			replaced = true
			break
		}
	}
	if !replaced {
		snapshot.Packs = append(snapshot.Packs, pack)
	}
	return r.saveLocked(snapshot)
}

func (r *fileExperiencePackRepository) ListExperiencePackReuseRecords(packID string, req ListExperiencePackReuseRecordsRequest) (ExperiencePackReuseRecordList, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot, err := r.loadLocked()
	if err != nil {
		return ExperiencePackReuseRecordList{}, err
	}
	items := append([]ExperiencePackReuseRecord(nil), snapshot.Reuse[strings.TrimSpace(packID)]...)
	if req.Limit > 0 && len(items) > req.Limit {
		items = items[:req.Limit]
	}
	return ExperiencePackReuseRecordList{Items: items, Total: len(items)}, nil
}

func (r *fileExperiencePackRepository) loadLocked() (experiencePackRepositorySnapshot, error) {
	path := r.normalizedPath()
	if path == "" {
		return experiencePackRepositorySnapshot{Reuse: map[string][]ExperiencePackReuseRecord{}}, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return experiencePackRepositorySnapshot{Reuse: map[string][]ExperiencePackReuseRecord{}}, nil
	}
	if err != nil {
		return experiencePackRepositorySnapshot{}, err
	}
	var snapshot experiencePackRepositorySnapshot
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return experiencePackRepositorySnapshot{}, err
		}
	}
	if snapshot.Reuse == nil {
		snapshot.Reuse = map[string][]ExperiencePackReuseRecord{}
	}
	return snapshot, nil
}

func (r *fileExperiencePackRepository) saveLocked(snapshot experiencePackRepositorySnapshot) error {
	path := r.normalizedPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (r *fileExperiencePackRepository) normalizedPath() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.path)
}
