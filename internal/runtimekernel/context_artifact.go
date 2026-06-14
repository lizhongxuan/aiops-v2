package runtimekernel

import (
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/resourceio"
)

var ErrContextArtifactNotFound = errors.New("context artifact not found")

type ContextArtifact struct {
	ID          string                `json:"id"`
	Kind        string                `json:"kind"`
	URI         string                `json:"uri"`
	ContentType string                `json:"contentType"`
	Extension   string                `json:"extension"`
	Digest      string                `json:"digest"`
	Bytes       int64                 `json:"bytes"`
	Summary     string                `json:"summary,omitempty"`
	Preview     string                `json:"preview,omitempty"`
	CreatedAt   time.Time             `json:"createdAt,omitempty"`
	Source      ContextArtifactSource `json:"source,omitempty"`
}

type ContextArtifactSource struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
	TurnID     string `json:"turnId,omitempty"`
}

type ContextArtifactWrite struct {
	ID          string
	Kind        string
	URI         string
	ContentType string
	Content     []byte
	Summary     string
	Preview     string
	CreatedAt   time.Time
	Source      ContextArtifactSource
}

type MemoryContextArtifactRepository struct {
	mu       sync.RWMutex
	metadata map[string]ContextArtifact
	content  map[string][]byte
}

func NewMemoryContextArtifactRepository() *MemoryContextArtifactRepository {
	return &MemoryContextArtifactRepository{
		metadata: make(map[string]ContextArtifact),
		content:  make(map[string][]byte),
	}
}

func (r *MemoryContextArtifactRepository) GetContextArtifact(id string) (ContextArtifact, []byte, error) {
	if r == nil {
		return ContextArtifact{}, nil, ErrContextArtifactNotFound
	}
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	artifact, ok := r.metadata[id]
	if !ok {
		return ContextArtifact{}, nil, ErrContextArtifactNotFound
	}
	return artifact, append([]byte(nil), r.content[id]...), nil
}

func (r *MemoryContextArtifactRepository) SaveContextArtifact(write ContextArtifactWrite) (ContextArtifact, error) {
	if r == nil {
		return ContextArtifact{}, fmt.Errorf("context artifact repository is nil")
	}
	artifact := BuildContextArtifact(write)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metadata[artifact.ID] = artifact
	r.content[artifact.ID] = append([]byte(nil), write.Content...)
	return artifact, nil
}

func BuildContextArtifact(write ContextArtifactWrite) ContextArtifact {
	contentType := normalizeContextArtifactContentType(write.ContentType, write.Content)
	digest := contextArtifactDigest(write.Content)
	id := strings.TrimSpace(write.ID)
	if id == "" {
		id = "artifact-" + strings.TrimPrefix(digest, "sha256:")[:16]
	}
	uri := strings.TrimSpace(write.URI)
	if uri == "" {
		uri = "store://artifacts/" + id
	}
	createdAt := write.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}
	preview := strings.TrimSpace(write.Preview)
	if preview == "" {
		preview = contextArtifactPreview(contentType, write.Content, uri, 240)
	}
	summary := strings.TrimSpace(write.Summary)
	if summary == "" {
		summary = contextArtifactBoundedSnippet(preview)
	}
	return ContextArtifact{
		ID:          id,
		Kind:        contextArtifactFirstNonBlank(write.Kind, "generated"),
		URI:         uri,
		ContentType: contentType,
		Extension:   contextArtifactExtension(contentType, uri),
		Digest:      digest,
		Bytes:       int64(len(write.Content)),
		Summary:     summary,
		Preview:     preview,
		CreatedAt:   createdAt,
		Source:      write.Source,
	}
}

func normalizeContextArtifactContentType(contentType string, content []byte) string {
	return resourceio.DetectContentType("", content, contentType)
}

func contextArtifactExtension(contentType, uri string) string {
	if ext := filepath.Ext(uri); ext != "" {
		return ext
	}
	switch contentType {
	case "application/json":
		return ".json"
	case "text/markdown":
		return ".md"
	case "text/csv":
		return ".csv"
	case "text/plain":
		return ".txt"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "application/octet-stream":
		return ".bin"
	}
	extensions, _ := mime.ExtensionsByType(contentType)
	if len(extensions) > 0 {
		return extensions[0]
	}
	if strings.HasPrefix(contentType, "image/") {
		return "." + strings.TrimPrefix(contentType, "image/")
	}
	return ".bin"
}

func contextArtifactDigest(content []byte) string {
	return resourceio.DigestContent(content)
}

func contextArtifactPreview(contentType string, content []byte, uri string, limit int) string {
	if limit <= 0 {
		limit = 240
	}
	if !contextArtifactIsText(contentType) {
		return fmt.Sprintf("Binary artifact omitted from model-visible content. ref=%s contentType=%s bytes=%d", uri, contentType, len(content))
	}
	text := strings.TrimSpace(strings.ToValidUTF8(string(content), ""))
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

func contextArtifactIsText(contentType string) bool {
	return resourceio.IsTextContentType(contentType)
}

func contextArtifactBoundedSnippet(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	const limit = 120
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func contextArtifactFirstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
