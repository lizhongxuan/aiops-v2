package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/resourceio"
	"aiops-v2/internal/tooling"
)

type externalReferenceResponse struct {
	ID           string             `json:"id"`
	Kind         string             `json:"kind"`
	ContentType  string             `json:"contentType,omitempty"`
	Summary      string             `json:"summary,omitempty"`
	Content      string             `json:"content,omitempty"`
	Matches      []resourceio.Match `json:"matches,omitempty"`
	Bytes        int64              `json:"bytes,omitempty"`
	Digest       string             `json:"digest,omitempty"`
	Range        resourceio.Range   `json:"range,omitempty"`
	Offset       int64              `json:"offset,omitempty"`
	Limit        int                `json:"limit,omitempty"`
	Page         int                `json:"page,omitempty"`
	Truncated    bool               `json:"truncated,omitempty"`
	MetadataOnly bool               `json:"metadataOnly,omitempty"`
	ToolCallID   string             `json:"toolCallId,omitempty"`
	ToolName     string             `json:"toolName,omitempty"`
	SessionID    string             `json:"sessionId,omitempty"`
	TurnID       string             `json:"turnId,omitempty"`
	CreatedAt    string             `json:"createdAt,omitempty"`
}

func (s *HTTPServer) handleExternalReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, nestedKind := externalReferenceIDFromPath(r.URL.EscapedPath())
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "external reference id is required"})
		return
	}
	if nestedKind {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "unknown external reference kind"})
		return
	}
	repo, ok := externalReferenceRepository(s.ui)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "external reference store is not configured"})
		return
	}
	spill, err := repo.GetToolResultSpill(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "external reference not found"})
		return
	}
	if spill == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "external reference not found"})
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(spill.ID), "spill-") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "unknown external reference kind"})
		return
	}
	resp, err := externalReferenceResponseFromSpill(spill, externalReferenceReadRequestFromQuery(r.URL.Query()))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func externalReferenceIDFromPath(escapedPath string) (string, bool) {
	for _, prefix := range []string{"/api/v1/external-references/", "/api/external-references/"} {
		if !strings.HasPrefix(escapedPath, prefix) {
			continue
		}
		trimmed := strings.Trim(strings.TrimPrefix(escapedPath, prefix), "/")
		if strings.HasPrefix(trimmed, "tool-spills/") {
			id, err := url.PathUnescape(strings.TrimPrefix(trimmed, "tool-spills/"))
			if err != nil {
				return "", true
			}
			return strings.TrimSpace(id), false
		}
		if strings.Contains(trimmed, "/") {
			return "", true
		}
		id, err := url.PathUnescape(trimmed)
		if err != nil {
			return "", true
		}
		return strings.TrimSpace(id), false
	}
	return "", true
}

func externalReferenceRepository(ui appui.HTTPServices) (appui.ToolResultSpillRepository, bool) {
	if ui == nil {
		return nil, false
	}
	provider, ok := ui.(interface {
		ToolResultSpillRepository() appui.ToolResultSpillRepository
	})
	if !ok {
		return nil, false
	}
	repo := provider.ToolResultSpillRepository()
	return repo, repo != nil
}

func externalReferenceResponseFromSpill(spill *tooling.ResultSpill, req resourceio.ReadRequest) (externalReferenceResponse, error) {
	if spill == nil {
		return externalReferenceResponse{}, errors.New("external reference not found")
	}
	if strings.TrimSpace(spill.ID) == "" {
		return externalReferenceResponse{}, errors.New("external reference id is required")
	}
	digest := resourceio.DigestContent(spill.Content)
	bytes := spill.Bytes
	if bytes == 0 {
		bytes = int64(len(spill.Content))
	}
	contentType := strings.TrimSpace(spill.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	read := resourceio.ReadBytes(spill.Content, req, contentType)
	read.Ref.Digest = digest
	read.Ref.Bytes = bytes
	noRangeRequest := req.Offset == 0 && req.Limit == 0 && req.Page == 0 && strings.TrimSpace(req.Query) == "" && strings.TrimSpace(req.Format) == ""
	content := read.Content
	if noRangeRequest && resourceio.IsTextContentType(contentType) {
		content = strings.ToValidUTF8(string(spill.Content), "")
		read.Truncated = false
		read.MetadataOnly = false
	}
	resp := externalReferenceResponse{
		ID:           spill.ID,
		Kind:         "blob",
		ContentType:  read.Ref.ContentType,
		Summary:      spill.Summary,
		Content:      content,
		Matches:      read.Matches,
		Bytes:        bytes,
		Digest:       digest,
		Range:        read.Ref.Range,
		Offset:       read.Offset,
		Limit:        read.Limit,
		Page:         read.Page,
		Truncated:    read.Truncated,
		MetadataOnly: read.MetadataOnly,
		ToolCallID:   spill.ToolCallID,
		ToolName:     spill.ToolName,
		SessionID:    spill.SessionID,
		TurnID:       spill.TurnID,
	}
	if !spill.CreatedAt.IsZero() {
		resp.CreatedAt = spill.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if got := strings.TrimSpace(resp.Digest); got == "" {
		return externalReferenceResponse{}, fmt.Errorf("external reference %q digest is empty", spill.ID)
	}
	return resp, nil
}

func externalReferenceReadRequestFromQuery(query url.Values) resourceio.ReadRequest {
	return resourceio.ReadRequest{
		Offset: int64FromQuery(query, "offset"),
		Limit:  intFromQuery(query, "limit"),
		Page:   intFromQuery(query, "page"),
		Query:  query.Get("query"),
		Format: query.Get("format"),
	}
}

func intFromQuery(query url.Values, key string) int {
	value := strings.TrimSpace(query.Get(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func int64FromQuery(query url.Values, key string) int64 {
	value := strings.TrimSpace(query.Get(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
