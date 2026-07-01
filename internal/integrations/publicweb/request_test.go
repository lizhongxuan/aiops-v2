package publicweb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRequestDefaultsToSearch(t *testing.T) {
	req, err := ParseRequest(json.RawMessage(`{
		"query":"PostgreSQL recovery_target_timeline official docs",
		"allowed_domains":["postgresql.org"],
		"limit":0
	}`))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if req.Operation != OperationSearch || req.Query == "" {
		t.Fatalf("request = %+v, want search operation with query", req)
	}
	if req.Limit != 5 || req.MaxContentResults != 2 || req.MaxBytes != 20000 {
		t.Fatalf("defaults = limit:%d maxContent:%d maxBytes:%d", req.Limit, req.MaxContentResults, req.MaxBytes)
	}
	if len(req.AllowedDomains) != 1 || req.AllowedDomains[0] != "postgresql.org" {
		t.Fatalf("allowed domains = %#v", req.AllowedDomains)
	}
}

func TestParseRequestDefaultsToOpenWhenURLPresent(t *testing.T) {
	req, err := ParseRequest(json.RawMessage(`{
		"url":"https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-RECOVERY-TARGET-TIMELINE",
		"max_bytes":12000
	}`))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if req.Operation != OperationOpen || req.URL == "" || req.MaxBytes != 12000 {
		t.Fatalf("request = %+v, want open operation with URL and max bytes", req)
	}
}

func TestParseRequestRejectsInvalidOperationInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"search missing query", `{"operation":"search"}`, "query is required"},
		{"open missing url", `{"operation":"open"}`, "url is required"},
		{"bad operation", `{"operation":"crawl","query":"docs"}`, "operation"},
		{"domain conflict", `{"query":"docs","allowed_domains":["postgresql.org"],"blocked_domains":["postgresql.org"]}`, "cannot both"},
		{"bad domain", `{"query":"docs","allowed_domains":["https://postgresql.org/path"]}`, "hostname without protocol or path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRequest(json.RawMessage(tc.input))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("ParseRequest() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
