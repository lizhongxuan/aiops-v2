package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"aiops-v2/internal/appui"
)

type authServiceProvider interface {
	appui.HTTPServices
	AuthService() appui.AuthService
}

// NewAuthAPIRouter returns a scoped handler for auth endpoints and delegates
// all other paths to next.
func NewAuthAPIRouter(service appui.AuthService, next http.Handler) http.Handler {
	return &authAPIRouter{service: service, next: next}
}

type authAPIRouter struct {
	service appui.AuthService
	next    http.Handler
}

func (h *authAPIRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch strings.TrimRight(r.URL.Path, "/") {
	case "/api/v1/auth/login":
		h.handleLogin(w, r)
		return
	case "/api/v1/auth/logout":
		h.handleLogout(w, r)
		return
	case "/api/v1/auth/oauth/start":
		h.handleOAuthStart(w, r)
		return
	case "/api/v1/auth/oauth/callback":
		h.handleOAuthCallback(w, r)
		return
	}
	if h.next != nil {
		h.next.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *authAPIRouter) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	svc := h.service
	if svc == nil {
		http.Error(w, "auth service is not configured", http.StatusServiceUnavailable)
		return
	}
	var req appui.AuthLoginRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	resp, err := svc.Login(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setAuthCookie(w, resp.Cookie)
	writeJSON(w, http.StatusOK, resp)
}

func (h *authAPIRouter) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		http.Error(w, "auth service is not configured", http.StatusServiceUnavailable)
		return
	}
	resp, err := h.service.Logout(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setAuthCookie(w, resp.Cookie)
	writeJSON(w, http.StatusOK, resp)
}

func (h *authAPIRouter) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		http.Error(w, "auth service is not configured", http.StatusServiceUnavailable)
		return
	}
	resp, err := h.service.OAuthStart(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setAuthCookie(w, resp.Cookie)
	writeJSON(w, http.StatusOK, resp)
}

func (h *authAPIRouter) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.service == nil {
		http.Error(w, "auth service is not configured", http.StatusServiceUnavailable)
		return
	}
	req := appui.AuthCallbackRequest{
		State: r.URL.Query().Get("state"),
		Code:  r.URL.Query().Get("code"),
		Error: r.URL.Query().Get("error"),
	}
	resp, err := h.service.OAuthCallback(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	setAuthCookie(w, resp.Cookie)
	target := "/?login=" + url.QueryEscape(strings.TrimSpace(resp.Result))
	http.Redirect(w, r, target, http.StatusFound)
}

func setAuthCookie(w http.ResponseWriter, cookie *http.Cookie) {
	if cookie == nil {
		return
	}
	http.SetCookie(w, cookie)
}

func authServiceFromHTTP(ui appui.HTTPServices) (appui.AuthService, bool) {
	if provider, ok := ui.(authServiceProvider); ok {
		return provider.AuthService(), true
	}
	return nil, false
}

func buildAuthRouter(ui appui.HTTPServices, next http.Handler) http.Handler {
	svc, _ := authServiceFromHTTP(ui)
	return NewAuthAPIRouter(svc, next)
}

func withBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
