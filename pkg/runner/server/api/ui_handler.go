package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

type UIHandler struct {
	fs         fs.FS
	fileServer http.Handler
	indexHTML  []byte
}

func NewUIHandler(distDir string, basePath string, embedded fs.FS, fallback fs.FS) (*UIHandler, error) {
	root, err := resolveUIRoot(distDir, embedded, fallback)
	if err != nil {
		return nil, err
	}
	indexHTML, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, err
	}
	indexHTML = injectUIBasePath(indexHTML, normalizeBasePath(basePath))

	return &UIHandler{
		fs:         root,
		fileServer: http.FileServer(http.FS(root)),
		indexHTML:  indexHTML,
	}, nil
}

func resolveUIRoot(distDir string, embedded fs.FS, fallback fs.FS) (fs.FS, error) {
	candidates := make([]fs.FS, 0, 3)
	if strings.TrimSpace(distDir) != "" {
		candidates = append(candidates, osDirFS(distDir))
	}
	if embedded != nil {
		candidates = append(candidates, embedded)
	}
	if fallback != nil {
		candidates = append(candidates, fallback)
	}

	var lastErr error
	for _, candidate := range candidates {
		if _, err := fs.ReadFile(candidate, "index.html"); err == nil {
			return candidate, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fs.ErrNotExist
	}
	return nil, lastErr
}

func (h *UIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	if name == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(h.indexHTML)
		return
	}

	if info, err := fs.Stat(h.fs, name); err == nil && !info.IsDir() {
		h.fileServer.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(h.indexHTML)
}

func osDirFS(root string) fs.FS {
	return osDirFSImpl(root)
}

func injectUIBasePath(indexHTML []byte, basePath string) []byte {
	injection := fmt.Sprintf(
		"<base href=\"%s\">\n<script>window.__RUNNER_WEB_CONFIG__=Object.assign({},window.__RUNNER_WEB_CONFIG__,{basePath:%q});</script>\n",
		basePath,
		basePath,
	)

	content := string(indexHTML)
	if strings.Contains(content, "<head>") {
		return []byte(strings.Replace(content, "<head>", "<head>\n"+injection, 1))
	}
	if strings.Contains(content, "</head>") {
		return []byte(strings.Replace(content, "</head>", injection+"</head>", 1))
	}
	return append([]byte(injection), indexHTML...)
}
