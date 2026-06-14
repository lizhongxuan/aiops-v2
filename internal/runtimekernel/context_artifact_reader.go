package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/resourceio"
	"aiops-v2/internal/tooling"
)

const defaultContextArtifactReadBytes = 4096

type ContextArtifactReaderOptions struct {
	Repository      ContextArtifactRepository
	SpillRepository ToolResultSpillRepository
	MaxReadBytes    int
	DefaultPreview  int
}

type ContextArtifactReader struct {
	repository      ContextArtifactRepository
	spillRepository ToolResultSpillRepository
	maxReadBytes    int
	defaultPreview  int
}

type ContextArtifactReadRequest struct {
	ID     string           `json:"id,omitempty"`
	Range  resourceio.Range `json:"range,omitempty"`
	Offset int64            `json:"offset,omitempty"`
	Limit  int              `json:"limit,omitempty"`
	Query  string           `json:"query,omitempty"`
	Page   int              `json:"page,omitempty"`
	Format string           `json:"format,omitempty"`
}

type ContextArtifactReadResult struct {
	Artifact     ContextArtifact        `json:"artifact"`
	Content      string                 `json:"content,omitempty"`
	Matches      []ContextArtifactMatch `json:"matches,omitempty"`
	Ref          string                 `json:"ref,omitempty"`
	Range        resourceio.Range       `json:"range,omitempty"`
	Offset       int64                  `json:"offset,omitempty"`
	Limit        int                    `json:"limit,omitempty"`
	Page         int                    `json:"page,omitempty"`
	Truncated    bool                   `json:"truncated,omitempty"`
	MetadataOnly bool                   `json:"metadataOnly,omitempty"`
}

type ContextArtifactMatch struct {
	Offset  int64  `json:"offset"`
	Content string `json:"content"`
}

func NewContextArtifactReader(opts ContextArtifactReaderOptions) ContextArtifactReader {
	maxReadBytes := opts.MaxReadBytes
	if maxReadBytes <= 0 {
		maxReadBytes = defaultContextArtifactReadBytes
	}
	defaultPreview := opts.DefaultPreview
	if defaultPreview <= 0 {
		defaultPreview = 240
	}
	return ContextArtifactReader{
		repository:      opts.Repository,
		spillRepository: opts.SpillRepository,
		maxReadBytes:    maxReadBytes,
		defaultPreview:  defaultPreview,
	}
}

func (r ContextArtifactReader) Read(req ContextArtifactReadRequest) (ContextArtifactReadResult, error) {
	artifact, content, err := r.resolve(req.ID)
	if err != nil {
		return ContextArtifactReadResult{}, err
	}
	result := ContextArtifactReadResult{
		Artifact: artifact,
		Ref:      artifact.URI,
	}
	read := resourceio.ReadBytesWithMax(content, resourceio.ReadRequest{
		ID:     artifact.ID,
		URI:    artifact.URI,
		Range:  req.Range,
		Offset: req.Offset,
		Limit:  req.Limit,
		Query:  req.Query,
		Page:   req.Page,
		Format: req.Format,
	}, artifact.ContentType, r.maxReadBytes)
	result.Content = read.Content
	result.Matches = contextArtifactMatchesFromResource(read.Matches)
	result.Range = read.Ref.Range
	result.Offset = read.Offset
	result.Limit = read.Limit
	result.Page = read.Page
	result.Truncated = read.Truncated
	result.MetadataOnly = read.MetadataOnly
	return result, nil
}

func (r ContextArtifactReader) readLimit(limit int) int {
	if limit <= 0 || limit > r.maxReadBytes {
		return r.maxReadBytes
	}
	return limit
}

func (r ContextArtifactReader) queryText(text, query string, limit int) []ContextArtifactMatch {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	var matches []ContextArtifactMatch
	searchFrom := 0
	for {
		idx := strings.Index(lowerText[searchFrom:], lowerQuery)
		if idx < 0 {
			break
		}
		pos := searchFrom + idx
		start := pos - limit/2
		if start < 0 {
			start = 0
		}
		end := start + limit
		if end < pos+len(query) {
			end = pos + len(query)
		}
		if end > len(text) {
			end = len(text)
		}
		matches = append(matches, ContextArtifactMatch{
			Offset:  int64(pos),
			Content: text[start:end],
		})
		searchFrom = pos + len(query)
		if len(matches) >= 20 {
			break
		}
	}
	return matches
}

func contextArtifactMatchesFromResource(matches []resourceio.Match) []ContextArtifactMatch {
	if len(matches) == 0 {
		return nil
	}
	out := make([]ContextArtifactMatch, 0, len(matches))
	for _, match := range matches {
		out = append(out, ContextArtifactMatch{
			Offset:  match.Offset,
			Content: match.Content,
		})
	}
	return out
}

func (r ContextArtifactReader) resolve(id string) (ContextArtifact, []byte, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ContextArtifact{}, nil, fmt.Errorf("context artifact id is required")
	}
	if r.repository != nil {
		if artifact, content, err := r.repository.GetContextArtifact(id); err == nil {
			return artifact, content, nil
		}
	}
	if r.spillRepository != nil {
		spillID := strings.TrimPrefix(id, "store://tool-spills/")
		if spillID != "" {
			spill, err := r.spillRepository.GetToolResultSpill(spillID)
			if err == nil {
				return contextArtifactFromSpill(spill), append([]byte(nil), spill.Content...), nil
			}
		}
	}
	return ContextArtifact{}, nil, ErrContextArtifactNotFound
}

func contextArtifactFromSpill(spill *tooling.ResultSpill) ContextArtifact {
	if spill == nil {
		return ContextArtifact{}
	}
	return BuildContextArtifact(ContextArtifactWrite{
		ID:          spill.ID,
		Kind:        "tool_result",
		URI:         "store://tool-spills/" + spill.ID,
		ContentType: spill.ContentType,
		Content:     spill.Content,
		Summary:     spill.Summary,
		CreatedAt:   spill.CreatedAt,
		Source: ContextArtifactSource{
			ToolCallID: spill.ToolCallID,
			ToolName:   spill.ToolName,
			SessionID:  spill.SessionID,
			TurnID:     spill.TurnID,
		},
	})
}
