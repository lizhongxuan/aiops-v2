package agentstore

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type FileStore struct {
	Path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore {
	return &FileStore{Path: path}
}

type filePayload struct {
	UpdatedAt time.Time              `json:"updated_at"`
	Agents    map[string]AgentRecord `json:"agents"`
}

func (s *FileStore) List(_ context.Context, filter Filter) ([]AgentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return nil, err
	}
	status := strings.ToLower(strings.TrimSpace(filter.Status))
	tag := strings.TrimSpace(filter.Tag)
	out := make([]AgentRecord, 0, len(data.Agents))
	for _, item := range data.Agents {
		if status != "" && strings.ToLower(item.Status) != status {
			continue
		}
		if tag != "" && !contains(item.Tags, tag) {
			continue
		}
		item.Token = strings.TrimSpace(item.Token)
		out = append(out, cloneAgent(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (s *FileStore) Get(_ context.Context, id string) (AgentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return AgentRecord{}, err
	}
	id = strings.TrimSpace(id)
	item, ok := data.Agents[id]
	if !ok {
		return AgentRecord{}, ErrNotFound
	}
	return cloneAgent(item), nil
}

func (s *FileStore) Create(_ context.Context, record AgentRecord) (AgentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return AgentRecord{}, err
	}
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		return AgentRecord{}, fmt.Errorf("agent id is required")
	}
	if _, ok := data.Agents[record.ID]; ok {
		return AgentRecord{}, ErrExists
	}
	record.Status = normalizeStatus(record.Status)
	if record.Status == "" {
		record.Status = StatusOnline
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	record.LastBeatAt = now
	record.Token = strings.TrimSpace(record.Token)
	data.Agents[record.ID] = cloneAgent(record)
	if err := s.saveNoLock(data); err != nil {
		return AgentRecord{}, err
	}
	return cloneAgent(record), nil
}

func (s *FileStore) Update(_ context.Context, id string, record AgentRecord) (AgentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return AgentRecord{}, err
	}
	id = strings.TrimSpace(id)
	existing, ok := data.Agents[id]
	if !ok {
		return AgentRecord{}, ErrNotFound
	}
	if name := strings.TrimSpace(record.Name); name != "" {
		existing.Name = name
	}
	if addr := strings.TrimSpace(record.Address); addr != "" {
		existing.Address = addr
	}
	if token := strings.TrimSpace(record.Token); token != "" {
		existing.Token = token
	}
	if record.Tags != nil {
		existing.Tags = append([]string{}, record.Tags...)
	}
	if record.Capabilities != nil {
		existing.Capabilities = append([]string{}, record.Capabilities...)
	}
	if status := normalizeStatus(record.Status); status != "" {
		existing.Status = status
	}
	existing.LastError = strings.TrimSpace(record.LastError)
	existing.UpdatedAt = time.Now().UTC()
	data.Agents[id] = cloneAgent(existing)
	if err := s.saveNoLock(data); err != nil {
		return AgentRecord{}, err
	}
	return cloneAgent(existing), nil
}

func (s *FileStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if _, ok := data.Agents[id]; !ok {
		return ErrNotFound
	}
	delete(data.Agents, id)
	return s.saveNoLock(data)
}

func (s *FileStore) Heartbeat(_ context.Context, id string, beat Heartbeat) (AgentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.loadNoLock()
	if err != nil {
		return AgentRecord{}, err
	}
	id = strings.TrimSpace(id)
	existing, ok := data.Agents[id]
	if !ok {
		return AgentRecord{}, ErrNotFound
	}
	if status := normalizeStatus(beat.Status); status != "" {
		existing.Status = status
	} else if existing.Status == "" {
		existing.Status = StatusOnline
	}
	existing.LastLoad = beat.Load
	existing.RunningTasks = beat.RunningTasks
	existing.LastError = strings.TrimSpace(beat.Error)
	existing.LastBeatAt = time.Now().UTC()
	existing.UpdatedAt = existing.LastBeatAt
	data.Agents[id] = cloneAgent(existing)
	if err := s.saveNoLock(data); err != nil {
		return AgentRecord{}, err
	}
	return cloneAgent(existing), nil
}

func (s *FileStore) loadNoLock() (filePayload, error) {
	if strings.TrimSpace(s.Path) == "" {
		return filePayload{}, fmt.Errorf("agent state file path is empty")
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return filePayload{
				Agents: map[string]AgentRecord{},
			}, nil
		}
		return filePayload{}, err
	}
	var data filePayload
	if err := json.Unmarshal(raw, &data); err != nil {
		return filePayload{}, err
	}
	if data.Agents == nil {
		data.Agents = map[string]AgentRecord{}
	}
	for id, item := range data.Agents {
		token, err := decryptToken(item.Token)
		if err != nil {
			return filePayload{}, err
		}
		item.Token = token
		data.Agents[id] = item
	}
	return data, nil
}

func (s *FileStore) saveNoLock(data filePayload) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	data.UpdatedAt = time.Now().UTC()
	for id, item := range data.Agents {
		token, err := encryptToken(item.Token)
		if err != nil {
			return err
		}
		item.Token = token
		data.Agents[id] = item
	}
	payload, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), "agents-*.json")
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
	return os.Rename(tmpPath, s.Path)
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusOnline:
		return StatusOnline
	case StatusOffline:
		return StatusOffline
	case StatusDegraded:
		return StatusDegraded
	default:
		return ""
	}
}

func contains(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func cloneAgent(input AgentRecord) AgentRecord {
	out := input
	out.Tags = append([]string{}, input.Tags...)
	out.Capabilities = append([]string{}, input.Capabilities...)
	return out
}

func encryptToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if strings.HasPrefix(token, "enc:") {
		return token, nil
	}
	block, err := aes.NewCipher(secretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(token), nil)
	return "enc:" + base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if !strings.HasPrefix(token, "enc:") {
		return token, nil
	}
	cipherText, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(token, "enc:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(secretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(cipherText) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted token")
	}
	nonce := cipherText[:gcm.NonceSize()]
	payload := cipherText[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func secretKey() []byte {
	secret := strings.TrimSpace(os.Getenv("RUNNER_SERVER_SECRET"))
	if secret == "" {
		secret = "runner-server-default-secret-change-me"
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
