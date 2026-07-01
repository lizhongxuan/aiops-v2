package server

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WebAssetsHandler serves the built frontend and SPA fallbacks without taking
// over API or websocket transport paths.
type WebAssetsHandler struct {
	distDir    string
	indexPath  string
	fileServer http.Handler
}

// NewWebAssetsHandler validates a built frontend directory and returns a
// handler suitable for mounting on "/".
func NewWebAssetsHandler(distDir string) (*WebAssetsHandler, error) {
	root := strings.TrimSpace(distDir)
	if root == "" {
		return nil, fmt.Errorf("web dist dir is required")
	}
	indexPath := filepath.Join(root, "index.html")
	info, err := os.Stat(indexPath)
	if err != nil {
		return nil, fmt.Errorf("stat web index: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("web index %q is a directory", indexPath)
	}
	return &WebAssetsHandler{
		distDir:    filepath.Clean(root),
		indexPath:  indexPath,
		fileServer: http.FileServer(http.Dir(root)),
	}, nil
}

func (h *WebAssetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanedPath := normalizeWebPath(r.URL.Path)
	if isReservedTransportPath(cleanedPath) {
		http.NotFound(w, r)
		return
	}
	if cleanedPath == "/" {
		http.ServeFile(w, r, h.indexPath)
		return
	}
	if strings.HasPrefix(cleanedPath, "/assets/") {
		if h.servePrecompressedGzip(w, r, cleanedPath) {
			return
		}
		h.fileServer.ServeHTTP(w, withPath(r, cleanedPath))
		return
	}
	if candidate, ok := h.resolveFile(cleanedPath); ok {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if h.servePrecompressedGzip(w, r, cleanedPath) {
				return
			}
			h.fileServer.ServeHTTP(w, withPath(r, cleanedPath))
			return
		}
	}
	http.ServeFile(w, r, h.indexPath)
}

func (h *WebAssetsHandler) servePrecompressedGzip(w http.ResponseWriter, r *http.Request, requestPath string) bool {
	if h == nil || !clientAcceptsGzip(r) {
		return false
	}
	gzipPath, ok := h.resolveFile(requestPath + ".gz")
	if !ok {
		return false
	}
	info, err := os.Stat(gzipPath)
	if err != nil || info.IsDir() {
		return false
	}
	if contentType := mime.TypeByExtension(filepath.Ext(requestPath)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Vary", "Accept-Encoding")
	http.ServeFile(w, r, gzipPath)
	return true
}

func (h *WebAssetsHandler) resolveFile(requestPath string) (string, bool) {
	trimmed := strings.TrimPrefix(requestPath, "/")
	candidate := filepath.Join(h.distDir, filepath.FromSlash(trimmed))
	rel, err := filepath.Rel(h.distDir, candidate)
	if err != nil {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return candidate, true
}

func normalizeWebPath(value string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(value))
	if cleaned == "." || cleaned == "" {
		return "/"
	}
	return cleaned
}

func isReservedTransportPath(value string) bool {
	return strings.HasPrefix(value, "/api/") || value == "/ws" || strings.HasPrefix(value, "/api/v1/terminal/ws")
}

func clientAcceptsGzip(r *http.Request) bool {
	if r == nil {
		return false
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		if strings.EqualFold(strings.TrimSpace(strings.Split(part, ";")[0]), "gzip") {
			return true
		}
	}
	return false
}

func withPath(r *http.Request, requestPath string) *http.Request {
	clone := r.Clone(r.Context())
	nextURL := *r.URL
	nextURL.Path = requestPath
	clone.URL = &nextURL
	return clone
}
