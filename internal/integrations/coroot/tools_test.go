package coroot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/tooling"
)

func executeCorootTool(t *testing.T, tool tooling.Tool, input string) map[string]any {
	t.Helper()
	result, err := tool.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s Execute() error = %v", tool.Metadata().Name, err)
	}
	legacyStub := `"message":"coroot tool ` + `executed"`
	if strings.Contains(result.Content, legacyStub) {
		t.Fatalf("%s returned legacy stub content: %s", tool.Metadata().Name, result.Content)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode %s content: %v\n%s", tool.Metadata().Name, err, result.Content)
	}
	return body
}

func newCorootTestTools(t *testing.T, handler http.HandlerFunc) []tooling.Tool {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	client, err := NewClient(ClientConfig{
		BaseURL: upstream.URL,
		Token:   "test-token",
		Project: "prod",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return corootToolsWithClient(client)
}

func TestCorootToolsReturnFixedSchemasFromUpstream(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "unexpected auth: "+got, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[
				{"id":"default:Deployment:checkout","cluster":"prod-a","category":"application","status":"critical","errors":{"status":"critical","value":"2.1%"},"latency":{"status":"warning","value":"450ms"}},
				{"id":"default:Deployment:payments","cluster":"prod-a","category":"application","status":"ok","errors":{"status":"ok","value":"0%"},"latency":{"status":"ok","value":"90ms"}}
			]}}`))
		case "/api/project/prod/overview/map":
			_, _ = w.Write([]byte(`{"data":{"map":[
				{"id":"default:Deployment:checkout","cluster":"prod-a","category":"application","status":"critical","upstreams":[{"id":"default:StatefulSet:postgres","status":"warning","stats":["12 rps","95ms"]}],"downstreams":[{"id":"default:Deployment:frontend","status":"ok"}]},
				{"id":"default:StatefulSet:postgres","cluster":"prod-a","category":"storage","status":"warning","upstreams":[],"downstreams":[{"id":"default:Deployment:checkout","status":"warning"}]},
				{"id":"default:Deployment:frontend","cluster":"prod-a","category":"application","status":"ok","upstreams":[{"id":"default:Deployment:checkout","status":"critical"}],"downstreams":[]}
			]}}`))
		case "/api/project/prod/incident/inc-1":
			_, _ = w.Write([]byte(`{"data":{
				"key":"inc-1",
				"application_id":"default:Deployment:checkout",
				"opened_at":1710000000,
				"resolved_at":1710000300,
				"severity":"critical",
				"short_description":"availability burn",
				"rca":{"status":"OK","short_summary":"DB saturation","root_cause":"Postgres CPU saturation","immediate_fixes":"Scale postgres","detailed_root_cause_analysis":"Postgres CPU saturation increased checkout latency."}
			}}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	slo := executeCorootTool(t, corootToolByName(t, tools, "coroot.slo_status"), `{"project":"prod","service":"checkout"}`)
	if slo["schemaVersion"] == "" || slo["status"] != "ok" {
		t.Fatalf("slo result = %#v, want ok fixed schema", slo)
	}
	slos := slo["slos"].([]any)
	if len(slos) != 2 {
		t.Fatalf("slos len = %d, want 2", len(slos))
	}
	if slos[0].(map[string]any)["name"] != "availability" {
		t.Fatalf("first slo = %#v, want availability", slos[0])
	}
	if slo["rawRef"].(map[string]any)["digest"] == "" {
		t.Fatalf("slo rawRef missing digest: %#v", slo["rawRef"])
	}

	topology := executeCorootTool(t, corootToolByName(t, tools, "coroot.service_topology"), `{"project":"prod","service":"checkout","depth":1}`)
	if topology["status"] != "ok" {
		t.Fatalf("topology status = %#v, want ok", topology["status"])
	}
	if got := len(topology["nodes"].([]any)); got != 3 {
		t.Fatalf("topology nodes len = %d, want 3", got)
	}
	if got := len(topology["edges"].([]any)); got != 2 {
		t.Fatalf("topology edges len = %d, want 2", got)
	}

	timeline := executeCorootTool(t, corootToolByName(t, tools, "coroot.incident_timeline"), `{"project":"prod","incidentId":"inc-1"}`)
	if timeline["incidentId"] != "inc-1" {
		t.Fatalf("timeline incidentId = %#v, want inc-1", timeline["incidentId"])
	}
	if got := len(timeline["events"].([]any)); got < 3 {
		t.Fatalf("timeline events len = %d, want at least 3", got)
	}

	rca := executeCorootTool(t, corootToolByName(t, tools, "coroot.rca_report"), `{"project":"prod","service":"checkout","incidentId":"inc-1"}`)
	if rca["summary"] != "DB saturation" {
		t.Fatalf("rca summary = %#v, want DB saturation", rca["summary"])
	}
	if rca["remediations"] != "Scale postgres" {
		t.Fatalf("rca remediations = %#v, want Scale postgres", rca["remediations"])
	}
	if got := len(rca["relatedServices"].([]any)); got == 0 {
		t.Fatalf("rca relatedServices empty")
	}
}

func TestCorootToolReturnsStructuredError(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.slo_status"), `{"project":"prod","service":"checkout"}`)
	if body["status"] != "error" {
		t.Fatalf("status = %#v, want error", body["status"])
	}
	errPayload := body["error"].(map[string]any)
	if errPayload["kind"] != "upstream_server_error" {
		t.Fatalf("error kind = %#v, want upstream_server_error", errPayload["kind"])
	}
}

func TestCorootToolPromptTellsAgentToProbeInsteadOfAskUser(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"applications":[]}}`))
	})
	tool := corootToolByName(t, tools, "coroot.list_services")
	prompt := tool.Prompt(tooling.PromptContext{SessionType: "host", Mode: "chat", Metadata: tool.Metadata()})
	for _, want := range []string{"aiops.coroot.project", "availability/service probe", "instead of asking the user whether Coroot evidence exists"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
}
