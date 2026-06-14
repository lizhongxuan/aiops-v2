package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/modeltrace"
)

const defaultPromptTraceLimit = 500
const maxPromptTraceLimit = 2000

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
	ID                string                         `json:"id"`
	RelativePath      string                         `json:"relativePath"`
	JSONPath          string                         `json:"jsonPath"`
	MarkdownPath      string                         `json:"markdownPath,omitempty"`
	DiffPath          string                         `json:"diffPath,omitempty"`
	Kind              string                         `json:"kind,omitempty"`
	SessionID         string                         `json:"sessionId,omitempty"`
	TurnID            string                         `json:"turnId,omitempty"`
	Iteration         int                            `json:"iteration"`
	CaseID            string                         `json:"caseId,omitempty"`
	CreatedAt         string                         `json:"createdAt,omitempty"`
	ModifiedAt        string                         `json:"modifiedAt,omitempty"`
	VisibleTools      []string                       `json:"visibleTools,omitempty"`
	MessageCount      int                            `json:"messageCount,omitempty"`
	LLMRequestCount   int                            `json:"llmRequestCount,omitempty"`
	Usage             *PromptTraceUsage              `json:"usage,omitempty"`
	AverageDurationMs int64                          `json:"averageDurationMs,omitempty"`
	UserPromptPreview string                         `json:"userPromptPreview,omitempty"`
	ToolSurface       *PromptTraceToolSurfaceSummary `json:"toolSurface,omitempty"`
	PromptFingerprint map[string]string              `json:"promptFingerprint,omitempty"`
	Metadata          map[string]string              `json:"metadata,omitempty"`
}

type PromptTraceUsage struct {
	PromptTokens     int `json:"promptTokens,omitempty"`
	CompletionTokens int `json:"completionTokens,omitempty"`
	TotalTokens      int `json:"totalTokens,omitempty"`
}

type PromptTraceToolSurfaceSummary struct {
	InitialToolCount     int               `json:"initialToolCount,omitempty"`
	BaseRegistryCount    int               `json:"baseRegistryCount,omitempty"`
	DeferredFamilyCount  int               `json:"deferredFamilyCount,omitempty"`
	LoadedToolCount      int               `json:"loadedToolCount,omitempty"`
	LoadedPackCount      int               `json:"loadedPackCount,omitempty"`
	FilteredToolCount    int               `json:"filteredToolCount,omitempty"`
	ToolSearchEventCount int               `json:"toolSearchEventCount,omitempty"`
	SelectedToolCount    int               `json:"selectedToolCount,omitempty"`
	RejectedToolCount    int               `json:"rejectedToolCount,omitempty"`
	MCPHealth            map[string]string `json:"mcpHealth,omitempty"`
	FilteredReasons      map[string]string `json:"filteredReasons,omitempty"`
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

type promptTraceMessage struct {
	Role         string `json:"role"`
	ProviderRole string `json:"providerRole"`
	SemanticRole string `json:"semanticRole"`
	PromptLayer  string `json:"promptLayer"`
	Content      string `json:"content"`
}

type promptTraceUsagePayload struct {
	PromptTokens          int `json:"prompt_tokens"`
	PromptTokensCamel     int `json:"promptTokens"`
	InputTokens           int `json:"input_tokens"`
	InputTokensCamel      int `json:"inputTokens"`
	CompletionTokens      int `json:"completion_tokens"`
	CompletionTokensCamel int `json:"completionTokens"`
	OutputTokens          int `json:"output_tokens"`
	OutputTokensCamel     int `json:"outputTokens"`
	TotalTokens           int `json:"total_tokens"`
	TotalTokensCamel      int `json:"totalTokens"`
	Total                 int `json:"total"`
}

type promptTraceLLMRequestPayload struct {
	Usage           promptTraceUsagePayload `json:"usage"`
	DurationMs      float64                 `json:"durationMs"`
	DurationMSSnake float64                 `json:"duration_ms"`
	LatencyMs       float64                 `json:"latencyMs"`
	LatencyMSSnake  float64                 `json:"latency_ms"`
	ElapsedMs       float64                 `json:"elapsedMs"`
	ElapsedMSSnake  float64                 `json:"elapsed_ms"`
}

type promptTraceToolSurfacePayload struct {
	InitialTools        []string                        `json:"initialTools"`
	BaseRegistryCount   int                             `json:"baseRegistryCount"`
	DeferredFamilies    []promptTraceDeferredFamily     `json:"deferredFamilies"`
	LoadedTools         []string                        `json:"loadedTools"`
	LoadedPacks         []string                        `json:"loadedPacks"`
	FilteredTools       []promptTraceFilteredTool       `json:"filteredTools"`
	MCPHealth           map[string]string               `json:"mcpHealth"`
	ToolSearchEvents    []promptTraceToolSearchEvent    `json:"toolSearchEvents"`
	SelectedTools       []string                        `json:"selectedTools"`
	RejectedToolReasons []promptTraceRejectedToolReason `json:"rejectedToolReasons"`
}

type promptTraceDeferredFamily struct {
	Pack string `json:"pack"`
}

type promptTraceFilteredTool struct {
	ToolName string `json:"toolName"`
	Reason   string `json:"reason"`
}

type promptTraceToolSearchEvent struct {
	Mode string `json:"mode"`
}

type promptTraceRejectedToolReason struct {
	ToolName string `json:"toolName"`
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
		Kind               string                         `json:"kind"`
		CreatedAt          string                         `json:"createdAt"`
		SessionID          string                         `json:"sessionId"`
		TurnID             string                         `json:"turnId"`
		Iteration          int                            `json:"iteration"`
		VisibleTools       []string                       `json:"visibleTools"`
		PromptFingerprint  map[string]string              `json:"promptFingerprint"`
		CaseID             string                         `json:"caseId"`
		Metadata           map[string]string              `json:"metadata"`
		ToolSurfaceTrace   promptTraceToolSurfacePayload  `json:"toolSurfaceTrace"`
		ModelInput         []promptTraceMessage           `json:"modelInput"`
		Usage              promptTraceUsagePayload        `json:"usage"`
		DurationMs         float64                        `json:"durationMs"`
		DurationMSSnake    float64                        `json:"duration_ms"`
		LatencyMs          float64                        `json:"latencyMs"`
		LatencyMSSnake     float64                        `json:"latency_ms"`
		ElapsedMs          float64                        `json:"elapsedMs"`
		ElapsedMSSnake     float64                        `json:"elapsed_ms"`
		LLMRequests        []promptTraceLLMRequestPayload `json:"llmRequests"`
		LLMRequestsSnake   []promptTraceLLMRequestPayload `json:"llm_requests"`
		ModelRequests      []promptTraceLLMRequestPayload `json:"modelRequests"`
		ModelRequestsSnake []promptTraceLLMRequestPayload `json:"model_requests"`
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
	item.LLMRequestCount, item.Usage, item.AverageDurationMs = promptTraceLLMStats(payload.Usage, promptTraceFirstDuration(payload.DurationMs, payload.DurationMSSnake, payload.LatencyMs, payload.LatencyMSSnake, payload.ElapsedMs, payload.ElapsedMSSnake), payload.LLMRequests, payload.LLMRequestsSnake, payload.ModelRequests, payload.ModelRequestsSnake)
	item.UserPromptPreview = promptTraceUserPromptPreview(payload.ModelInput)
	item.ToolSurface = promptTraceToolSurfaceSummary(payload.ToolSurfaceTrace)
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

var promptTraceSecretPattern = regexp.MustCompile(`(?i)\b(api[_\s-]*key|token|secret|password|cookie|authorization)\s*[:=]\s*["']?[^"\s,;}\]]+`)

func promptTraceUserPromptPreview(messages []promptTraceMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		msg := messages[index]
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") && !strings.EqualFold(strings.TrimSpace(msg.ProviderRole), "user") && !strings.EqualFold(strings.TrimSpace(msg.SemanticRole), "user") {
			continue
		}
		return promptTracePreviewText(promptTraceRedactPreview(msg.Content), 180)
	}
	return ""
}

func promptTraceRedactPreview(value string) string {
	value = diagnostics.RedactSensitiveText(value)
	return promptTraceSecretPattern.ReplaceAllStringFunc(value, func(match string) string {
		key := match
		if index := strings.IndexAny(match, ":="); index >= 0 {
			key = strings.TrimSpace(match[:index])
		}
		return key + "=[REDACTED]"
	})
}

func promptTraceToolSurfaceSummary(in promptTraceToolSurfacePayload) *PromptTraceToolSurfaceSummary {
	summary := &PromptTraceToolSurfaceSummary{
		InitialToolCount:     len(in.InitialTools),
		BaseRegistryCount:    in.BaseRegistryCount,
		DeferredFamilyCount:  len(in.DeferredFamilies),
		LoadedToolCount:      len(in.LoadedTools),
		LoadedPackCount:      len(in.LoadedPacks),
		FilteredToolCount:    len(in.FilteredTools),
		ToolSearchEventCount: len(in.ToolSearchEvents),
		SelectedToolCount:    len(in.SelectedTools),
		RejectedToolCount:    len(in.RejectedToolReasons),
		MCPHealth:            redactPromptTraceStringMap(in.MCPHealth),
		FilteredReasons:      promptTraceFilteredReasons(in.FilteredTools),
	}
	if promptTraceToolSurfaceSummaryEmpty(summary) {
		return nil
	}
	return summary
}

func promptTraceFilteredReasons(filtered []promptTraceFilteredTool) map[string]string {
	if len(filtered) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, item := range filtered {
		name := strings.TrimSpace(diagnostics.RedactSensitiveText(item.ToolName))
		reason := strings.TrimSpace(diagnostics.RedactSensitiveText(item.Reason))
		if name == "" || reason == "" {
			continue
		}
		out[name] = reason
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func redactPromptTraceStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range in {
		key = strings.TrimSpace(diagnostics.RedactSensitiveText(key))
		value = strings.TrimSpace(diagnostics.RedactSensitiveText(value))
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func promptTraceToolSurfaceSummaryEmpty(summary *PromptTraceToolSurfaceSummary) bool {
	return summary == nil ||
		summary.InitialToolCount == 0 &&
			summary.BaseRegistryCount == 0 &&
			summary.DeferredFamilyCount == 0 &&
			summary.LoadedToolCount == 0 &&
			summary.LoadedPackCount == 0 &&
			summary.FilteredToolCount == 0 &&
			summary.ToolSearchEventCount == 0 &&
			summary.SelectedToolCount == 0 &&
			summary.RejectedToolCount == 0 &&
			len(summary.MCPHealth) == 0 &&
			len(summary.FilteredReasons) == 0
}

func promptTracePreviewText(value string, maxRunes int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if text == "" || maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func promptTraceLLMStats(rootUsage promptTraceUsagePayload, rootDuration float64, groups ...[]promptTraceLLMRequestPayload) (int, *PromptTraceUsage, int64) {
	requests := make([]promptTraceLLMRequestPayload, 0)
	for _, group := range groups {
		requests = append(requests, group...)
	}
	if len(requests) == 0 {
		usage := promptTraceNormalizeUsage(rootUsage)
		duration := promptTraceRoundDuration(rootDuration)
		count := 0
		if usage != nil || duration > 0 {
			count = 1
		}
		return count, usage, duration
	}
	total := PromptTraceUsage{}
	durationTotal := int64(0)
	durationCount := int64(0)
	for _, request := range requests {
		if usage := promptTraceNormalizeUsage(request.Usage); usage != nil {
			total.PromptTokens += usage.PromptTokens
			total.CompletionTokens += usage.CompletionTokens
			total.TotalTokens += usage.TotalTokens
		}
		if duration := promptTraceRoundDuration(promptTraceFirstDuration(request.DurationMs, request.DurationMSSnake, request.LatencyMs, request.LatencyMSSnake, request.ElapsedMs, request.ElapsedMSSnake)); duration > 0 {
			durationTotal += duration
			durationCount++
		}
	}
	var usage *PromptTraceUsage
	if total.PromptTokens > 0 || total.CompletionTokens > 0 || total.TotalTokens > 0 {
		usage = &total
	}
	averageDuration := int64(0)
	if durationCount > 0 {
		averageDuration = durationTotal / durationCount
	}
	return len(requests), usage, averageDuration
}

func promptTraceNormalizeUsage(usage promptTraceUsagePayload) *PromptTraceUsage {
	out := PromptTraceUsage{
		PromptTokens:     firstPromptTraceInt(usage.PromptTokens, usage.PromptTokensCamel, usage.InputTokens, usage.InputTokensCamel),
		CompletionTokens: firstPromptTraceInt(usage.CompletionTokens, usage.CompletionTokensCamel, usage.OutputTokens, usage.OutputTokensCamel),
		TotalTokens:      firstPromptTraceInt(usage.TotalTokens, usage.TotalTokensCamel, usage.Total),
	}
	if out.TotalTokens == 0 && (out.PromptTokens > 0 || out.CompletionTokens > 0) {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	if out.PromptTokens == 0 && out.CompletionTokens == 0 && out.TotalTokens == 0 {
		return nil
	}
	return &out
}

func promptTraceFirstDuration(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func promptTraceRoundDuration(value float64) int64 {
	if value <= 0 {
		return 0
	}
	return int64(value + 0.5)
}

func firstPromptTraceInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
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
