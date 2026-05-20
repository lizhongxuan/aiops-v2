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

func TestCorootServiceMetricsIncludesNativeChartReports(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[{"id":"prod:default:Deployment:checkout","status":"ok"}]}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout":
			_, _ = w.Write([]byte(`{"data":{
				"app_map":{"application":{"id":"prod:default:Deployment:checkout","status":"ok","indicators":[{"status":"ok","message":"CPU"},{"status":"warning","message":"Memory"}]}},
				"reports":[
					{"name":"CPU","status":"ok","widgets":[{"chart_group":{"title":"CPU usage <selector>, cores","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"container: checkout","series":[{"name":"checkout-1","data":[0.4,0.6],"value":""}]}]}}]},
					{"name":"Memory","status":"warning","widgets":[
						{"chart_group":{"title":"Memory usage <selector>, bytes","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"RSS container: checkout","series":[{"name":"checkout-1","data":[1024,2048],"value":""}]}]}},
						{"table":{"header":["Instance"],"rows":[[{"value":"checkout-1"}]]}}
					]},
					{"name":"Net","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"postgres","data":[0,1],"value":""}]}}]}
				]
			}}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.service_metrics"), `{"project":"prod","service":"checkout","metrics":["cpu","memory"]}`)
	metrics := body["metrics"].([]any)
	if len(metrics) != 2 {
		t.Fatalf("metrics len = %d, want cpu and memory", len(metrics))
	}
	byName := map[string]map[string]any{}
	for _, item := range metrics {
		metric := item.(map[string]any)
		byName[metric["name"].(string)] = metric
	}
	cpu := byName["cpu"]
	if cpu == nil || cpu["chartTitle"] != "CPU usage <selector>, cores" || cpu["unit"] != "cores" {
		t.Fatalf("cpu metric = %#v, want chart title and unit", cpu)
	}
	cpuValues := cpu["values"].([]any)
	if len(cpuValues) != 2 || cpuValues[0].([]any)[0] != float64(1710000000000) || cpuValues[1].([]any)[1] != 0.6 {
		t.Fatalf("cpu values = %#v, want timestamped chart points", cpuValues)
	}
	memory := byName["memory"]
	if memory == nil || memory["chartTitle"] != "Memory usage <selector>, bytes" || memory["unit"] != "bytes" {
		t.Fatalf("memory metric = %#v, want chart title and unit", memory)
	}
	series := memory["series"].([]any)
	if len(series) != 1 || series[0].(map[string]any)["name"] != "checkout-1" {
		t.Fatalf("memory series = %#v, want named chart series", series)
	}
	chartReports := body["chartReports"].([]any)
	if len(chartReports) != 3 {
		t.Fatalf("chartReports len = %d, want CPU, Memory, and Net reports", len(chartReports))
	}
	reportNames := map[string]bool{}
	for _, rawReport := range chartReports {
		report := rawReport.(map[string]any)
		reportNames[report["name"].(string)] = true
		widgets := report["widgets"].([]any)
		if len(widgets) == 0 {
			t.Fatalf("report %#v has no chart widgets", report)
		}
		for _, rawWidget := range widgets {
			widget := rawWidget.(map[string]any)
			if _, hasTable := widget["table"]; hasTable {
				t.Fatalf("chartReports included non-chart widget: %#v", widget)
			}
		}
	}
	for _, name := range []string{"CPU", "Memory", "Net"} {
		if !reportNames[name] {
			t.Fatalf("chartReports missing %s: %#v", name, chartReports)
		}
	}
	chartSummary, ok := body["chartSummary"].(map[string]any)
	if !ok {
		t.Fatalf("chartSummary type = %T, want compact summary map", body["chartSummary"])
	}
	if chartSummary["service"] != "prod:default:Deployment:checkout" {
		t.Fatalf("chartSummary service = %#v, want service id", chartSummary["service"])
	}
	summaryJSON, err := json.Marshal(chartSummary)
	if err != nil {
		t.Fatalf("marshal chartSummary: %v", err)
	}
	if strings.Contains(string(summaryJSON), `"data"`) || strings.Contains(string(summaryJSON), `"values"`) {
		t.Fatalf("chartSummary leaked raw series arrays: %s", summaryJSON)
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
	for _, want := range []string{"aiops.coroot.project", "configured Coroot project", "omit project", "do not send default as a placeholder", "availability/service probe", "call coroot.service_metrics", "call coroot.rca_report", "chartReports render as Agent-to-UI coroot_chart artifacts in chat", "instead of asking the user whether Coroot evidence exists"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
}
