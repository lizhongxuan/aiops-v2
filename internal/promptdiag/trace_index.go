package promptdiag

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type TraceIndex struct {
	RootDir       string
	traces        []TraceLink
	byPath        map[string]TraceLink
	byFingerprint map[string][]TraceLink
}

func BuildTraceIndex(root string) (TraceIndex, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(".data", "model-input-traces")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return TraceIndex{}, fmt.Errorf("resolve trace dir: %w", err)
	}
	index := TraceIndex{
		RootDir:       absRoot,
		byPath:        map[string]TraceLink{},
		byFingerprint: map[string][]TraceLink{},
	}
	if _, err := os.Stat(absRoot); err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return TraceIndex{}, fmt.Errorf("stat trace dir: %w", err)
	}
	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".json") || shouldSkipTraceIndexJSON(absRoot, path) {
			return nil
		}
		trace, err := readTraceLink(absRoot, path)
		if err != nil {
			return nil
		}
		index.traces = append(index.traces, trace)
		index.register(trace)
		return nil
	})
	if err != nil {
		return TraceIndex{}, fmt.Errorf("walk trace dir: %w", err)
	}
	sort.SliceStable(index.traces, func(i, j int) bool {
		if index.traces[i].CreatedAt == index.traces[j].CreatedAt {
			return index.traces[i].JSONPath < index.traces[j].JSONPath
		}
		return index.traces[i].CreatedAt < index.traces[j].CreatedAt
	})
	return index, nil
}

func (i TraceIndex) Lookup(path string) (TraceLink, bool) {
	for _, key := range traceLookupKeys(i.RootDir, path) {
		if trace, ok := i.byPath[key]; ok {
			return trace, true
		}
	}
	return TraceLink{}, false
}

func (i TraceIndex) FindByCaseID(caseID string) []TraceLink {
	caseID = sanitizeLoose(caseID)
	if caseID == "" {
		return nil
	}
	var out []TraceLink
	for _, trace := range i.traces {
		haystack := sanitizeLoose(strings.Join([]string{trace.CaseID, trace.SessionID, trace.TurnID, trace.JSONPath, trace.MarkdownPath}, " "))
		if strings.Contains(haystack, caseID) {
			out = append(out, trace)
		}
	}
	return out
}

func (i TraceIndex) FindByFingerprint(fingerprints []map[string]string) []TraceLink {
	var out []TraceLink
	for _, fp := range fingerprints {
		key := promptFingerprintKey(fp)
		if key == "" {
			continue
		}
		for _, trace := range i.byFingerprint[key] {
			out = appendTrace(out, trace)
		}
	}
	return out
}

func (i TraceIndex) register(trace TraceLink) {
	for _, path := range []string{trace.JSONPath, trace.MarkdownPath, trace.DiffPath} {
		for _, key := range traceLookupKeys(i.RootDir, path) {
			if key != "" {
				i.byPath[key] = trace
			}
		}
	}
	if key := promptFingerprintKey(trace.Fingerprint); key != "" {
		i.byFingerprint[key] = appendTrace(i.byFingerprint[key], trace)
	}
}

func shouldSkipTraceIndexJSON(root, path string) bool {
	if strings.EqualFold(filepath.Base(path), "index.json") {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.EqualFold(part, "raw") {
			return true
		}
	}
	return false
}

func readTraceLink(root, jsonPath string) (TraceLink, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return TraceLink{}, err
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
		Prompt            struct {
			Stable    string `json:"stable"`
			Dynamic   string `json:"dynamic"`
			System    string `json:"system"`
			Developer string `json:"developer"`
			Tools     string `json:"tools"`
			Policy    string `json:"policy"`
		} `json:"prompt"`
		ModelInput []struct {
			ProviderRole string `json:"providerRole"`
			SemanticRole string `json:"semanticRole"`
			PromptLayer  string `json:"promptLayer"`
			Content      string `json:"content"`
		} `json:"modelInput"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return TraceLink{}, err
	}
	base := strings.TrimSuffix(jsonPath, filepath.Ext(jsonPath))
	trace := TraceLink{
		CaseID:          firstNonEmpty(payload.CaseID, payload.Metadata["eval.caseId"], payload.Metadata["caseId"]),
		SessionID:       strings.TrimSpace(payload.SessionID),
		TurnID:          strings.TrimSpace(payload.TurnID),
		Iteration:       payload.Iteration,
		CreatedAt:       strings.TrimSpace(payload.CreatedAt),
		JSONPath:        relPath(root, jsonPath),
		MarkdownPath:    relIfExists(root, base+".md"),
		DiffPath:        relIfExists(root, base+".diff.md"),
		StableHash:      strings.TrimSpace(payload.PromptFingerprint["stableHash"]),
		MessageCount:    len(payload.ModelInput),
		PromptSizeChars: promptSize(payload),
		VisibleTools:    cleanStrings(payload.VisibleTools),
		Fingerprint:     cleanMap(payload.PromptFingerprint),
	}
	if trace.CaseID == "" {
		trace.CaseID = firstNonEmpty(deriveTraceCaseID(trace.SessionID), deriveTraceCaseID(trace.JSONPath))
	}
	for _, msg := range payload.ModelInput {
		if strings.EqualFold(strings.TrimSpace(msg.ProviderRole), "user") || strings.EqualFold(strings.TrimSpace(msg.SemanticRole), "user") {
			trace.HasUserMessage = true
			break
		}
	}
	return trace, nil
}

func promptFingerprintKey(fp map[string]string) string {
	if len(fp) == 0 {
		return ""
	}
	stable := strings.TrimSpace(fp["stableHash"])
	developer := strings.TrimSpace(fp["developerHash"])
	tools := strings.TrimSpace(fp["toolRegistryHash"])
	if stable == "" || developer == "" || tools == "" {
		return ""
	}
	return stable + "\x00" + developer + "\x00" + tools
}

func deriveTraceCaseID(value string) string {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" {
		return ""
	}
	for _, part := range strings.Split(value, "/") {
		if id := deriveTraceCaseIDFromPart(part); id != "" {
			return id
		}
	}
	return deriveTraceCaseIDFromPart(value)
}

func deriveTraceCaseIDFromPart(value string) string {
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

func promptSize(payload struct {
	Kind              string            `json:"kind"`
	CreatedAt         string            `json:"createdAt"`
	SessionID         string            `json:"sessionId"`
	TurnID            string            `json:"turnId"`
	Iteration         int               `json:"iteration"`
	VisibleTools      []string          `json:"visibleTools"`
	PromptFingerprint map[string]string `json:"promptFingerprint"`
	CaseID            string            `json:"caseId"`
	Metadata          map[string]string `json:"metadata"`
	Prompt            struct {
		Stable    string `json:"stable"`
		Dynamic   string `json:"dynamic"`
		System    string `json:"system"`
		Developer string `json:"developer"`
		Tools     string `json:"tools"`
		Policy    string `json:"policy"`
	} `json:"prompt"`
	ModelInput []struct {
		ProviderRole string `json:"providerRole"`
		SemanticRole string `json:"semanticRole"`
		PromptLayer  string `json:"promptLayer"`
		Content      string `json:"content"`
	} `json:"modelInput"`
}) int {
	total := len(payload.Prompt.Stable) + len(payload.Prompt.Dynamic)
	if total > 0 {
		return total
	}
	for _, msg := range payload.ModelInput {
		total += len(msg.Content)
	}
	return total
}

func traceLookupKeys(root, path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	var out []string
	add := func(value string) {
		value = filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
		if value == "." || value == "" {
			return
		}
		out = append(out, value)
		if strings.EqualFold(filepath.Ext(value), ".md") {
			out = append(out, strings.TrimSuffix(value, filepath.Ext(value))+".json")
		}
	}
	add(path)
	if !filepath.IsAbs(path) {
		add(filepath.Join(root, path))
	} else if rel, err := filepath.Rel(root, path); err == nil {
		add(rel)
	}
	if strings.HasPrefix(path, ".data"+string(filepath.Separator)) || strings.HasPrefix(path, ".data/") {
		if abs, err := filepath.Abs(path); err == nil {
			add(abs)
			if rel, err := filepath.Rel(root, abs); err == nil {
				add(rel)
			}
		}
	}
	return cleanStrings(out)
}

func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func relIfExists(root, path string) string {
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return relPath(root, path)
}

func sanitizeLoose(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	return value
}
