package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"runner/logging"
	"go.uber.org/zap"
)

type StateFile struct {
	UpdatedAt time.Time  `json:"updated_at"`
	Runs      []RunState `json:"runs"`
}

type Store interface {
	Load() (StateFile, error)
	Save(state StateFile) error
}

type FileStore struct {
	Path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore {
	return &FileStore{Path: path}
}

func (s *FileStore) Load() (StateFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadNoLock()
}

func (s *FileStore) loadNoLock() (StateFile, error) {
	file, err := os.Open(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			logging.L().Debug("state load missing", zap.String("path", s.Path))
			return StateFile{}, nil
		}
		logging.L().Debug("state load failed", zap.String("path", s.Path), zap.Error(err))
		return StateFile{}, err
	}
	defer file.Close()

	dec := json.NewDecoder(file)
	var data StateFile
	if err := dec.Decode(&data); err != nil {
		logging.L().Debug("state decode failed", zap.String("path", s.Path), zap.Error(err))
		return StateFile{}, err
	}
	logging.L().Debug("state loaded", zap.String("path", s.Path), zap.Int("runs", len(data.Runs)))
	return data, nil
}

func (s *FileStore) Save(state StateFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveNoLock(state)
}

func (s *FileStore) saveNoLock(state StateFile) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logging.L().Debug("state mkdir failed", zap.String("dir", dir), zap.Error(err))
		return err
	}

	state.UpdatedAt = time.Now().UTC()
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logging.L().Debug("state encode failed", zap.String("path", s.Path), zap.Error(err))
		return err
	}

	tmp, err := os.CreateTemp(dir, "state-*.json")
	if err != nil {
		logging.L().Debug("state tempfile failed", zap.String("dir", dir), zap.Error(err))
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		logging.L().Debug("state write failed", zap.String("tmp", tmpPath), zap.Error(err))
		return err
	}
	if err := tmp.Close(); err != nil {
		logging.L().Debug("state close failed", zap.String("tmp", tmpPath), zap.Error(err))
		return err
	}

	if err := os.Rename(tmpPath, s.Path); err != nil {
		logging.L().Debug("state rename failed", zap.String("tmp", tmpPath), zap.String("path", s.Path), zap.Error(err))
		return fmt.Errorf("persist state: %w", err)
	}

	logging.L().Debug("state saved", zap.String("path", s.Path), zap.Int("runs", len(state.Runs)))
	return nil
}

var _ RunStateStore = (*FileStore)(nil)

func (s *FileStore) CreateRun(ctx context.Context, run RunState) error {
	_ = ctx
	if err := ValidateRunID(run.RunID); err != nil {
		return err
	}
	if strings.TrimSpace(run.Status) == "" {
		run.Status = RunStatusQueued
	}
	if err := ValidateRunStatus(run.Status); err != nil {
		return err
	}
	now := time.Now().UTC()
	if run.StartedAt.IsZero() {
		run.StartedAt = now
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = now
	}
	if run.Version < 1 {
		run.Version = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadNoLock()
	if err != nil {
		return err
	}
	if _, idx := findRun(data.Runs, run.RunID); idx >= 0 {
		return ErrRunExists
	}
	data.Runs = append(data.Runs, CloneRunState(run))
	sortRuns(data.Runs)
	return s.saveNoLock(data)
}

func (s *FileStore) UpdateRun(ctx context.Context, run RunState) error {
	_ = ctx
	if err := ValidateRunID(run.RunID); err != nil {
		return err
	}
	if err := ValidateRunStatus(run.Status); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadNoLock()
	if err != nil {
		return err
	}
	prev, idx := findRun(data.Runs, run.RunID)
	if idx < 0 {
		return ErrRunNotFound
	}
	if err := ValidateRunTransition(prev.Status, run.Status); err != nil {
		return err
	}

	now := time.Now().UTC()
	if run.StartedAt.IsZero() {
		run.StartedAt = prev.StartedAt
	}
	if IsTerminalRunStatus(run.Status) && run.FinishedAt.IsZero() {
		run.FinishedAt = now
	}
	run.UpdatedAt = now
	if run.Version <= prev.Version {
		run.Version = prev.Version + 1
	}
	data.Runs[idx] = CloneRunState(run)
	sortRuns(data.Runs)
	return s.saveNoLock(data)
}

func (s *FileStore) GetRun(ctx context.Context, runID string) (RunState, error) {
	_ = ctx
	if err := ValidateRunID(runID); err != nil {
		return RunState{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadNoLock()
	if err != nil {
		return RunState{}, err
	}
	run, idx := findRun(data.Runs, runID)
	if idx < 0 {
		return RunState{}, ErrRunNotFound
	}
	return CloneRunState(run), nil
}

func (s *FileStore) ListRuns(ctx context.Context, filter ListFilter) ([]RunState, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadNoLock()
	if err != nil {
		return nil, err
	}
	runs := make([]RunState, 0, len(data.Runs))
	statusFilter := strings.TrimSpace(strings.ToLower(filter.Status))
	for _, run := range data.Runs {
		if statusFilter != "" && strings.ToLower(strings.TrimSpace(run.Status)) != statusFilter {
			continue
		}
		runs = append(runs, CloneRunState(run))
	}
	sortRuns(runs)
	if filter.Limit > 0 && len(runs) > filter.Limit {
		runs = runs[:filter.Limit]
	}
	return runs, nil
}

func (s *FileStore) MarkInterruptedRunning(ctx context.Context, reason string) (int, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadNoLock()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	updated := 0
	for i := range data.Runs {
		if strings.EqualFold(data.Runs[i].Status, RunStatusRunning) {
			if err := ValidateRunTransition(data.Runs[i].Status, RunStatusInterrupted); err != nil {
				return updated, err
			}
			data.Runs[i].Status = RunStatusInterrupted
			data.Runs[i].InterruptedReason = strings.TrimSpace(reason)
			if data.Runs[i].InterruptedReason != "" {
				data.Runs[i].Message = data.Runs[i].InterruptedReason
			}
			data.Runs[i].FinishedAt = now
			data.Runs[i].UpdatedAt = now
			data.Runs[i].Version++
			updated++
		}
	}
	if updated == 0 {
		return 0, nil
	}
	sortRuns(data.Runs)
	if err := s.saveNoLock(data); err != nil {
		return 0, err
	}
	return updated, nil
}

func findRun(runs []RunState, runID string) (RunState, int) {
	for i := range runs {
		if runs[i].RunID == runID {
			return runs[i], i
		}
	}
	return RunState{}, -1
}

func sortRuns(runs []RunState) {
	sort.Slice(runs, func(i, j int) bool {
		left := runs[i].StartedAt
		right := runs[j].StartedAt
		if left.IsZero() && !runs[i].UpdatedAt.IsZero() {
			left = runs[i].UpdatedAt
		}
		if right.IsZero() && !runs[j].UpdatedAt.IsZero() {
			right = runs[j].UpdatedAt
		}
		return left.After(right)
	})
}

func IsNotFound(err error) bool {
	return errors.Is(err, ErrRunNotFound)
}
