package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/modeltrace"
)

const defaultPromptTraceLimit = 100
const maxPromptTraceLimit = 200

type PromptTraceListRequest struct {
	Limit  int
	Query  string
	CaseID string
	Trace  string
}

type PromptTraceListResponse struct {
	RootDir    string            `json:"rootDir"`
	Traces     []PromptTraceItem `json:"traces"`
	SelectedID string            `json:"selectedId,omitempty"`
}

type PromptTraceItem struct {
	ID                string            `json:"id"`
	RelativePath      string            `json:"relativePath"`
	JSONPath          string            `json:"jsonPath"`
	MarkdownPath      string            `json:"markdownPath,omitempty"`
	DiffPath          string            `json:"diffPath,omitempty"`
	Kind              string            `json:"kind,omitempty"`
	SessionID         string            `json:"sessionId,omitempty"`
	TurnID            string            `json:"turnId,omitempty"`
	Iteration         int               `json:"iteration"`
	CaseID            string            `json:"caseId,omitempty"`
	CreatedAt         string            `json:"createdAt,omitempty"`
	ModifiedAt        string            `json:"modifiedAt,omitempty"`
	VisibleTools      []string          `json:"visibleTools,omitempty"`
	MessageCount      int               `json:"messageCount,omitempty"`
	PromptFingerprint map[string]string `json:"promptFingerprint,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type PromptTraceFileRequest struct {
	Path string
}

type PromptTraceFileResponse struct {
	Path    string `json:"path"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

type PromptTraceService interface {
	ListModelInputTraces(ctx context.Context, req PromptTraceListRequest) (PromptTraceListResponse, error)
	GetModelInputTraceFile(ctx context.Context, req PromptTraceFileRequest) (PromptTraceFileResponse, error)
}

type promptTraceService struct {
	rootDir string
}

func NewPromptTraceService(rootDir string) PromptTraceService {
	return promptTraceService{rootDir: strings.TrimSpace(rootDir)}
}

func (s promptTraceService) ListModelInputTraces(ctx context.Context, req PromptTraceListRequest) (PromptTraceListResponse, error) {
	root, err := promptTraceRootDir(s.rootDir)
	if err != nil {
		return PromptTraceListResponse{}, err
	}
	limit := normalizePromptTraceLimit(req.Limit)
	response := PromptTraceListResponse{RootDir: root}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return response, nil
		}
		return PromptTraceListResponse{}, fmt.Errorf("stat prompt trace root: %w", err)
	}

	items := make([]PromptTraceItem, 0, limit)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		item, err := readPromptTraceItem(root, path)
		if err != nil {
			return nil
		}
		items = append(items, item)
		return nil
	})
	if err != nil {
		return PromptTraceListResponse{}, fmt.Errorf("walk prompt traces: %w", err)
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := promptTraceSortTime(items[i])
		right := promptTraceSortTime(items[j])
		if left.Equal(right) {
			return items[i].RelativePath > items[j].RelativePath
		}
		return left.After(right)
	})
	selectedID := promptTraceSelectedID(items, req.Trace)
	items = filterPromptTraceItems(items, req)
	if selectedID == "" && strings.TrimSpace(req.CaseID) != "" && len(items) > 0 {
		selectedID = items[0].ID
	}
	if len(items) > limit {
		items = items[:limit]
	}
	response.Traces = items
	if selectedID != "" && promptTraceContainsID(items, selectedID) {
		response.SelectedID = selectedID
	}
	return response, nil
}

func (s promptTraceService) GetModelInputTraceFile(ctx context.Context, req PromptTraceFileRequest) (PromptTraceFileResponse, error) {
	if err := ctx.Err(); err != nil {
		return PromptTraceFileResponse{}, err
	}
	root, err := promptTraceRootDir(s.rootDir)
	if err != nil {
		return PromptTraceFileResponse{}, err
	}
	path, rel, err := securePromptTracePath(root, req.Path)
	if err != nil {
		return PromptTraceFileResponse{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return PromptTraceFileResponse{}, fmt.Errorf("read prompt trace file: %w", err)
	}
	return PromptTraceFileResponse{
		Path:    rel,
		Format:  promptTraceFileFormat(rel),
		Content: string(data),
	}, nil
}

func promptTraceRootDir(configured string) (string, error) {
	root := strings.TrimSpace(configured)
	if root == "" {
		root = strings.TrimSpace(os.Getenv(modeltrace.DirEnv))
	}
	if root == "" {
		root = filepath.Join(".data", "model-input-traces")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve prompt trace root: %w", err)
	}
	return abs, nil
}

func normalizePromptTraceLimit(limit int) int {
	if limit <= 0 {
		return defaultPromptTraceLimit
	}
	if limit > maxPromptTraceLimit {
		return maxPromptTraceLimit
	}
	return limit
}

func readPromptTraceItem(root, jsonPath string) (PromptTraceItem, error) {
	info, err := os.Stat(jsonPath)
	if err != nil {
		return PromptTraceItem{}, err
	}
	rel := promptTraceRelativePath(root, jsonPath)
	item := PromptTraceItem{
		ID:           rel,
		RelativePath: rel,
		JSONPath:     rel,
		ModifiedAt:   info.ModTime().UTC().Format(time.RFC3339Nano),
	}
	var payload struct {
		Kind              string            `json:"kind"`
		CreatedAt         string            `json:"createdAt"`
		SessionID         string            `json:"sessionId"`
		TurnID            string            `json:"turnId"`
		Iteration         int               `json:"iteration"`
		VisibleTools      []string          `json:"visibleTools"`
		PromptFingerprint map[string]string `json:"promptFingerprint"`
		CaseID            string            `json:"caseId"`
		Metadata          map[string]string `json:"metadata"`
		ModelInput        []json.RawMessage `json:"modelInput"`
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return PromptTraceItem{}, err
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return PromptTraceItem{}, err
	}
	item.Kind = strings.TrimSpace(payload.Kind)
	item.CreatedAt = strings.TrimSpace(payload.CreatedAt)
	item.SessionID = strings.TrimSpace(payload.SessionID)
	item.TurnID = strings.TrimSpace(payload.TurnID)
	item.Iteration = payload.Iteration
	item.CaseID = firstPromptTraceNonEmpty(payload.CaseID, payload.Metadata["eval.caseId"], payload.Metadata["caseId"], derivePromptTraceCaseID(item.SessionID), derivePromptTraceCaseID(item.RelativePath))
	item.VisibleTools = append([]string(nil), payload.VisibleTools...)
	item.MessageCount = len(payload.ModelInput)
	item.PromptFingerprint = cleanPromptTraceFingerprint(payload.PromptFingerprint)
	item.Metadata = cleanPromptTraceFingerprint(payload.Metadata)

	base := strings.TrimSuffix(jsonPath, filepath.Ext(jsonPath))
	if fileExists(base + ".md") {
		item.MarkdownPath = promptTraceRelativePath(root, base+".md")
	}
	if fileExists(base + ".diff.md") {
		item.DiffPath = promptTraceRelativePath(root, base+".diff.md")
	}
	return item, nil
}

func filterPromptTraceItems(items []PromptTraceItem, req PromptTraceListRequest) []PromptTraceItem {
	needle := strings.ToLower(strings.TrimSpace(req.Query))
	caseID := strings.ToLower(strings.TrimSpace(req.CaseID))
	traceNeedle := strings.TrimSpace(req.Trace)
	if needle == "" && caseID == "" && traceNeedle == "" {
		return items
	}
	var out []PromptTraceItem
	for _, item := range items {
		if caseID != "" && !strings.EqualFold(item.CaseID, caseID) {
			continue
		}
		if traceNeedle != "" && !promptTraceMatchesTrace(item, traceNeedle) && caseID == "" {
			continue
		}
		if needle != "" && !promptTraceMatchesQuery(item, needle) {
			continue
		}
		out = append(out, item)
	}
	if traceNeedle != "" && !promptTraceContainsID(out, promptTraceSelectedID(items, traceNeedle)) {
		selected := promptTraceSelectedID(items, traceNeedle)
		for _, item := range items {
			if item.ID == selected {
				out = append([]PromptTraceItem{item}, out...)
				break
			}
		}
	}
	return out
}

func promptTraceMatchesQuery(item PromptTraceItem, needle string) bool {
	values := []string{
		item.CaseID,
		item.SessionID,
		item.TurnID,
		item.RelativePath,
		item.Kind,
		item.PromptFingerprint["stableHash"],
		item.PromptFingerprint["developerHash"],
		item.PromptFingerprint["toolRegistryHash"],
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func promptTraceSelectedID(items []PromptTraceItem, trace string) string {
	trace = strings.TrimSpace(trace)
	if trace == "" {
		return ""
	}
	for _, item := range items {
		if promptTraceMatchesTrace(item, trace) {
			return item.ID
		}
	}
	return ""
}

func promptTraceMatchesTrace(item PromptTraceItem, trace string) bool {
	trace = filepath.ToSlash(strings.TrimSpace(trace))
	if trace == "" {
		return false
	}
	for _, value := range []string{item.ID, item.RelativePath, item.JSONPath, item.MarkdownPath, item.DiffPath} {
		if filepath.ToSlash(strings.TrimSpace(value)) == trace {
			return true
		}
	}
	return false
}

func promptTraceContainsID(items []PromptTraceItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func securePromptTracePath(root, requested string) (string, string, error) {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(requested)))
	if rel == "" || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return "", "", fmt.Errorf("invalid prompt trace path")
	}
	ext := filepath.Ext(rel)
	if ext != ".md" && ext != ".json" {
		return "", "", fmt.Errorf("unsupported prompt trace file type")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	path, err := filepath.Abs(filepath.Join(absRoot, rel))
	if err != nil {
		return "", "", err
	}
	if path != absRoot && !strings.HasPrefix(path, absRoot+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("prompt trace path escapes root")
	}
	return path, filepath.ToSlash(rel), nil
}

func promptTraceRelativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func promptTraceFileFormat(path string) string {
	switch filepath.Ext(path) {
	case ".json":
		return "json"
	default:
		return "markdown"
	}
}

func promptTraceSortTime(item PromptTraceItem) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, item.CreatedAt); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, item.ModifiedAt); err == nil {
		return t
	}
	return time.Time{}
}

func cleanPromptTraceFingerprint(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range in {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstPromptTraceNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func derivePromptTraceCaseID(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" {
		return ""
	}
	for _, part := range strings.Split(value, "/") {
		if id := derivePromptTraceCaseIDFromPart(part); id != "" {
			return id
		}
	}
	return derivePromptTraceCaseIDFromPart(value)
}

func derivePromptTraceCaseIDFromPart(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{
		"eval-prompt-regression-",
		"eval-prompt-p0-server-local-gpt54-",
		"eval-prompt-p0-server-",
		"eval-prompt-p0-mock-",
	} {
		if !strings.HasPrefix(value, prefix) {
			continue
		}
		rest := strings.TrimPrefix(value, prefix)
		if prefix == "eval-prompt-regression-" {
			if idx := strings.Index(rest, "-"); idx > 0 {
				rest = rest[idx+1:]
			}
		}
		return strings.Trim(rest, "-")
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
