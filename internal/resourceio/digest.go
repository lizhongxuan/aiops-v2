package resourceio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func DigestContent(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func DetectContentType(name string, content []byte, explicit string) string {
	if normalized := NormalizeContentType(explicit); normalized != "" {
		return normalized
	}
	if len(content) == 0 {
		return "application/octet-stream"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".json" || json.Valid(content) {
		return "application/json"
	}
	if utf8.Valid(content) && !containsNUL(content) {
		return "text/plain"
	}
	detected := NormalizeContentType(http.DetectContentType(content))
	if detected == "" {
		return "application/octet-stream"
	}
	return detected
}

func NormalizeContentType(contentType string) string {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return ""
	}
	contentType = strings.ToLower(strings.SplitN(contentType, ";", 2)[0])
	switch contentType {
	case "application/x-json":
		return "application/json"
	default:
		return contentType
	}
}

func IsTextContentType(contentType string) bool {
	contentType = NormalizeContentType(contentType)
	return strings.HasPrefix(contentType, "text/") ||
		contentType == "application/json" ||
		contentType == "application/xml" ||
		contentType == "application/yaml" ||
		contentType == "application/x-yaml" ||
		contentType == "application/javascript" ||
		contentType == "application/x-javascript" ||
		strings.HasSuffix(contentType, "+json") ||
		strings.HasSuffix(contentType, "+xml")
}

func containsNUL(content []byte) bool {
	for _, b := range content {
		if b == 0 {
			return true
		}
	}
	return false
}
