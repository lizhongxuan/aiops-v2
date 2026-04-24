package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"time"
)

func SessionCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     DefaultCookieName,
		Value:    strings.TrimSpace(value),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func ClearingSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     DefaultCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func newSessionToken() string {
	return newToken(24)
}

func newOAuthStateToken() string {
	return newToken(18)
}

func newToken(size int) string {
	if size <= 0 {
		size = 18
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(buf), "=")
}
