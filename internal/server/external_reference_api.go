package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/tooling"
)

type externalReferenceResponse struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ContentType string `json:"contentType,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Content     string `json:"content,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	Digest      string `json:"digest,omitempty"`
	ToolCallID  string `json:"toolCallId,omitempty"`
	ToolName    string `json:"toolName,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	TurnID      string `json:"turnId,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
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
	resp, err := externalReferenceResponseFromSpill(spill)
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

func externalReferenceResponseFromSpill(spill *tooling.ResultSpill) (externalReferenceResponse, error) {
	if spill == nil {
		return externalReferenceResponse{}, errors.New("external reference not found")
	}
	if strings.TrimSpace(spill.ID) == "" {
		return externalReferenceResponse{}, errors.New("external reference id is required")
	}
	content := string(spill.Content)
	digest := digestExternalReferenceContent(content)
	bytes := spill.Bytes
	if bytes == 0 {
		bytes = int64(len(spill.Content))
	}
	contentType := strings.TrimSpace(spill.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	resp := externalReferenceResponse{
		ID:          spill.ID,
		Kind:        "blob",
		ContentType: contentType,
		Summary:     spill.Summary,
		Content:     content,
		Bytes:       bytes,
		Digest:      digest,
		ToolCallID:  spill.ToolCallID,
		ToolName:    spill.ToolName,
		SessionID:   spill.SessionID,
		TurnID:      spill.TurnID,
	}
	if !spill.CreatedAt.IsZero() {
		resp.CreatedAt = spill.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if got := strings.TrimSpace(resp.Digest); got == "" {
		return externalReferenceResponse{}, fmt.Errorf("external reference %q digest is empty", spill.ID)
	}
	return resp, nil
}

func digestExternalReferenceContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
