package modeltrace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type RawPayloadRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Sha256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
}

func WriteRawPayloadRef(root, id, kind string, payload any) (RawPayloadRef, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = defaultTraceDocumentV2Root()
	}
	id = safeTraceDocumentV2Name(firstNonEmptyTraceV2(id, kind, "raw-payload"))
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "raw_payload"
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return RawPayloadRef{}, err
	}
	relPath := filepath.Join("raw", id+".json")
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return RawPayloadRef{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return RawPayloadRef{}, err
	}
	sum := sha256.Sum256(data)
	return RawPayloadRef{
		ID:     id,
		Kind:   kind,
		Path:   filepath.ToSlash(relPath),
		Sha256: "sha256:" + hex.EncodeToString(sum[:]),
		Bytes:  len(data),
	}, nil
}
