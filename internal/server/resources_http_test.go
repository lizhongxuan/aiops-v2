package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
)

func TestHTTPServer_RegistersResourceRoutes(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "approval audits", method: http.MethodGet, path: "/api/v1/approval-audits", want: http.StatusOK},
		{name: "approval grants", method: http.MethodGet, path: "/api/v1/approval-grants", want: http.StatusOK},
		{name: "capability bindings", method: http.MethodGet, path: "/api/v1/capability-bindings", want: http.StatusOK},
		{name: "ui cards", method: http.MethodGet, path: "/api/v1/ui-cards", want: http.StatusOK},
		{name: "script configs", method: http.MethodGet, path: "/api/v1/script-configs", want: http.StatusOK},
		{name: "lab environments", method: http.MethodGet, path: "/api/v1/lab-environments", want: http.StatusOK},
		{name: "coroot config proxy", method: http.MethodGet, path: "/api/v1/coroot/config", want: http.StatusOK},
		{name: "generator base", method: http.MethodGet, path: "/api/v1/generator/", want: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			resp, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("%s %s status=%d want=%d", tc.method, tc.path, resp.StatusCode, tc.want)
			}
		})
	}
}
