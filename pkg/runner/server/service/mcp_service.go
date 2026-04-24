package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"runner/server/store/mcpstore"
)

type McpService struct {
	store      mcpstore.Store
	httpClient *http.Client
}

func NewMcpService(store mcpstore.Store) *McpService {
	return &McpService{
		store:      store,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *McpService) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	s.httpClient = client
}

func (s *McpService) List(ctx context.Context) ([]*mcpstore.ServerRecord, error) {
	items, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*mcpstore.ServerRecord, 0, len(items))
	for i := range items {
		item := items[i]
		out = append(out, &item)
	}
	return out, nil
}

func (s *McpService) Get(ctx context.Context, id string) (*mcpstore.ServerRecord, error) {
	item, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, mcpstore.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (s *McpService) Create(ctx context.Context, record *mcpstore.ServerRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty mcp record", ErrInvalid)
	}
	normalized, err := s.normalizeRecord(mcpstore.ServerRecord{}, *record, true)
	if err != nil {
		return err
	}
	if _, err := s.store.Create(ctx, normalized); err != nil {
		if errors.Is(err, mcpstore.ErrExists) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (s *McpService) Update(ctx context.Context, id string, record *mcpstore.ServerRecord) error {
	if record == nil {
		return fmt.Errorf("%w: empty mcp record", ErrInvalid)
	}
	current, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, mcpstore.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	normalized, err := s.normalizeRecord(current, *record, false)
	if err != nil {
		return err
	}
	if _, err := s.store.Update(ctx, current.ID, normalized); err != nil {
		if errors.Is(err, mcpstore.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *McpService) Delete(ctx context.Context, id string) error {
	if err := s.store.Delete(ctx, strings.TrimSpace(id)); err != nil {
		if errors.Is(err, mcpstore.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *McpService) Toggle(ctx context.Context, id string) (*mcpstore.ServerRecord, error) {
	record, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, mcpstore.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if record.Status == mcpstore.StatusRunning {
		record.Status = mcpstore.StatusStopped
		record.LastError = ""
		updated, err := s.store.Update(ctx, record.ID, record)
		if err != nil {
			return nil, err
		}
		return &updated, nil
	}

	record.Status = mcpstore.StatusRunning
	record.LastError = ""
	switch record.Type {
	case mcpstore.TypeHTTP:
		if err := s.probeHTTP(ctx, record.URL); err != nil {
			record.Status = mcpstore.StatusStopped
			record.LastError = err.Error()
		}
		tools, discoverErr := s.discoverHTTPTools(ctx, record.URL)
		if discoverErr == nil && len(tools) > 0 {
			record.Tools = tools
		}
	case mcpstore.TypeStdio:
		if err := s.probeCommand(record.Command); err != nil {
			record.Status = mcpstore.StatusStopped
			record.LastError = err.Error()
		}
	default:
		return nil, fmt.Errorf("%w: unsupported mcp type %q", ErrInvalid, record.Type)
	}

	updated, err := s.store.Update(ctx, record.ID, record)
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *McpService) ListTools(ctx context.Context, id string) ([]mcpstore.ToolRecord, error) {
	record, err := s.store.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, mcpstore.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if record.Type == mcpstore.TypeHTTP && record.Status == mcpstore.StatusRunning {
		tools, discoverErr := s.discoverHTTPTools(ctx, record.URL)
		if discoverErr == nil && !reflect.DeepEqual(record.Tools, tools) {
			record.Tools = tools
			if updated, updateErr := s.store.Update(ctx, record.ID, record); updateErr == nil {
				record = updated
			}
		}
	}

	out := make([]mcpstore.ToolRecord, 0, len(record.Tools))
	for _, item := range record.Tools {
		out = append(out, item)
	}
	return out, nil
}

func (s *McpService) normalizeRecord(current, incoming mcpstore.ServerRecord, creating bool) (mcpstore.ServerRecord, error) {
	record := current
	record.ID = strings.TrimSpace(incoming.ID)
	if record.ID == "" {
		record.ID = slugifyID(incoming.Name)
	}
	record.Name = strings.TrimSpace(incoming.Name)
	record.Type = strings.TrimSpace(incoming.Type)
	record.Command = strings.TrimSpace(incoming.Command)
	record.URL = strings.TrimSpace(incoming.URL)
	record.EnvVars = normalizeEnvVars(incoming.EnvVars)
	record.LastError = strings.TrimSpace(incoming.LastError)
	if len(incoming.Tools) > 0 {
		record.Tools = incoming.Tools
	}
	if strings.TrimSpace(incoming.Status) != "" {
		record.Status = strings.TrimSpace(incoming.Status)
	}

	if record.ID == "" {
		return mcpstore.ServerRecord{}, fmt.Errorf("%w: id is required", ErrInvalid)
	}
	if record.Name == "" {
		return mcpstore.ServerRecord{}, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	switch record.Type {
	case mcpstore.TypeHTTP:
		if record.URL == "" {
			return mcpstore.ServerRecord{}, fmt.Errorf("%w: url is required for http mcp", ErrInvalid)
		}
		if _, err := url.ParseRequestURI(record.URL); err != nil {
			return mcpstore.ServerRecord{}, fmt.Errorf("%w: invalid url", ErrInvalid)
		}
		record.Command = ""
	case mcpstore.TypeStdio:
		if record.Command == "" {
			return mcpstore.ServerRecord{}, fmt.Errorf("%w: command is required for stdio mcp", ErrInvalid)
		}
		record.URL = ""
	default:
		return mcpstore.ServerRecord{}, fmt.Errorf("%w: type must be stdio or http", ErrInvalid)
	}

	if record.Status == "" {
		record.Status = mcpstore.StatusStopped
	}
	if !creating && len(incoming.Tools) == 0 && len(current.Tools) > 0 {
		record.Tools = current.Tools
	}
	return record, nil
}

func normalizeEnvVars(env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		out[trimmed] = value
	}
	return out
}

func slugifyID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}

func (s *McpService) probeHTTP(ctx context.Context, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("probe failed: %s", resp.Status)
	}
	return nil
}

func (s *McpService) probeCommand(command string) error {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return fmt.Errorf("%w: empty command", ErrInvalid)
	}
	bin := parts[0]
	if strings.Contains(bin, "/") {
		if _, err := os.Stat(bin); err != nil {
			return err
		}
		return nil
	}
	_, err := exec.LookPath(bin)
	return err
}

func (s *McpService) discoverHTTPTools(ctx context.Context, rawURL string) ([]mcpstore.ToolRecord, error) {
	candidates := []toolDiscoveryAttempt{
		{Method: http.MethodGet, URL: rawURL},
		{Method: http.MethodGet, URL: strings.TrimRight(rawURL, "/") + "/tools"},
		{
			Method: http.MethodPost,
			URL:    rawURL,
			Body: map[string]any{
				"jsonrpc": "2.0",
				"id":      "runner-web",
				"method":  "tools/list",
				"params":  map[string]any{},
			},
		},
	}
	for _, attempt := range candidates {
		tools, err := s.tryDiscoverTools(ctx, attempt)
		if err == nil && len(tools) > 0 {
			return tools, nil
		}
	}
	return []mcpstore.ToolRecord{}, nil
}

type toolDiscoveryAttempt struct {
	Method string
	URL    string
	Body   map[string]any
}

func (s *McpService) tryDiscoverTools(ctx context.Context, attempt toolDiscoveryAttempt) ([]mcpstore.ToolRecord, error) {
	var bodyReader *bytes.Reader
	if len(attempt.Body) > 0 {
		raw, err := json.Marshal(attempt.Body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, attempt.Method, attempt.URL, bodyReader)
	if err != nil {
		return nil, err
	}
	if len(attempt.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discover tools failed: %s", resp.Status)
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	tools := extractTools(payload)
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools discovered")
	}
	return tools, nil
}

func extractTools(payload any) []mcpstore.ToolRecord {
	switch value := payload.(type) {
	case []any:
		return adaptToolList(value)
	case map[string]any:
		for _, key := range []string{"tools", "items"} {
			if nested, ok := value[key]; ok {
				return extractTools(nested)
			}
		}
		if result, ok := value["result"]; ok {
			return extractTools(result)
		}
	}
	return []mcpstore.ToolRecord{}
}

func adaptToolList(items []any) []mcpstore.ToolRecord {
	out := make([]mcpstore.ToolRecord, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		tool := mcpstore.ToolRecord{
			Name:        toString(raw["name"]),
			Description: toString(raw["description"]),
		}
		switch schema := raw["parameters_schema"].(type) {
		case map[string]any:
			tool.ParametersSchema = schema
		default:
			if schema, ok := raw["parametersSchema"].(map[string]any); ok {
				tool.ParametersSchema = schema
			} else if schema, ok := raw["inputSchema"].(map[string]any); ok {
				tool.ParametersSchema = schema
			}
		}
		if tool.Name == "" {
			continue
		}
		if tool.ParametersSchema == nil {
			tool.ParametersSchema = map[string]any{}
		}
		out = append(out, tool)
	}
	return out
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}
