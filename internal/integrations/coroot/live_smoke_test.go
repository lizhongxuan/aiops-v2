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
	BaseURL         string `json:"baseUrl"`
	ProductBasePath string `json:"productBasePath"`
	Token           string `json:"token"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Project         string `json:"project"`
	Timeout         string `json:"timeout"`
}

func TestLiveCorootMCPTools(t *testing.T) {
	cfg, client, timeout := newLiveCorootClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout*4)
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
	nodes := liveExecuteTool(t, ctx, tools, "coroot.nodes_overview", map[string]any{"project": projectArg})
	nodeID := chooseLiveNodeID(nodes)
	if nodeID != "" {
		t.Logf("coroot live smoke target node: %s", nodeID)
	}
	dashboardID, panelID := chooseLiveDashboardCandidate(t, ctx, client, projectArg)
	if dashboardID != "" {
		t.Logf("coroot live smoke target dashboard: %s panel=%s", dashboardID, panelID)
	}
	integrations := liveExecuteTool(t, ctx, tools, "coroot.list_integrations", map[string]any{"project": projectArg})
	integrationType := chooseLiveIntegrationType(integrations)
	if integrationType != "" {
		t.Logf("coroot live smoke target integration: %s", integrationType)
	}
	inspections := liveExecuteTool(t, ctx, tools, "coroot.list_inspections", map[string]any{"project": projectArg})
	inspectionType := chooseLiveInspectionType(inspections)
	if inspectionType != "" {
		t.Logf("coroot live smoke target inspection: %s", inspectionType)
	}

	cases := []liveSmokeCase{
		{name: "coroot.list_services", args: map[string]any{"project": projectArg}},
		{name: "coroot.health_check", args: map[string]any{}},
		{name: "coroot.list_projects", args: map[string]any{}},
		{name: "coroot.get_project_status", args: map[string]any{"project": projectArg}},
		{name: "coroot.collect_rca_context", args: map[string]any{"project": projectArg, "service": service, "timeRange": "1h", "depth": 2, "limit": 10}},
		{name: "coroot.service_metrics", args: map[string]any{"project": projectArg, "service": service, "timeRange": "1h", "metrics": []string{"latency", "errors", "throughput", "cpu", "memory"}}},
		{name: "coroot.rca_report", args: map[string]any{"project": projectArg, "service": service}},
		{name: "coroot.service_topology", args: map[string]any{"project": projectArg, "service": service, "depth": 2}},
		{name: "coroot.nodes_overview", args: map[string]any{"project": projectArg}},
		{name: "coroot.traces_overview", args: map[string]any{"project": projectArg}},
		{name: "coroot.deployments_overview", args: map[string]any{"project": projectArg}},
		{name: "coroot.risks_overview", args: map[string]any{"project": projectArg}},
		{name: "coroot.application_logs", args: map[string]any{"project": projectArg, "service": service, "limit": 5}},
		{name: "coroot.application_traces", args: map[string]any{"project": projectArg, "service": service, "limit": 5}},
		{name: "coroot.application_profiling", args: map[string]any{"project": projectArg, "service": service, "limit": 5}},
		{name: "coroot.alert_rules", args: map[string]any{"project": projectArg}},
		{name: "coroot.incidents", args: map[string]any{"project": projectArg, "limit": 20, "showResolved": true}},
		{name: "coroot.slo_status", args: map[string]any{"project": projectArg, "service": service}},
		{name: "coroot.list_dashboards", args: map[string]any{"project": projectArg}},
		{name: "coroot.list_integrations", args: map[string]any{"project": projectArg}},
		{name: "coroot.list_inspections", args: map[string]any{"project": projectArg}},
		{name: "coroot.get_application_categories", args: map[string]any{"project": projectArg}},
		{name: "coroot.get_custom_applications", args: map[string]any{"project": projectArg}},
	}
	if nodeID != "" {
		cases = append(cases, liveSmokeCase{name: "coroot.get_node", args: map[string]any{"project": projectArg, "nodeId": nodeID}})
	} else {
		cases = append(cases, liveSmokeCase{name: "coroot.get_node", skipReason: "no live node found in coroot.nodes_overview"})
	}
	if incidentID != "" {
		cases = append(cases, liveSmokeCase{name: "coroot.incident_timeline", args: map[string]any{"project": projectArg, "incidentId": incidentID}})
	} else {
		cases = append(cases, liveSmokeCase{name: "coroot.incident_timeline", skipReason: "no live incident found in coroot.incidents"})
	}
	if dashboardID != "" {
		cases = append(cases, liveSmokeCase{name: "coroot.get_dashboard", args: map[string]any{"project": projectArg, "dashboardId": dashboardID}})
	} else {
		cases = append(cases, liveSmokeCase{name: "coroot.get_dashboard", skipReason: "no live dashboard found in coroot.list_dashboards"})
	}
	if dashboardID != "" && panelID != "" {
		cases = append(cases, liveSmokeCase{name: "coroot.get_panel_data", args: map[string]any{"project": projectArg, "dashboardId": dashboardID, "panelId": panelID}})
	} else {
		cases = append(cases, liveSmokeCase{name: "coroot.get_panel_data", skipReason: "no live dashboard panel found in Coroot"})
	}
	if integrationType != "" {
		cases = append(cases, liveSmokeCase{name: "coroot.get_integration", args: map[string]any{"project": projectArg, "integrationType": integrationType}})
	} else {
		cases = append(cases, liveSmokeCase{name: "coroot.get_integration", skipReason: "no live integration found in coroot.list_integrations"})
	}
	if inspectionType != "" {
		cases = append(cases, liveSmokeCase{name: "coroot.get_inspection_config", args: map[string]any{"project": projectArg, "service": service, "inspectionType": inspectionType}})
	} else {
		cases = append(cases, liveSmokeCase{name: "coroot.get_inspection_config", skipReason: "no live inspection found in coroot.list_inspections"})
	}
	assertLiveMatrixCoversAllCorootTools(t, tools, cases)

	results := make([]liveSmokeResult, 0, len(cases))
	for _, tc := range cases {
		if strings.TrimSpace(tc.skipReason) != "" {
			results = append(results, liveSmokeResult{Tool: tc.name, Status: "skipped", ErrorBrief: tc.skipReason})
			continue
		}
		body := liveTryExecuteTool(t, ctx, tools, tc.name, tc.args)
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
		if result.Status != "ok" && result.Status != "skipped" {
			failed = append(failed, result)
		}
	}
	if len(failed) > 0 {
		t.Fatalf("Coroot live MCP smoke failed for %d/%d tools: %#v", len(failed), len(results), failed)
	}
}

type liveSmokeCase struct {
	name       string
	args       map[string]any
	skipReason string
}

func assertLiveMatrixCoversAllCorootTools(t *testing.T, tools []tooling.Tool, cases []liveSmokeCase) {
	t.Helper()
	seen := map[string]int{}
	for _, tc := range cases {
		seen[tc.name]++
	}
	var missing []string
	var duplicates []string
	for name, count := range seen {
		if count > 1 {
			duplicates = append(duplicates, name)
		}
	}
	for _, tool := range tools {
		name := tool.Metadata().Name
		if !strings.HasPrefix(name, "coroot.") {
			continue
		}
		if seen[name] == 0 {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(duplicates)
	if len(missing) > 0 || len(duplicates) > 0 {
		t.Fatalf("live Coroot matrix coverage mismatch: missing=%v duplicates=%v", missing, duplicates)
	}
}

func liveTryExecuteTool(t *testing.T, ctx context.Context, tools []tooling.Tool, name string, args map[string]any) map[string]any {
	t.Helper()
	tool := liveToolByName(t, tools, name)
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args for %s: %v", name, err)
	}
	result, err := tool.Execute(ctx, data)
	if err != nil {
		return map[string]any{
			"schemaVersion": corootSchemaVersion,
			"tool":          name,
			"status":        "execute_error",
			"error":         map[string]any{"message": err.Error()},
		}
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		return map[string]any{
			"schemaVersion": corootSchemaVersion,
			"tool":          name,
			"status":        "decode_error",
			"error":         map[string]any{"message": err.Error()},
		}
	}
	if name == "coroot.collect_rca_context" && body["status"] != "error" {
		assertNoLiveUnavailableLimitations(t, body)
		assertNoLiveUnknownDependencyStatuses(t, body)
	}
	return body
}

func chooseLiveNodeID(body map[string]any) string {
	data, _ := body["data"].(map[string]any)
	for _, raw := range anySlice(firstNonNil(data["nodes"], data["items"])) {
		node, _ := raw.(map[string]any)
		if id := firstNonBlank(stringField(node, "id"), stringField(node, "name")); id != "" {
			return id
		}
	}
	return ""
}

func chooseLiveDashboardCandidate(t *testing.T, ctx context.Context, client *Client, project string) (string, string) {
	t.Helper()
	raw, _, err := getCorootRaw(ctx, client, dashboardsPath(project), nil)
	if err != nil {
		t.Logf("unable to discover live dashboards: %v", err)
		return "", ""
	}
	for _, dashboard := range objectArray(raw, "dashboards", "items") {
		dashboardID := firstNonBlank(stringField(dashboard, "id"), stringField(dashboard, "key"), stringField(dashboard, "uid"))
		if dashboardID == "" {
			continue
		}
		panelID := firstLivePanelID(dashboard)
		if panelID == "" {
			if detail, _, err := getCorootRaw(ctx, client, dashboardPath(project, dashboardID), nil); err == nil {
				panelID = firstLivePanelID(firstObject(detail))
			}
		}
		return dashboardID, panelID
	}
	return "", ""
}

func firstLivePanelID(obj map[string]any) string {
	if dashboard := objectField(obj, "dashboard"); len(dashboard) > 0 {
		obj = dashboard
	}
	for _, panel := range objectSlice(firstNonNil(obj["panels"], obj["widgets"])) {
		if id := firstNonBlank(stringField(panel, "id"), stringField(panel, "key"), stringField(panel, "uid")); id != "" {
			return id
		}
	}
	return ""
}

func chooseLiveIntegrationType(body map[string]any) string {
	data, _ := body["data"].(map[string]any)
	for _, raw := range anySlice(firstNonNil(data["integrations"], data["items"])) {
		item, _ := raw.(map[string]any)
		if value := firstNonBlank(stringField(item, "type"), stringField(item, "id"), stringField(item, "name")); value != "" {
			return value
		}
	}
	return firstNonBlankMapKey(data)
}

func chooseLiveInspectionType(body map[string]any) string {
	data, _ := body["data"].(map[string]any)
	for _, raw := range anySlice(firstNonNil(data["checks"], data["inspections"], data["items"])) {
		item, _ := raw.(map[string]any)
		if value := firstNonBlank(stringField(item, "id"), stringField(item, "type"), stringField(item, "name")); value != "" {
			return value
		}
	}
	return firstNonBlankMapKey(data)
}

func firstNonBlankMapKey(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for key, value := range data {
		if strings.TrimSpace(key) == "" || value == nil {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		return key
	}
	return ""
}

func TestLiveCorootRCAExternalDependencyDrilldown(t *testing.T) {
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
	body := liveExecuteTool(t, ctx, tools, "coroot.collect_rca_context", map[string]any{"project": project, "service": service, "timeRange": "1h", "depth": 2, "limit": 10})

	var externalEdge map[string]any
	for _, raw := range anySlice(body["edgeEvidence"]) {
		edge, _ := raw.(map[string]any)
		if stringField(edge, "targetKind") == "external" {
			externalEdge = edge
			break
		}
	}
	if externalEdge == nil {
		t.Skipf("live service %s has no external dependency edge in the current Coroot window", service)
	}
	endpoint := firstNonBlank(stringField(externalEdge, "targetEndpoint"), stringField(externalEdge, "targetName"))
	if endpoint == "" {
		t.Fatalf("external edge missing endpoint: %#v", externalEdge)
	}

	var matchedHypothesis map[string]any
	for _, raw := range anySlice(body["hypotheses"]) {
		hypothesis, _ := raw.(map[string]any)
		if stringField(hypothesis, "rootCauseStatus") == "requires_external_dependency_drilldown" &&
			strings.Contains(stringField(hypothesis, "suspectService"), stringField(externalEdge, "target")) {
			matchedHypothesis = hypothesis
			break
		}
	}
	if matchedHypothesis == nil {
		t.Fatalf("missing external dependency drill-down hypothesis for edge %#v; hypotheses=%#v", externalEdge, body["hypotheses"])
	}
	drilldowns := strings.Join(stringsFromAnySlice(anySlice(matchedHypothesis["nextDrilldowns"])), "\n")
	for _, want := range []string{"resolve external dependency " + endpoint, "port/protocol", "caller-to-dependency network path"} {
		if !strings.Contains(drilldowns, want) {
			t.Fatalf("external nextDrilldowns = %q, want %q", drilldowns, want)
		}
	}
	summary, _ := body["summary"].(map[string]any)
	missingEvidence := strings.Join(stringsFromAnySlice(anySlice(summary["missingEvidence"])), "\n")
	if !strings.Contains(missingEvidence, endpoint) || !strings.Contains(missingEvidence, "underlying cause is unresolved") {
		t.Fatalf("missingEvidence = %q, want unresolved external dependency gap for %s", missingEvidence, endpoint)
	}
	t.Logf("external dependency drill-down verified: service=%s endpoint=%s edge=%s", service, endpoint, stringField(externalEdge, "target"))
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
		BaseURL:         cfg.BaseURL,
		ProductBasePath: cfg.ProductBasePath,
		Token:           cfg.Token,
		Project:         cfg.Project,
		Timeout:         timeout,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if strings.TrimSpace(cfg.Username) != "" || strings.TrimSpace(cfg.Password) != "" {
		if err := client.Login(context.Background(), cfg.Username, cfg.Password); err != nil {
			t.Fatalf("Coroot live login failed: %v", err)
		}
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
	if baseURL := strings.TrimSpace(os.Getenv("COROOT_LIVE_BASE_URL")); baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if project := strings.TrimSpace(os.Getenv("COROOT_LIVE_PROJECT")); project != "" {
		cfg.Project = project
	}
	if productBasePath := strings.TrimSpace(os.Getenv("COROOT_LIVE_PRODUCT_BASE_PATH")); productBasePath != "" {
		cfg.ProductBasePath = productBasePath
	}
	if token := strings.TrimSpace(os.Getenv("COROOT_LIVE_TOKEN")); token != "" {
		cfg.Token = token
	}
	if username := strings.TrimSpace(os.Getenv("COROOT_LIVE_USERNAME")); username != "" {
		cfg.Username = username
	}
	if password := strings.TrimSpace(os.Getenv("COROOT_LIVE_PASSWORD")); password != "" {
		cfg.Password = password
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.Project) == "" {
		t.Fatalf("Coroot live config requires baseUrl and project")
	}
	if strings.TrimSpace(cfg.Token) == "" && (strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "") {
		t.Fatalf("Coroot live config requires token or username/password")
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
	if body["status"] == "error" {
		t.Fatalf("%s returned structured error: %s", name, result.Content)
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
	var candidates []string
	for _, field := range []string{"services", "problemServices", "sampleServices"} {
		for _, raw := range anySlice(body[field]) {
			item, _ := raw.(map[string]any)
			id := stringField(item, "id")
			name := stringField(item, "name")
			if serviceMatches(id, preferred) || strings.EqualFold(name, preferred) {
				return firstNonBlank(id, name)
			}
			candidates = append(candidates, firstNonBlank(id, name))
		}
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

func stringsFromAnySlice(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		value := stringFromAny(item)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func sortedMapKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
