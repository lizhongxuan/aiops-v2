package eventstore

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"runner/server/events"
)

type Store interface {
	Append(ctx context.Context, evt events.Event) error
	List(ctx context.Context, runID string) ([]events.Event, error)
}

type FileStore struct {
	Dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) Append(_ context.Context, evt events.Event) error {
	if strings.TrimSpace(evt.RunID) == "" {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, evt.RunID+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *FileStore) List(_ context.Context, runID string) ([]events.Event, error) {
	path := filepath.Join(s.Dir, strings.TrimSpace(runID)+".jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []events.Event{}, nil
		}
		return nil, err
	}
	defer file.Close()

	items := make([]events.Event, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt events.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return nil, err
		}
		items = append(items, evt)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func DeriveRunEventDir(runStateFile string) string {
	trimmed := strings.TrimSpace(runStateFile)
	ext := filepath.Ext(trimmed)
	if ext == "" {
		return trimmed + "-events"
	}
	return strings.TrimSuffix(trimmed, ext) + "-events"
}
