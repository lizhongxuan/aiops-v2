//go:build coroot_live

package coroot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/tooling"
)

type liveCorootConfig struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
	Project string `json:"project"`
	Timeout string `json:"timeout"`
}

func TestLiveCorootMCPTools(t *testing.T) {
	cfg, client, timeout := newLiveCorootClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout*2)
	defer cancel()
	tools := corootToolsWithClient(client)

	projectArg := cfg.Project
	list := liveExecuteTool(t, ctx, tools, "coroot.list_services", map[string]any{"project": projectArg})
	service := chooseLiveService(list, "mservice")
	if service == "" {
		t.Fatalf("coroot.list_services returned no services")
	}
	t.Logf("coroot live smoke target service: %s", service)
	incidents := liveExecuteTool(t, ctx, tools, "coroot.incidents", map[string]any{"project": projectArg, "limit": 20, "showResolved": true})
	incidentID := chooseLiveIncidentID(incidents, service)
	if incidentID != "" {
		t.Logf("coroot live smoke target incident: %s", incidentID)
	}

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "coroot.list_services", args: map[string]any{"project": projectArg}},
		{name: "coroot.collect_rca_context", args: map[string]any{"project": projectArg, "service": service, "timeRange": "1h", "depth": 2, "limit": 10}},
		{name: "coroot.service_metrics", args: map[string]any{"project": projectArg, "service": service, "timeRange": "1h", "metrics": []string{"latency", "errors", "throughput", "cpu", "memory"}}},
		{name: "coroot.rca_report", args: map[string]any{"project": projectArg, "service": service}},
		{name: "coroot.service_topology", args: map[string]any{"project": projectArg, "service": service, "depth": 2}},
		{name: "coroot.alert_rules", args: map[string]any{"project": projectArg}},
		{name: "coroot.incidents", args: map[string]any{"project": projectArg, "limit": 20, "showResolved": true}},
		{name: "coroot.slo_status", args: map[string]any{"project": projectArg, "service": service}},
	}
	if incidentID != "" {
		cases = append(cases, struct {
			name string
			args map[string]any
		}{name: "coroot.incident_timeline", args: map[string]any{"project": projectArg, "incidentId": incidentID}})
	} else {
		cases = append(cases, struct {
			name string
			args map[string]any
		}{name: "coroot.incident_timeline", args: map[string]any{"project": projectArg, "incidentId": "__missing_live_incident__"}})
	}

	results := make([]liveSmokeResult, 0, len(cases))
	for _, tc := range cases {
		body := liveExecuteTool(t, ctx, tools, tc.name, tc.args)
		results = append(results, liveSmokeResult{
			Tool:       tc.name,
			Status:     stringField(body, "status"),
			Schema:     stringField(body, "schemaVersion"),
			ErrorKind:  nestedStringField(body, "error", "kind"),
			ErrorURI:   nestedStringField(body, "error", "uri"),
			ErrorBrief: truncateCorootText(nestedStringField(body, "error", "message"), 180),
		})
	}
	for _, result := range results {
		t.Logf("coroot live smoke: tool=%s status=%s schema=%s errorKind=%s error=%s uri=%s", result.Tool, result.Status, result.Schema, result.ErrorKind, result.ErrorBrief, result.ErrorURI)
	}
	var failed []liveSmokeResult
	for _, result := range results {
		if result.Status != "ok" {
			failed = append(failed, result)
		}
	}
	if len(failed) > 0 {
		t.Fatalf("Coroot live MCP smoke failed for %d/%d tools: %#v", len(failed), len(results), failed)
	}
}

func TestLiveCorootRCAInternalCollectors(t *testing.T) {
	cfg, client, timeout := newLiveCorootClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout*2)
	defer cancel()
	tools := corootToolsWithClient(client)

	project := cfg.Project
	list := liveExecuteTool(t, ctx, tools, "coroot.list_services", map[string]any{"project": project})
	service := chooseLiveService(list, "mservice")
	if service == "" {
		t.Fatalf("coroot.list_services returned no services")
	}
	t.Logf("coroot internal collector target service: %s", service)

	cases := []struct {
		name     string
		path     string
		validate func(json.RawMessage)
	}{
		{
			name: "overview_logs",
			path: logsOverviewPath(project),
			validate: func(raw json.RawMessage) {
				summary := logSummaryFromRaw(raw, []string{service}, 5)
				t.Logf("collector=overview_logs total=%d matched=%d errorLike=%d entries=%d apps=%v severities=%v", summary.TotalCount, summary.MatchedCount, summary.ErrorLikeCount, len(summary.Entries), summary.Applications, summary.Severities)
			},
		},
		{
			name: "app_logs",
			path: applicationPath(project, service) + "/logs",
			validate: func(raw json.RawMessage) {
				obj := firstObject(raw)
				t.Logf("collector=app_logs keys=%v", sortedMapKeys(obj))
			},
		},
		{
			name: "app_tracing",
			path: tracingPath(project, service),
			validate: func(raw json.RawMessage) {
				summary := traceSummaryFromRaw(raw, 5)
				t.Logf("collector=app_tracing status=%s spans=%d errors=%d sources=%v linked=%v slowest=%d", summary.Status, summary.SpanCount, summary.ErrorSpanCount, summary.Sources, summary.LinkedServices, len(summary.SlowestSpans))
			},
		},
		{
			name: "app_profiling",
			path: profilingPath(project, service),
			validate: func(raw json.RawMessage) {
				summary := profilingSummaryFromRaw(raw, 8)
				t.Logf("collector=app_profiling status=%s profiles=%d instances=%d linked=%v message=%s", summary.Status, summary.ProfileCount, summary.InstanceCount, summary.LinkedServices, summary.Message)
			},
		},
		{
			name: "overview_deployments",
			path: deploymentsPath(project),
			validate: func(raw json.RawMessage) {
				events := deploymentEventsFromRaw(raw, service, 5)
				t.Logf("collector=overview_deployments matchedEvents=%d", len(events))
			},
		},
	}

	for _, tc := range cases {
		raw, rawRef, err := getCorootRaw(ctx, client, tc.path, nil)
		if err != nil {
			t.Fatalf("collector %s path %s failed: %v", tc.name, tc.path, err)
		}
		if rawRef == nil || rawRef.Digest == "" || rawRef.Bytes <= 0 {
			t.Fatalf("collector %s returned invalid rawRef: %#v", tc.name, rawRef)
		}
		t.Logf("collector=%s path=%s bytes=%d digest=%s", tc.name, tc.path, rawRef.Bytes, rawRef.Digest)
		tc.validate(raw)
	}
}

func newLiveCorootClient(t *testing.T) (liveCorootConfig, *Client, time.Duration) {
	t.Helper()
	cfg := loadLiveCorootConfig(t)
	timeout := 30 * time.Second
	if cfg.Timeout != "" {
		parsed, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			t.Fatalf("invalid Coroot timeout %q: %v", cfg.Timeout, err)
		}
		timeout = parsed
	}
	client, err := NewClient(ClientConfig{
		BaseURL: cfg.BaseURL,
		Token:   cfg.Token,
		Project: cfg.Project,
		Timeout: timeout,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return cfg, client, timeout
}

type liveSmokeResult struct {
	Tool       string
	Status     string
	Schema     string
	ErrorKind  string
	ErrorURI   string
	ErrorBrief string
}

func loadLiveCorootConfig(t *testing.T) liveCorootConfig {
	t.Helper()
	root := findRepoRootForLiveTest(t)
	raw, err := os.ReadFile(filepath.Join(root, ".data", "coroot-config.json"))
	if err != nil {
		t.Fatalf("read .data/coroot-config.json: %v", err)
	}
	var cfg liveCorootConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode .data/coroot-config.json: %v", err)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Project) == "" || strings.TrimSpace(cfg.Token) == "" {
		t.Fatalf("Coroot live config requires baseUrl, project, and token")
	}
	return cfg
}

func findRepoRootForLiveTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("unable to find repo root from %s", dir)
		}
		dir = parent
	}
}

func liveExecuteTool(t *testing.T, ctx context.Context, tools []tooling.Tool, name string, args map[string]any) map[string]any {
	t.Helper()
	tool := liveToolByName(t, tools, name)
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args for %s: %v", name, err)
	}
	result, err := tool.Execute(ctx, data)
	if err != nil {
		t.Fatalf("%s Execute() error = %v", name, err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode %s content: %v\n%s", name, err, result.Content)
	}
	if name == "coroot.collect_rca_context" {
		assertNoLiveUnavailableLimitations(t, body)
		assertNoLiveUnknownDependencyStatuses(t, body)
	}
	return body
}

func assertNoLiveUnavailableLimitations(t *testing.T, body map[string]any) {
	t.Helper()
	var unavailable []string
	for _, raw := range anySlice(body["limitations"]) {
		item := strings.ToLower(stringFromAny(raw))
		if strings.Contains(item, "unavailable") ||
			strings.Contains(item, "upstream_client_error") ||
			strings.Contains(item, "upstream_server_error") ||
			strings.Contains(item, "status=404") {
			unavailable = append(unavailable, stringFromAny(raw))
		}
	}
	if len(unavailable) > 0 {
		t.Fatalf("collect_rca_context has unavailable Coroot sub-collectors: %v", unavailable)
	}
}

func assertNoLiveUnknownDependencyStatuses(t *testing.T, body map[string]any) {
	t.Helper()
	var offenders []string
	dependencies, _ := body["dependencies"].(map[string]any)
	for _, group := range []string{"upstream", "downstream"} {
		for _, raw := range anySlice(dependencies[group]) {
			item, _ := raw.(map[string]any)
			if strings.EqualFold(stringFromAny(item["status"]), "unknown") {
				offenders = append(offenders, group+":"+firstNonBlank(stringFromAny(item["name"]), stringFromAny(item["id"])))
			}
		}
	}
	for _, field := range []string{"relatedServices", "abnormalServices"} {
		for _, raw := range anySlice(body[field]) {
			item, _ := raw.(map[string]any)
			if strings.EqualFold(stringFromAny(item["status"]), "unknown") {
				offenders = append(offenders, field+":"+firstNonBlank(stringFromAny(item["name"]), stringFromAny(item["id"])))
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("collect_rca_context exposed unknown dependency health statuses: %v", offenders)
	}
}

func liveToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %q", name)
	return nil
}

func chooseLiveService(body map[string]any, preferred string) string {
	services, _ := body["services"].([]any)
	var candidates []string
	for _, raw := range services {
		item, _ := raw.(map[string]any)
		id := stringField(item, "id")
		name := stringField(item, "name")
		if serviceMatches(id, preferred) || strings.EqualFold(name, preferred) {
			return firstNonBlank(id, name)
		}
		candidates = append(candidates, firstNonBlank(id, name))
	}
	sort.Strings(candidates)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func chooseLiveIncidentID(body map[string]any, service string) string {
	incidents, _ := body["incidents"].([]any)
	var fallback string
	for _, raw := range incidents {
		item, _ := raw.(map[string]any)
		id := firstNonBlank(stringField(item, "key"), stringField(item, "id"))
		id = strings.TrimPrefix(id, "i-")
		if fallback == "" {
			fallback = id
		}
		if serviceMatches(stringField(item, "applicationId"), service) {
			return id
		}
	}
	return fallback
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	return stringFromAny(obj[key])
}

func nestedStringField(obj map[string]any, path ...string) string {
	var current any = obj
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	return stringFromAny(current)
}

func anySlice(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return nil
}

func sortedMapKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
