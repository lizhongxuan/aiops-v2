package api

import (
	"net/http"
	"strings"
)

func BasePathMiddleware(basePath string) func(http.Handler) http.Handler {
	normalized := normalizeBasePath(basePath)
	if normalized == "/" {
		return func(next http.Handler) http.Handler { return next }
	}

	prefix := strings.TrimSuffix(normalized, "/")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path, ok := stripBasePath(r.URL.Path, prefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			cloned := r.Clone(r.Context())
			urlCopy := *r.URL
			urlCopy.Path = path
			if rawPath, ok := stripBasePath(r.URL.RawPath, prefix); ok {
				urlCopy.RawPath = rawPath
			} else {
				urlCopy.RawPath = ""
			}
			cloned.URL = &urlCopy
			next.ServeHTTP(w, cloned)
		})
	}
}

func normalizeBasePath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || value == "/" {
		return "/"
	}
	return "/" + strings.Trim(value, "/") + "/"
}

func stripBasePath(path string, prefix string) (string, bool) {
	if prefix == "" || prefix == "/" {
		if path == "" {
			return "/", true
		}
		return path, true
	}
	if path == prefix || path == prefix+"/" {
		return "/", true
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimPrefix(path, prefix), true
	}
	return "", false
}
