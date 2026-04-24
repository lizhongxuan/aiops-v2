package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHTTPTestServerOrSkip(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Skipf("skip test requiring local listener: %v", r)
		}
	}()

	return httptest.NewServer(handler)
}
