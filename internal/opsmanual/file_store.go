package opsmanual

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type FileStore struct {
	mu    sync.Mutex
	path  string
	mem   *MemoryStore
	state fileStoreState
}

type fileStoreState struct {
	Manuals    []OpsManual       `json:"manuals"`
	Candidates []ManualCandidate `json:"candidates"`
	RunRecords []RunRecord       `json:"run_records"`
}

var _ ManualRepository = (*FileStore)(nil)
var _ CandidateRepository = (*FileStore)(nil)
var _ RunRecordRepository = (*FileStore)(nil)

func NewFileStore(path string) (*FileStore, error) {
	if path == "" {
		path = filepath.Join(".data", "ops-manuals", "library.json")
	}
	store := &FileStore{path: path, mem: NewMemoryStore()}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileStore) ListManuals(req ListManualsRequest) ([]OpsManual, error) {
	return s.mem.ListManuals(req)
}

func (s *FileStore) GetManual(id string) (OpsManual, error) {
	return s.mem.GetManual(id)
}

func (s *FileStore) SaveManual(manual OpsManual) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.SaveManual(manual); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) DeleteManual(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.DeleteManual(id); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) GetCandidate(id string) (ManualCandidate, error) {
	return s.mem.GetCandidate(id)
}

func (s *FileStore) ListCandidates() ([]ManualCandidate, error) {
	return s.mem.ListCandidates()
}

func (s *FileStore) SaveCandidate(candidate ManualCandidate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.SaveCandidate(candidate); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) DeleteCandidate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.DeleteCandidate(id); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) ListRunRecords(req ListRunRecordsRequest) ([]RunRecord, error) {
	return s.mem.ListRunRecords(req)
}

func (s *FileStore) SaveRunRecord(record RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mem.SaveRunRecord(record); err != nil {
		return err
	}
	return s.persist()
}

func (s *FileStore) load() error {
	raw, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state fileStoreState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	for _, manual := range state.Manuals {
		if err := s.mem.SaveManual(manual); err != nil {
			return err
		}
	}
	for _, candidate := range state.Candidates {
		if err := s.mem.SaveCandidate(candidate); err != nil {
			return err
		}
	}
	for _, record := range state.RunRecords {
		if err := s.mem.SaveRunRecord(record); err != nil {
			return err
		}
	}
	return nil
}

func (s *FileStore) persist() error {
	manuals, err := s.mem.ListManuals(ListManualsRequest{})
	if err != nil {
		return err
	}
	candidates, err := s.mem.ListCandidates()
	if err != nil {
		return err
	}
	records, err := s.mem.ListRunRecords(ListRunRecordsRequest{Limit: 1000000})
	if err != nil {
		return err
	}
	state := fileStoreState{Manuals: manuals, Candidates: candidates, RunRecords: records}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
