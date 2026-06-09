package coroot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/tooling"
)

func executeCorootTool(t *testing.T, tool tooling.Tool, input string) map[string]any {
	t.Helper()
	result := executeCorootToolResult(t, tool, input)
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode %s content: %v\n%s", tool.Metadata().Name, err, result.Content)
	}
	return body
}

func executeCorootToolResult(t *testing.T, tool tooling.Tool, input string) tooling.ToolResult {
	t.Helper()
	result, err := tool.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s Execute() error = %v", tool.Metadata().Name, err)
	}
	legacyStub := `"message":"coroot tool ` + `executed"`
	if strings.Contains(result.Content, legacyStub) {
		t.Fatalf("%s returned legacy stub content: %s", tool.Metadata().Name, result.Content)
	}
	return result
}

func anySliceStrings(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
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
	dependencies, ok := topology["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("topology dependencies = %#v, want compact dependency summary", topology["dependencies"])
	}
	upstreams := dependencies["upstream"].([]any)
	downstreams := dependencies["downstream"].([]any)
	if len(upstreams) != 1 || upstreams[0].(map[string]any)["name"] != "postgres" {
		t.Fatalf("topology upstreams = %#v, want postgres dependency", upstreams)
	}
	if len(downstreams) != 1 || downstreams[0].(map[string]any)["name"] != "frontend" {
		t.Fatalf("topology downstreams = %#v, want frontend caller", downstreams)
	}
	if _, ok := topology["nodes"]; ok {
		t.Fatalf("model-facing topology leaked raw nodes: %#v", topology["nodes"])
	}
	if _, ok := topology["edges"]; ok {
		t.Fatalf("model-facing topology leaked raw edges: %#v", topology["edges"])
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

func TestCorootServiceMetricsSummarizesModelContentAndKeepsNativeChartsForDisplay(t *testing.T) {
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

	result := executeCorootToolResult(t, corootToolByName(t, tools, "coroot.service_metrics"), `{"project":"prod","service":"checkout","metrics":["cpu","memory"]}`)
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode model content: %v\n%s", err, result.Content)
	}
	if _, ok := body["chartReports"]; ok {
		t.Fatalf("model-facing content leaked chartReports: %s", result.Content)
	}
	modelJSON := result.Content
	for _, leaked := range []string{`"values"`, `"series"`, `"data"`, "1710000000000"} {
		if strings.Contains(modelJSON, leaked) {
			t.Fatalf("model-facing content leaked raw metric payload marker %q: %s", leaked, modelJSON)
		}
	}
	chartSummary, ok := body["chartSummary"].(map[string]any)
	if !ok {
		t.Fatalf("chartSummary type = %T, want compact summary map", body["chartSummary"])
	}
	if chartSummary["service"] != "prod:default:Deployment:checkout" {
		t.Fatalf("chartSummary service = %#v, want service id", chartSummary["service"])
	}
	metricSummaries := body["metricSummaries"].([]any)
	if len(metricSummaries) != 2 {
		t.Fatalf("metricSummaries len = %d, want cpu and memory", len(metricSummaries))
	}
	byName := map[string]map[string]any{}
	for _, item := range metricSummaries {
		metric := item.(map[string]any)
		byName[metric["name"].(string)] = metric
	}
	cpu := byName["cpu"]
	if cpu == nil || cpu["chartTitle"] != "CPU usage <selector>, cores" || cpu["unit"] != "cores" {
		t.Fatalf("cpu metric = %#v, want chart title and unit", cpu)
	}
	memory := byName["memory"]
	if memory == nil || memory["chartTitle"] != "Memory usage <selector>, bytes" || memory["unit"] != "bytes" {
		t.Fatalf("memory metric = %#v, want chart title and unit", memory)
	}

	if result.Display == nil || len(result.Display.Data) == 0 {
		t.Fatalf("display payload missing native chart data")
	}
	var display map[string]any
	if err := json.Unmarshal(result.Display.Data, &display); err != nil {
		t.Fatalf("decode display data: %v\n%s", err, result.Display.Data)
	}
	displayMetrics := display["metrics"].([]any)
	if len(displayMetrics) != 2 {
		t.Fatalf("display metrics len = %d, want raw cpu and memory metrics for charts", len(displayMetrics))
	}
	chartReports := display["chartReports"].([]any)
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
	summaryJSON, err := json.Marshal(chartSummary)
	if err != nil {
		t.Fatalf("marshal chartSummary: %v", err)
	}
	if strings.Contains(string(summaryJSON), `"data"`) || strings.Contains(string(summaryJSON), `"values"`) {
		t.Fatalf("chartSummary leaked raw series arrays: %s", summaryJSON)
	}
}

func TestCorootCollectRCAContextAggregatesUsefulFactsWithoutLeakingRawAPIData(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[
				{"id":"prod:default:Deployment:checkout","cluster":"prod-a","category":"application","status":"warning","errors":{"status":"ok","value":"0%"},"latency":{"status":"warning","value":"850ms"}},
				{"id":"prod:default:StatefulSet:postgres","cluster":"prod-a","category":"storage","status":"critical"}
			]}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout":
			_, _ = w.Write([]byte(`{"data":{
				"app_map":{"application":{"id":"prod:default:Deployment:checkout","status":"warning","errors":{"status":"ok","value":"0%"},"latency":{"status":"warning","value":"850ms"}}},
				"reports":[
					{"name":"CPU","status":"ok","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"CPU usage, cores","series":[{"name":"checkout","data":[0.2,0.3],"value":""}]}}]},
					{"name":"Net","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"postgres","data":[0,5],"value":""}]}}]}
				]
			}}`))
		case "/api/project/prod/overview/map":
			_, _ = w.Write([]byte(`{"data":{"map":[
				{"id":"prod:default:Deployment:checkout","cluster":"prod-a","category":"application","status":"warning","upstreams":[{"id":"prod:default:StatefulSet:postgres","status":"critical","stats":["failed connections","p99 420ms"]}],"downstreams":[{"id":"prod:default:Deployment:frontend","status":"warning","stats":["p99 1.2s"]}]},
				{"id":"prod:default:StatefulSet:postgres","cluster":"prod-a","category":"storage","status":"critical","downstreams":[{"id":"prod:default:Deployment:checkout","status":"critical"}]},
				{"id":"prod:default:Deployment:frontend","cluster":"prod-a","category":"application","status":"warning","upstreams":[{"id":"prod:default:Deployment:checkout","status":"warning"}]}
			]}}`))
		case "/api/project/prod/overview/logs":
			_, _ = w.Write([]byte(`{"data":{"logs":{"entries":[
				{"application_id":"prod:default:Deployment:checkout","application":"checkout","severity":"error","message":"timeout connecting to postgres while serving /checkout","timestamp":1710000300000},
				{"application_id":"prod:default:Deployment:frontend","application":"frontend","severity":"info","message":"request completed","timestamp":1710000200000}
			]}}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout/tracing":
			_, _ = w.Write([]byte(`{"data":{
				"status":"warning",
				"sources":[{"name":"otel","selected":true}],
				"services":[{"name":"checkout","linked":true},{"name":"postgres","linked":true}],
				"spans":[
					{"service":"checkout","trace_id":"trace-1","id":"span-1","name":"GET /checkout","duration":1200,"status":"error","client":"frontend"},
					{"service":"postgres","trace_id":"trace-2","id":"span-2","name":"SELECT cart","duration":80,"status":"ok","client":"checkout"}
				],
				"limit":100
			}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout/profiling":
			_, _ = w.Write([]byte(`{"data":{
				"status":"ok",
				"services":[{"name":"checkout","linked":true},{"name":"unrelated","linked":false}],
				"profiles":[{"type":"cpu","name":"CPU"},{"type":"memory","name":"Memory"}],
				"instances":["checkout-7d9f"]
			}}`))
		case "/api/project/prod/overview/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[
				{"application":{"id":"prod:default:Deployment:checkout","category":"application"},"version":"v2","deployed":"2026-05-22T10:00:00Z","status":"warning","age":"5m","summary":[{"status":"warning","message":"new image rolled out during incident window"}]},
				{"application":{"id":"prod:default:Deployment:payments","category":"application"},"version":"v1","status":"ok"}
			]}}`))
		case "/api/project/prod/incidents":
			if r.URL.Query().Get("limit") != "20" {
				http.Error(w, "unexpected incidents limit: "+r.URL.RawQuery, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"data":[
				{"application_id":"prod:default:Deployment:checkout","key":"inc-checkout","opened_at":1710000000000,"severity":"warning","short_description":"checkout latency","application_category":"application","impact":4.2,"rca":{"status":"AI disabled"}}
			]}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	result := executeCorootToolResult(t, corootToolByName(t, tools, "coroot.collect_rca_context"), `{"project":"prod","service":"checkout","timeRange":"30m","depth":1}`)
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode model content: %v\n%s", err, result.Content)
	}
	if body["schemaVersion"] != "coroot.rca_context/v1" || body["tool"] != "coroot.collect_rca_context" || body["status"] != "ok" {
		t.Fatalf("context header = %#v, want coroot RCA context", body)
	}
	target := body["target"].(map[string]any)
	if target["service"] != "prod:default:Deployment:checkout" || target["serviceName"] != "checkout" || target["status"] != "warning" {
		t.Fatalf("target = %#v, want resolved checkout service", target)
	}
	dependencies := body["dependencies"].(map[string]any)
	upstreams := dependencies["upstream"].([]any)
	if len(upstreams) != 1 || upstreams[0].(map[string]any)["name"] != "postgres" {
		t.Fatalf("upstreams = %#v, want postgres summary", upstreams)
	}
	abnormal := body["abnormalServices"].([]any)
	if len(abnormal) == 0 || abnormal[0].(map[string]any)["name"] != "postgres" {
		t.Fatalf("abnormalServices = %#v, want postgres", abnormal)
	}
	summary := body["summary"].(map[string]any)
	signals := summary["topSignals"].([]any)
	if len(signals) == 0 {
		t.Fatalf("topSignals empty: %#v", summary)
	}
	foundPostgresSignal := false
	for _, raw := range signals {
		if strings.Contains(strings.ToLower(raw.(string)), "postgres") {
			foundPostgresSignal = true
		}
	}
	if !foundPostgresSignal {
		t.Fatalf("topSignals = %#v, want postgres signal", signals)
	}
	incidents := body["recentIncidents"].([]any)
	if len(incidents) != 1 || incidents[0].(map[string]any)["id"] != "i-inc-checkout" {
		t.Fatalf("recentIncidents = %#v, want normalized checkout incident", incidents)
	}
	rawRefs := body["rawRefs"].([]any)
	if len(rawRefs) < 4 {
		t.Fatalf("rawRefs = %#v, want refs for applications, metrics, topology, incidents", rawRefs)
	}
	logSummary := body["logSummary"].(map[string]any)
	if logSummary["errorLikeCount"].(float64) != 1 {
		t.Fatalf("logSummary = %#v, want one error-like checkout log", logSummary)
	}
	traceSummary := body["traceSummary"].(map[string]any)
	if traceSummary["errorSpanCount"].(float64) != 1 || traceSummary["spanCount"].(float64) != 2 {
		t.Fatalf("traceSummary = %#v, want compact trace error counts", traceSummary)
	}
	profilingSummary := body["profilingSummary"].(map[string]any)
	if profilingSummary["profileCount"].(float64) != 2 || profilingSummary["instanceCount"].(float64) != 1 {
		t.Fatalf("profilingSummary = %#v, want profile and instance counts", profilingSummary)
	}
	deployments := body["deploymentEvents"].([]any)
	if len(deployments) != 1 || deployments[0].(map[string]any)["version"] != "v2" {
		t.Fatalf("deploymentEvents = %#v, want filtered checkout deployment", deployments)
	}
	enrichedSignals := summary["topSignals"].([]any)
	for _, want := range []string{"timeout connecting to postgres", "trace errors", "deployment"} {
		found := false
		for _, raw := range enrichedSignals {
			if strings.Contains(strings.ToLower(raw.(string)), want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("topSignals = %#v, want signal containing %q", enrichedSignals, want)
		}
	}
	for _, leaked := range []string{`"chartReports"`, `"widgets"`, `"data"`, "1710000000000"} {
		if strings.Contains(result.Content, leaked) {
			t.Fatalf("model-facing RCA context leaked raw Coroot marker %q: %s", leaked, result.Content)
		}
	}
}

func TestCorootTimeWindowQueryParamsUseUnixMilliseconds(t *testing.T) {
	from := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	in := corootInput{
		FromTimestamp: from.Unix(),
		ToTimestamp:   to.Unix(),
	}

	appValues := applicationQueryParams(in)
	if got, want := appValues.Get("from"), strconv.FormatInt(from.UnixMilli(), 10); got != want {
		t.Fatalf("applicationQueryParams from = %q, want unix milliseconds %q", got, want)
	}
	if got, want := appValues.Get("to"), strconv.FormatInt(to.UnixMilli(), 10); got != want {
		t.Fatalf("applicationQueryParams to = %q, want unix milliseconds %q", got, want)
	}

	windowValues := rcaContextWindowQueryParams(in)
	if got, want := windowValues.Get("from"), strconv.FormatInt(from.UnixMilli(), 10); got != want {
		t.Fatalf("rcaContextWindowQueryParams from = %q, want unix milliseconds %q", got, want)
	}
	if got, want := windowValues.Get("to"), strconv.FormatInt(to.UnixMilli(), 10); got != want {
		t.Fatalf("rcaContextWindowQueryParams to = %q, want unix milliseconds %q", got, want)
	}
}

func TestCorootNativeRCAApplicationNotFoundIsUnavailableForResolvedService(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[{"id":"prod:smecloud:Deployment:mservice","status":"warning"}]}}`))
		case "/api/project/prod/app/prod:smecloud:Deployment:mservice/rca":
			http.Error(w, "Application not found", http.StatusNotFound)
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.rca_report"), `{"project":"prod","service":"mservice"}`)
	if body["status"] != "unavailable" {
		t.Fatalf("rca_report status = %#v, want unavailable for optional native RCA 404; body=%#v", body["status"], body)
	}
	if body["service"] != "prod:smecloud:Deployment:mservice" {
		t.Fatalf("rca_report service = %#v, want resolved Coroot app id", body["service"])
	}
	if !strings.Contains(strings.ToLower(body["summary"].(string)), "native rca") {
		t.Fatalf("rca_report summary = %#v, want native RCA availability limitation", body["summary"])
	}
}

func TestCorootRCAReportPromptTreatsNativeRCAFailureAsOptionalEvidence(t *testing.T) {
	prompt := corootToolPrompt("coroot.rca_report")
	for _, want := range []string{
		"optional native Coroot RCA",
		"404",
		"Application not found",
		"AI disabled",
		"does not prove the service is absent",
		"continue with coroot.collect_rca_context",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("rca_report prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCorootCollectRCAPromptRequiresExternalDependencyDrilldown(t *testing.T) {
	prompt := corootToolPrompt("coroot.collect_rca_context")
	for _, want := range []string{
		"external dependency",
		"ExternalService",
		"not the final root cause",
		"identity",
		"endpoint",
		"network path",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("collect_rca_context prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCorootServiceMetricsPromptPrefersCorootChartsForServiceResourceUsage(t *testing.T) {
	prompt := corootToolPrompt("coroot.service_metrics")
	for _, want := range []string{
		"service/application CPU",
		"memory",
		"resource usage",
		"Do not require the user to say chart",
		"not use exec_command",
		"selected host OS",
		"Agent-to-UI coroot_chart artifacts",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("service_metrics prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCorootServiceMetricsMetadataIncludesChineseResourceTriggers(t *testing.T) {
	meta := corootToolMetadata("coroot.service_metrics", "Get service metrics")
	for _, want := range []string{"CPU占用", "CPU 使用率", "资源占用", "资源使用", "内存使用率", "占用率"} {
		if !slices.Contains(meta.Triggers, want) {
			t.Fatalf("service_metrics triggers = %#v, want %q", meta.Triggers, want)
		}
	}
	if !strings.Contains(meta.SearchHint, "CPU占用") || !strings.Contains(meta.SearchHint, "资源占用") {
		t.Fatalf("service_metrics SearchHint = %q, want Chinese resource usage hints", meta.SearchHint)
	}
}

func TestCorootCollectRCAContextFindsDeepDependencyNetworkIssue(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[
				{"id":"prod:default:Deployment:a","cluster":"prod-a","category":"application","status":"critical","latency":{"status":"critical","value":"2s"}},
				{"id":"prod:default:Deployment:b","cluster":"prod-a","category":"application","status":"ok"},
				{"id":"prod:default:Deployment:c","cluster":"prod-a","category":"application","status":"warning"},
				{"id":"prod:default:Deployment:d","cluster":"prod-a","category":"storage","status":"critical"}
			]}}`))
		case "/api/project/prod/app/prod:default:Deployment:a":
			_, _ = w.Write([]byte(`{"data":{
				"app_map":{"application":{"id":"prod:default:Deployment:a","status":"critical","latency":{"status":"critical","value":"2s"}}},
				"reports":[{"name":"SLO","status":"critical","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Latency","series":[{"name":"slow","data":[1,2],"value":"2s"}]}}]}]
			}}`))
		case "/api/project/prod/overview/map":
			_, _ = w.Write([]byte(`{"data":{"map":[
				{"id":"prod:default:Deployment:a","cluster":"prod-a","category":"application","status":"critical","upstreams":[{"id":"prod:default:Deployment:b","status":"ok","stats":["p99 2s"]}]},
				{"id":"prod:default:Deployment:b","cluster":"prod-a","category":"application","status":"ok","upstreams":[{"id":"prod:default:Deployment:c","status":"ok","stats":["p99 1.8s"]}],"downstreams":[{"id":"prod:default:Deployment:a","status":"ok"}]},
				{"id":"prod:default:Deployment:c","cluster":"prod-a","category":"application","status":"warning","upstreams":[{"id":"prod:default:Deployment:d","status":"critical","stats":["failed connections","connection refused","retransmissions increased"]}],"downstreams":[{"id":"prod:default:Deployment:b","status":"ok"}]},
				{"id":"prod:default:Deployment:d","cluster":"prod-a","category":"storage","status":"critical","downstreams":[{"id":"prod:default:Deployment:c","status":"critical"}]}
			]}}`))
		case "/api/project/prod/incidents":
			_, _ = w.Write([]byte(`{"data":[{"application_id":"prod:default:Deployment:a","key":"a-latency","severity":"critical","short_description":"A latency SLO violation"}]}`))
		case "/api/project/prod/overview/logs":
			_, _ = w.Write([]byte(`{"data":{"logs":{"entries":[
				{"application_id":"prod:default:Deployment:c","severity":"error","message":"connection refused calling d","timestamp":1710000300000}
			]}}}`))
		case "/api/project/prod/app/prod:default:Deployment:a/tracing":
			_, _ = w.Write([]byte(`{"data":{"status":"warning","services":[{"name":"a","linked":true},{"name":"b","linked":true},{"name":"c","linked":true},{"name":"d","linked":true}],"spans":[{"service":"a","client":"frontend","name":"GET /a","duration":2200,"status":"error","trace_id":"t1"},{"service":"c","client":"b","name":"call d","duration":1800,"status":"error","trace_id":"t1"}]}}`))
		case "/api/project/prod/app/prod:default:Deployment:a/profiling":
			_, _ = w.Write([]byte(`{"data":{"status":"ok","services":[{"name":"a","linked":true}]}}`))
		case "/api/project/prod/overview/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[]}}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.collect_rca_context"), `{"project":"prod","service":"a"}`)
	if body["timeWindow"] == nil {
		t.Fatalf("missing timeWindow in RCA v2 body: %#v", body)
	}
	edgeEvidence := body["edgeEvidence"].([]any)
	foundCD := false
	for _, raw := range edgeEvidence {
		edge := raw.(map[string]any)
		if edge["sourceName"] == "c" && edge["targetName"] == "d" {
			foundCD = true
			if edge["connectivity"] != "critical" {
				t.Fatalf("C->D edge = %#v, want critical connectivity", edge)
			}
			if edge["score"].(float64) < 70 {
				t.Fatalf("C->D edge score = %#v, want strong network score", edge)
			}
		}
	}
	if !foundCD {
		t.Fatalf("edgeEvidence = %#v, want C->D failed connection edge", edgeEvidence)
	}
	hypotheses := body["hypotheses"].([]any)
	if len(hypotheses) == 0 {
		t.Fatalf("hypotheses empty: %#v", body)
	}
	top := hypotheses[0].(map[string]any)
	if top["suspectEdge"] != "prod:default:Deployment:c->prod:default:Deployment:d" || top["confidence"] != "high" {
		t.Fatalf("top hypothesis = %#v, want high-confidence C->D root cause", top)
	}
	graph := body["evidenceGraph"].(map[string]any)
	paths := graph["paths"].([]any)
	if len(paths) == 0 {
		t.Fatalf("evidenceGraph paths empty: %#v", graph)
	}
	pathNames := paths[0].(map[string]any)["serviceNames"].([]any)
	for _, want := range []string{"d", "c", "b", "a"} {
		found := false
		for _, rawName := range pathNames {
			if rawName == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("propagation path = %#v, want %s included", pathNames, want)
		}
	}
}

func TestCorootCollectRCAContextMarksExternalDependencyAsUnresolvedRootCause(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[
				{"id":"prod:smecloud:Deployment:mservice","cluster":"prod-a","category":"application","status":"warning","errors":{"status":"ok","value":"0%"},"latency":{"status":"ok","value":"5ms"}}
			]}}`))
		case "/api/project/prod/app/prod:smecloud:Deployment:mservice":
			_, _ = w.Write([]byte(`{"data":{
				"app_map":{"application":{"id":"prod:smecloud:Deployment:mservice","status":"warning","errors":{"status":"ok","value":"0%"},"latency":{"status":"ok","value":"5ms"}}},
				"reports":[{"name":"Network","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"10.43.64.6","data":[0,3],"value":""}]}}]}]
			}}`))
		case "/api/project/prod/overview/map":
			_, _ = w.Write([]byte(`{"data":{"map":[
				{"id":"prod:smecloud:Deployment:mservice","cluster":"prod-a","category":"application","status":"warning","upstreams":[{"id":"external:external:ExternalService:10.43.64.6","status":"warning","stats":["failed connections"]}]},
				{"id":"external:external:ExternalService:10.43.64.6","cluster":"external","category":"external","status":"warning","downstreams":[{"id":"prod:smecloud:Deployment:mservice","status":"warning"}]}
			]}}`))
		case "/api/project/prod/incidents":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/project/prod/overview/logs":
			_, _ = w.Write([]byte(`{"data":{"logs":{"entries":[]}}}`))
		case "/api/project/prod/app/prod:smecloud:Deployment:mservice/tracing":
			_, _ = w.Write([]byte(`{"data":{"status":"ok","services":[{"name":"mservice","linked":true}],"spans":[]}}`))
		case "/api/project/prod/app/prod:smecloud:Deployment:mservice/profiling":
			_, _ = w.Write([]byte(`{"data":{"status":"ok","services":[{"name":"mservice","linked":true}]}}`))
		case "/api/project/prod/overview/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[]}}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.collect_rca_context"), `{"project":"prod","service":"mservice","timeRange":"30m","depth":2}`)
	edgeEvidence := body["edgeEvidence"].([]any)
	if len(edgeEvidence) == 0 {
		t.Fatalf("edgeEvidence empty: %#v", body)
	}
	edge := edgeEvidence[0].(map[string]any)
	if edge["targetKind"] != "external" || edge["targetEndpoint"] != "10.43.64.6" {
		t.Fatalf("external edge fields = %#v, want targetKind external and targetEndpoint 10.43.64.6", edge)
	}

	hypotheses := body["hypotheses"].([]any)
	if len(hypotheses) == 0 {
		t.Fatalf("hypotheses empty: %#v", body)
	}
	top := hypotheses[0].(map[string]any)
	if top["rootCauseStatus"] != "requires_external_dependency_drilldown" {
		t.Fatalf("top hypothesis = %#v, want unresolved external dependency status", top)
	}
	drilldowns := strings.Join(anySliceStrings(top["nextDrilldowns"].([]any)), "\n")
	for _, want := range []string{
		"resolve external dependency 10.43.64.6",
		"Kubernetes Service or Endpoint",
		"port/protocol",
		"caller-to-dependency network path",
	} {
		if !strings.Contains(drilldowns, want) {
			t.Fatalf("nextDrilldowns = %q, want %q", drilldowns, want)
		}
	}

	summary := body["summary"].(map[string]any)
	missing := strings.Join(anySliceStrings(summary["missingEvidence"].([]any)), "\n")
	if !strings.Contains(missing, "external dependency 10.43.64.6") || !strings.Contains(missing, "underlying cause is unresolved") {
		t.Fatalf("missingEvidence = %q, want unresolved external dependency gap", missing)
	}
}

func TestCorootTopologyModelDoesNotExposeUnknownAsHealthStatus(t *testing.T) {
	model := serviceTopologyModelFacingPayload(ServiceTopologyResult{
		SchemaVersion: corootSchemaVersion,
		Tool:          "coroot.service_topology",
		Status:        "ok",
		Project:       "prod",
		Service:       "prod:default:Deployment:mservice",
		Depth:         1,
		Nodes: []TopologyNode{
			{ID: "prod:default:Deployment:mservice", Name: "mservice", Status: "ok"},
			{ID: "prod:default:Deployment:mc", Name: "mc", Status: "ok"},
			{ID: "prod:default:Deployment:web", Name: "web", Status: "ok"},
			{ID: "external:external:ExternalService:external-postgres", Name: "external-postgres", Status: "critical"},
		},
		Edges: []TopologyEdge{
			{Source: "prod:default:Deployment:mservice", Target: "prod:default:Deployment:mc", Direction: "upstream", Status: "unknown"},
			{Source: "prod:default:Deployment:web", Target: "prod:default:Deployment:mservice", Direction: "downstream", Status: "unknown"},
			{Source: "prod:default:Deployment:mservice", Target: "external:external:ExternalService:external-postgres", Direction: "upstream", Status: "ok", Stats: []string{"failed connections"}},
			{Source: "prod:default:Deployment:mservice", Target: "external:external:ExternalService:external:7507", Direction: "upstream", Status: "unknown"},
		},
	})

	upstreamByName := map[string]TopologyDependencySummary{}
	for _, item := range model.Dependencies.Upstream {
		upstreamByName[item.Name] = item
		if item.Status == "unknown" {
			t.Fatalf("upstream dependency exposed unknown status: %#v", item)
		}
	}
	downstreamByName := map[string]TopologyDependencySummary{}
	for _, item := range model.Dependencies.Downstream {
		downstreamByName[item.Name] = item
		if item.Status == "unknown" {
			t.Fatalf("downstream dependency exposed unknown status: %#v", item)
		}
	}
	if upstreamByName["mc"].Status != "ok" {
		t.Fatalf("mc status = %#v, want node ok to replace edge unknown", upstreamByName["mc"].Status)
	}
	if downstreamByName["web"].Status != "ok" {
		t.Fatalf("web status = %#v, want node ok to replace edge unknown", downstreamByName["web"].Status)
	}
	if upstreamByName["external-postgres"].Status != "critical" {
		t.Fatalf("external-postgres status = %#v, want node critical to remain visible", upstreamByName["external-postgres"].Status)
	}
	if upstreamByName["external:7507"].Status != "" {
		t.Fatalf("external:7507 status = %#v, want omitted when only Coroot status is unknown", upstreamByName["external:7507"].Status)
	}
	for _, item := range model.AbnormalServices {
		if item.Status == "unknown" {
			t.Fatalf("abnormalServices should not include unknown-only dependencies: %#v", item)
		}
	}
}

func TestCorootCollectRCAContextCanStartFromIncidentID(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/project/prod/incident/inc-1":
			_, _ = w.Write([]byte(`{"data":{"key":"inc-1","application_id":"prod:default:Deployment:checkout","severity":"critical","short_description":"checkout latency"}}`))
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[{"id":"prod:default:Deployment:checkout","cluster":"prod-a","category":"application","status":"critical","latency":{"status":"critical","value":"2s"}}]}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout":
			_, _ = w.Write([]byte(`{"data":{"app_map":{"application":{"id":"prod:default:Deployment:checkout","status":"critical","latency":{"status":"critical","value":"2s"}}},"reports":[]}}`))
		case "/api/project/prod/overview/map":
			_, _ = w.Write([]byte(`{"data":{"map":[{"id":"prod:default:Deployment:checkout","status":"critical"}]}}`))
		case "/api/project/prod/incidents":
			_, _ = w.Write([]byte(`{"data":[{"application_id":"prod:default:Deployment:checkout","key":"inc-1","severity":"critical","short_description":"checkout latency"}]}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.collect_rca_context"), `{"project":"prod","incidentId":"inc-1"}`)
	target := body["target"].(map[string]any)
	if target["service"] != "prod:default:Deployment:checkout" || target["incidentId"] != "inc-1" {
		t.Fatalf("target = %#v, want incident-derived checkout target", target)
	}
	rawRefs := body["rawRefs"].([]any)
	foundIncidentRef := false
	for _, item := range rawRefs {
		if item.(map[string]any)["purpose"] == "incident" {
			foundIncidentRef = true
		}
	}
	if !foundIncidentRef {
		t.Fatalf("rawRefs = %#v, want incident rawRef", rawRefs)
	}
}

func TestCorootIncidentsListsAndFiltersNormalizedIncidents(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/prod/incidents" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("limit") != "200" {
			http.Error(w, "unexpected limit: "+r.URL.RawQuery, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{
				"application_id":"prod:default:Deployment:checkout",
				"key":"abc123",
				"opened_at":1710000000000,
				"resolved_at":1710000300000,
				"severity":"critical",
				"short_description":"High latency",
				"application_category":"application",
				"duration":300000,
				"impact":7.4,
				"rca":{"status":"OK","root_cause":"Postgres saturation","short_summary":"DB slow"}
			},
			{
				"application_id":"prod:default:Deployment:payments",
				"key":"def456",
				"opened_at":1710000100000,
				"resolved_at":0,
				"severity":"warning",
				"short_description":"Error rate",
				"application_category":"application",
				"duration":60000,
				"impact":1.5,
				"rca":{"status":"AI disabled"}
			}
		]}`))
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.incidents"), `{"project":"prod","limit":2,"service":"checkout","showResolved":true}`)
	if body["tool"] != "coroot.incidents" || body["status"] != "ok" {
		t.Fatalf("result header = %#v, want coroot.incidents ok", body)
	}
	incidents := body["incidents"].([]any)
	if len(incidents) != 1 {
		t.Fatalf("incidents len = %d, want filtered checkout incident", len(incidents))
	}
	incident := incidents[0].(map[string]any)
	if incident["id"] != "i-abc123" || incident["key"] != "abc123" {
		t.Fatalf("incident ids = %#v, want stable Coroot display id and key", incident)
	}
	if incident["application"] != "checkout" || incident["applicationId"] != "prod:default:Deployment:checkout" {
		t.Fatalf("incident application fields = %#v", incident)
	}
	if incident["state"] != "resolved" || incident["severity"] != "critical" {
		t.Fatalf("incident state/severity = %#v", incident)
	}
	if incident["impactedRequestsPercent"] != 7.4 {
		t.Fatalf("impact = %#v, want 7.4", incident["impactedRequestsPercent"])
	}
	if incident["rootCause"] != "Postgres saturation" || incident["rcaStatus"] != "OK" {
		t.Fatalf("incident RCA fields = %#v", incident)
	}
	if body["rawRef"].(map[string]any)["digest"] == "" {
		t.Fatalf("rawRef missing digest: %#v", body["rawRef"])
	}
}

func TestCorootIncidentsFetchesEnoughBeforeFilteringResolvedRecentIncidents(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/project/prod/incidents" {
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
			return
		}
		if r.URL.Query().Get("limit") != "200" {
			http.Error(w, "unexpected limit: "+r.URL.RawQuery, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{
				"application_id":"prod:coroot:Deployment:coroot",
				"key":"critical-monitoring",
				"resolved_at":1710000400000,
				"severity":"critical",
				"short_description":"High latency",
				"application_category":"monitoring",
				"impact":15.9,
				"rca":{"status":"AI disabled"}
			},
			{
				"application_id":"prod:default:Deployment:nginx",
				"key":"warning-app-1",
				"resolved_at":1710000300000,
				"severity":"warning",
				"short_description":"High latency",
				"application_category":"application",
				"impact":7.4,
				"rca":{"status":"AI disabled"}
			},
			{
				"application_id":"prod:default:Deployment:rabbitmq-server",
				"key":"warning-app-2",
				"resolved_at":1710000200000,
				"severity":"warning",
				"short_description":"High latency",
				"application_category":"application",
				"impact":5.6,
				"rca":{"status":"AI disabled"}
			}
		]}`))
	})

	body := executeCorootTool(t, corootToolByName(t, tools, "coroot.incidents"), `{"project":"prod","limit":2,"applicationCategory":"application","severity":"warning","status":"open","showResolved":true}`)
	incidents := body["incidents"].([]any)
	if len(incidents) != 2 {
		t.Fatalf("incidents len = %d, want two resolved warning application incidents after over-fetch/filter", len(incidents))
	}
	first := incidents[0].(map[string]any)
	if first["id"] != "i-warning-app-1" || first["state"] != "resolved" {
		t.Fatalf("first incident = %#v, want resolved application incident preserved by showResolved=true", first)
	}
}

func TestCorootReadOnlyExpansionToolsUseSafeSummariesAndRedaction(t *testing.T) {
	tools := newCorootTestTools(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/user":
			_, _ = w.Write([]byte(`{"data":{"projects":[{"id":"prod","name":"Production","role":"Admin"},{"id":"stage","name":"Staging"}]}}`))
		case "/api/project/prod/status":
			_, _ = w.Write([]byte(`{"data":{"prometheus":{"status":"ok"},"agent":{"status":"warning"},"token":"secret-token"}}`))
		case "/api/project/prod/overview/applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[{"id":"prod:default:Deployment:checkout","status":"warning"}]}}`))
		case "/api/project/prod/overview/nodes":
			_, _ = w.Write([]byte(`{"data":{"nodes":[{"id":"node-a","status":"ok"}]}}`))
		case "/api/project/prod/overview/traces":
			_, _ = w.Write([]byte(`{"data":{"status":"ok","services":[{"name":"checkout"}]}}`))
		case "/api/project/prod/overview/deployments":
			_, _ = w.Write([]byte(`{"data":{"deployments":[{"application":{"id":"prod:default:Deployment:checkout","category":"application"},"version":"v3","status":"warning","summary":[{"status":"warning","message":"rollout in incident window"}]}]}}`))
		case "/api/project/prod/overview/risks":
			_, _ = w.Write([]byte(`{"data":{"risks":[{"application_id":"prod:default:Deployment:checkout","status":"warning"}]}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout/logs":
			if r.URL.Query().Get("severity") != "error" {
				http.Error(w, "unexpected logs query: "+r.URL.RawQuery, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"entries":[{"application_id":"prod:default:Deployment:checkout","severity":"error","message":"timeout connecting to postgres","timestamp":1710000000000},{"application_id":"other","severity":"error","message":"not matched"}]}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout/tracing":
			_, _ = w.Write([]byte(`{"data":{"status":"warning","spans":[{"service":"checkout","trace_id":"trace-1","name":"GET /checkout","duration":1200,"status":"error"},{"service":"postgres","trace_id":"trace-1","name":"SELECT","duration":80,"status":"ok"}],"services":[{"name":"checkout","linked":true}]}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout/profiling":
			_, _ = w.Write([]byte(`{"data":{"status":"ok","profiles":[{"type":"cpu","name":"CPU"}],"instances":["checkout-1"],"services":[{"name":"checkout","linked":true}]}}`))
		case "/api/project/prod/node/node-a":
			_, _ = w.Write([]byte(`{"data":{"node":{"id":"node-a","name":"node-a","status":"warning","cpu":{"status":"warning","value":"92%"},"applications":[{"id":"prod:default:Deployment:checkout"}]}}}`))
		case "/api/project/prod/dashboards":
			_, _ = w.Write([]byte(`{"data":{"dashboards":[{"id":"dash-1","name":"Checkout","panels":[{"id":"p1"}]}]}}`))
		case "/api/project/prod/dashboards/dash-1":
			_, _ = w.Write([]byte(`{"data":{"id":"dash-1","name":"Checkout","panels":[{"id":"p1"},{"id":"p2"}]}}`))
		case "/api/project/prod/panel/data":
			if r.URL.Query().Get("dashboard") != "dash-1" || r.URL.Query().Get("panel") != "p1" {
				http.Error(w, "unexpected panel query: "+r.URL.RawQuery, http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"reports":[{"name":"Latency","status":"warning","widgets":[{"chart":{"title":"Latency, ms","series":[{"name":"p99","data":[1,2,3]}]}}]}]}}`))
		case "/api/project/prod/integrations":
			_, _ = w.Write([]byte(`{"data":{"prometheus":{"status":"ok","token":"secret-token"}}}`))
		case "/api/project/prod/integrations/prometheus":
			_, _ = w.Write([]byte(`{"data":{"url":"http://prometheus","password":"secret-password","status":"ok"}}`))
		case "/api/project/prod/inspections":
			_, _ = w.Write([]byte(`{"data":{"cpu":{"enabled":true}}}`))
		case "/api/project/prod/app/prod:default:Deployment:checkout/inspection/cpu/config":
			_, _ = w.Write([]byte(`{"data":{"threshold":"90%","api_key":"secret-key"}}`))
		case "/api/project/prod/application_categories":
			_, _ = w.Write([]byte(`{"data":{"categories":[{"name":"application","custom_patterns":"default/*"}]}}`))
		case "/api/project/prod/custom_applications":
			_, _ = w.Write([]byte(`{"data":{"applications":[{"name":"checkout","patterns":["checkout-*"]}]}}`))
		default:
			http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
		}
	})

	for _, name := range []string{
		"coroot.health_check",
		"coroot.list_projects",
		"coroot.get_project_status",
		"coroot.nodes_overview",
		"coroot.traces_overview",
		"coroot.deployments_overview",
		"coroot.risks_overview",
		"coroot.get_node",
		"coroot.list_dashboards",
		"coroot.get_dashboard",
		"coroot.get_application_categories",
		"coroot.get_custom_applications",
	} {
		body := executeCorootTool(t, corootToolByName(t, tools, name), readOnlyExpansionInputForTool(name))
		if body["status"] != "ok" {
			t.Fatalf("%s status = %#v, want ok: %#v", name, body["status"], body)
		}
	}

	logs := executeCorootTool(t, corootToolByName(t, tools, "coroot.application_logs"), `{"project":"prod","service":"checkout","severity":"error","limit":5}`)
	logSummary := logs["summary"].(map[string]any)
	if logSummary["errorLikeCount"].(float64) != 1 {
		t.Fatalf("log summary = %#v, want one error-like log", logSummary)
	}

	traces := executeCorootTool(t, corootToolByName(t, tools, "coroot.application_traces"), `{"project":"prod","service":"checkout","limit":5}`)
	traceSummary := traces["summary"].(map[string]any)
	if traceSummary["errorSpanCount"].(float64) != 1 || traceSummary["spanCount"].(float64) != 2 {
		t.Fatalf("trace summary = %#v, want compact error counts", traceSummary)
	}

	profiling := executeCorootTool(t, corootToolByName(t, tools, "coroot.application_profiling"), `{"project":"prod","service":"checkout","limit":5}`)
	profilingSummary := profiling["summary"].(map[string]any)
	if profilingSummary["profileCount"].(float64) != 1 || profilingSummary["instanceCount"].(float64) != 1 {
		t.Fatalf("profiling summary = %#v, want profile and instance counts", profilingSummary)
	}

	panel := executeCorootTool(t, corootToolByName(t, tools, "coroot.get_panel_data"), `{"project":"prod","dashboardId":"dash-1","panelId":"p1"}`)
	chartSummary := panel["chartSummary"].(map[string]any)
	reports := chartSummary["reports"].([]any)
	if len(reports) != 1 || reports[0].(map[string]any)["name"] != "Latency" {
		t.Fatalf("panel chart summary = %#v, want Latency report", chartSummary)
	}

	for _, tc := range []struct {
		name       string
		input      string
		wantRedact bool
	}{
		{name: "coroot.list_integrations", input: `{"project":"prod"}`, wantRedact: true},
		{name: "coroot.get_integration", input: `{"project":"prod","integrationType":"prometheus"}`, wantRedact: true},
		{name: "coroot.list_inspections", input: `{"project":"prod"}`},
		{name: "coroot.get_inspection_config", input: `{"project":"prod","service":"checkout","inspectionType":"cpu"}`, wantRedact: true},
	} {
		result := executeCorootToolResult(t, corootToolByName(t, tools, tc.name), tc.input)
		if strings.Contains(result.Content, "secret-token") || strings.Contains(result.Content, "secret-password") || strings.Contains(result.Content, "secret-key") {
			t.Fatalf("%s leaked secret in model content: %s", tc.name, result.Content)
		}
		if tc.wantRedact && !strings.Contains(result.Content, "[redacted]") {
			t.Fatalf("%s content = %s, want redacted marker", tc.name, result.Content)
		}
	}
}

func readOnlyExpansionInputForTool(name string) string {
	switch name {
	case "coroot.get_node":
		return `{"project":"prod","nodeId":"node-a"}`
	case "coroot.get_dashboard":
		return `{"project":"prod","dashboardId":"dash-1"}`
	default:
		return `{"project":"prod"}`
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
	for _, want := range []string{"aiops.coroot.project", "configured Coroot project", "omit project", "do not send default as a placeholder", "availability/service probe", "tool_search", "reveal the relevant deferred pack", "instead of asking the user whether Coroot evidence exists"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
	for _, hidden := range []string{"coroot.collect_rca_context", "coroot.service_metrics", "coroot.rca_report"} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("prompt = %q, should not name hidden tool %q", prompt, hidden)
		}
	}
}
