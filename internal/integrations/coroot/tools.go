package coroot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"aiops-v2/internal/tooling"
)

type corootInput struct {
	Project    string   `json:"project,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Status     string   `json:"status,omitempty"`
	Service    string   `json:"service,omitempty"`
	TimeRange  string   `json:"timeRange,omitempty"`
	Metrics    []string `json:"metrics,omitempty"`
	IncidentID string   `json:"incidentId,omitempty"`
	Depth      int      `json:"depth,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	SLOName    string   `json:"sloName,omitempty"`
}

func corootToolsWithClient(client *Client) []tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"chat", "inspect", "plan", "execute"},
	}

	return []tooling.Tool{
		newCorootTool("coroot.list_services", "List monitored services from Coroot with normalized health status", listServicesSchema, visibility, executeListServices(client)),
		newCorootTool("coroot.service_metrics", "Get normalized Coroot metric summaries and native chart widgets for a service", serviceMetricsSchema, visibility, executeServiceMetrics(client)),
		newCorootTool("coroot.rca_report", "Read Coroot RCA summary for a service or incident", rcaReportSchema, visibility, executeRCAReport(client)),
		newCorootTool("coroot.service_topology", "Read Coroot service dependency topology for a service", serviceTopologySchema, visibility, executeServiceTopology(client)),
		newCorootTool("coroot.alert_rules", "List Coroot alert rules as normalized read-only data", alertRulesSchema, visibility, executeAlertRules(client)),
		newCorootTool("coroot.incident_timeline", "Read Coroot incident timeline and RCA milestones", incidentTimelineSchema, visibility, executeIncidentTimeline(client)),
		newCorootTool("coroot.slo_status", "Read current Coroot SLO compliance status for services", sloStatusSchema, visibility, executeSLOStatus(client)),
	}
}

func newCorootTool(name, description string, schema json.RawMessage, visibility tooling.Visibility, execute func(context.Context, json.RawMessage) (any, *CorootRawRef, error)) tooling.Tool {
	meta := corootToolMetadata(name, description)
	return &tooling.StaticTool{
		Meta:             meta,
		Visibility:       visibility,
		InputSchemaData:  schema,
		OutputSchemaData: corootToolOutputSchema,
		PromptFunc: func(ctx tooling.PromptContext) string {
			return "Use the session-bound Coroot project from aiops.coroot.project when present; otherwise omit project so the configured AIOPS_COROOT_PROJECT is used, and do not send default as a placeholder. For ambiguous targets, start with coroot.list_services as a read-only availability/service probe. When the user names a service or a listed service is warning/critical, call coroot.service_metrics to collect metric summaries and native chart/chart_group widgets before final RCA; chartReports render as Agent-to-UI coroot_chart artifacts in chat, so when chartReports are present say the chart card is attached or visible instead of claiming the chat cannot render Coroot-style charts. When root-cause evidence is needed, call coroot.rca_report or coroot.service_topology. If Coroot is unavailable, report that evidence as unavailable and continue with other evidence instead of asking the user whether Coroot evidence exists."
		},
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return false
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			payload, rawRef, err := execute(ctx, input)
			if err != nil {
				payload = corootStructuredError(name, rawRef, err)
			}
			payload = withCorootEnvelopeFields(payload)
			data, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				return tooling.ToolResult{}, marshalErr
			}
			return tooling.ToolResult{
				Content: string(data),
				Display: &tooling.ToolDisplayPayload{
					Type:  "coroot",
					Title: name,
					Data:  data,
				},
			}, nil
		},
	}
}

func corootToolMetadata(name, description string) tooling.ToolMetadata {
	meta := tooling.ToolMetadata{
		Name:        name,
		Description: description,
		Domain:      "coroot",
		RiskLevel:   tooling.ToolRiskLow,
	}
	switch name {
	case "coroot.list_services":
		meta.Layer = tooling.ToolLayerCore
	case "coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "coroot.slo_status":
		meta.Layer = tooling.ToolLayerDeferred
		meta.Pack = "coroot_rca"
		meta.DeferByDefault = true
		meta.Triggers = []string{"RCA", "root cause", "根因", "异常", "warning", "延迟升高", "error rate", "SLO", "topology", "依赖", "CPU", "memory", "内存", "net", "网络", "指标", "图表", "趋势", "时序", "metric", "metrics", "chart", "timeseries"}
	case "coroot.alert_rules", "coroot.incident_timeline":
		meta.Layer = tooling.ToolLayerDeferred
		meta.Pack = "coroot_incident"
		meta.DeferByDefault = true
		meta.Triggers = []string{"incident", "alert", "告警", "事件", "timeline"}
	}
	return meta
}

func withCorootEnvelopeFields(payload any) any {
	data, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return payload
	}
	if _, ok := out["source"]; !ok {
		out["source"] = "coroot"
	}
	if _, ok := out["evidenceRefs"]; !ok {
		out["evidenceRefs"] = []string{}
	}
	return out
}

var corootToolOutputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"project":{"type":"string"},
		"rawRef":{"type":"object"},
		"error":{"type":"object"},
		"evidenceRefs":{"type":"array","items":{"type":"string"}},
		"source":{"type":"string"}
	},
	"required":["schemaVersion","tool","status"]
}`)

func executeListServices(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, applicationsPath(project), url.Values{"query": {in.Namespace}})
		if err != nil {
			return nil, rawRef, err
		}
		var services []ServiceSummary
		for _, app := range objectArray(raw, "applications") {
			service := serviceSummaryFromObject(app)
			if in.Namespace != "" && !strings.Contains(strings.ToLower(service.ID), strings.ToLower(in.Namespace)) {
				continue
			}
			if in.Status != "" && !strings.EqualFold(service.Status, in.Status) {
				continue
			}
			services = append(services, service)
		}
		return ListServicesResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.list_services",
			Status:        "ok",
			Project:       project,
			Services:      services,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeServiceMetrics(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Service) == "" {
			return nil, nil, fmt.Errorf("service is required")
		}
		project := client.ResolveProject(in.Project)
		appID, rawRef, err := resolveApplicationID(ctx, client, project, in.Service)
		if err != nil {
			return nil, rawRef, err
		}
		raw, rawRef, err := getCorootRaw(ctx, client, applicationPath(project, appID), nil)
		if err != nil {
			return nil, rawRef, err
		}
		metrics := metricsFromApplication(raw, in.Metrics)
		chartReports := chartReportsFromApplication(raw)
		return ServiceMetricsResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.service_metrics",
			Status:        "ok",
			Project:       project,
			Service:       appID,
			Metrics:       metrics,
			ChartReports:  chartReports,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeSLOStatus(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, applicationsPath(project), nil)
		if err != nil {
			return nil, rawRef, err
		}
		var slos []SLOStatus
		for _, app := range objectArray(raw, "applications") {
			id := stringFromAny(app["id"])
			if in.Service != "" && !serviceMatches(id, in.Service) {
				continue
			}
			slos = append(slos, sloFromParam("availability", objectField(app, "errors"))...)
			slos = append(slos, sloFromParam("latency", objectField(app, "latency"))...)
		}
		if in.SLOName != "" {
			filtered := slos[:0]
			for _, slo := range slos {
				if strings.EqualFold(slo.Name, in.SLOName) {
					filtered = append(filtered, slo)
				}
			}
			slos = filtered
		}
		return SLOStatusResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.slo_status",
			Status:        "ok",
			Project:       project,
			Service:       in.Service,
			SLOName:       in.SLOName,
			SLOs:          slos,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeServiceTopology(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Service) == "" {
			return nil, nil, fmt.Errorf("service is required")
		}
		project := client.ResolveProject(in.Project)
		depth := in.Depth
		if depth <= 0 {
			depth = 2
		}
		raw, rawRef, err := getCorootRaw(ctx, client, topologyPath(project), nil)
		if err != nil {
			return nil, rawRef, err
		}
		nodes, edges := topologyFromRaw(raw, in.Service, depth)
		return ServiceTopologyResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.service_topology",
			Status:        "ok",
			Project:       project,
			Service:       in.Service,
			Depth:         depth,
			Nodes:         nodes,
			Edges:         edges,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeIncidentTimeline(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.IncidentID) == "" {
			return nil, nil, fmt.Errorf("incidentId is required")
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, incidentPath(project, in.IncidentID), nil)
		if err != nil {
			return nil, rawRef, err
		}
		incident := firstObject(raw)
		events := timelineFromIncident(incident)
		return IncidentTimelineResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.incident_timeline",
			Status:        "ok",
			Project:       project,
			IncidentID:    in.IncidentID,
			Service:       firstNonBlank(in.Service, stringFromAny(incident["application_id"])),
			Events:        events,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeRCAReport(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		var raw json.RawMessage
		var rawRef *CorootRawRef
		if strings.TrimSpace(in.IncidentID) != "" {
			raw, rawRef, err = getCorootRaw(ctx, client, incidentPath(project, in.IncidentID), nil)
		} else {
			if strings.TrimSpace(in.Service) == "" {
				return nil, nil, fmt.Errorf("service is required when incidentId is empty")
			}
			var appID string
			appID, rawRef, err = resolveApplicationID(ctx, client, project, in.Service)
			if err == nil {
				raw, rawRef, err = getCorootRaw(ctx, client, rcaPath(project, appID), url.Values{"withSummary": {"true"}})
			}
		}
		if err != nil {
			return nil, rawRef, err
		}
		report := rcaFromRaw(project, in.Service, in.IncidentID, raw, rawRef)
		return report, rawRef, nil
	}
}

func executeAlertRules(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, alertRulesPath(project), nil)
		if err != nil {
			return nil, rawRef, err
		}
		var rules []AlertRuleSummary
		for _, rule := range objectArray(raw, "rules", "alerting_rules", "items") {
			summary := AlertRuleSummary{
				ID:          firstNonBlank(stringFromAny(rule["id"]), stringFromAny(rule["key"])),
				Name:        firstNonBlank(stringFromAny(rule["name"]), stringFromAny(rule["title"])),
				Severity:    stringFromAny(rule["severity"]),
				Description: firstNonBlank(stringFromAny(rule["description"]), stringFromAny(rule["message"])),
			}
			if in.Severity != "" && !strings.EqualFold(summary.Severity, in.Severity) {
				continue
			}
			rules = append(rules, summary)
		}
		return AlertRulesResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.alert_rules",
			Status:        "ok",
			Project:       project,
			Rules:         rules,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func decodeCorootInput(input json.RawMessage) (corootInput, error) {
	if len(strings.TrimSpace(string(input))) == 0 {
		return corootInput{}, nil
	}
	var in corootInput
	if err := json.Unmarshal(input, &in); err != nil {
		return corootInput{}, fmt.Errorf("invalid coroot input: %w", err)
	}
	return in, nil
}

func getCorootRaw(ctx context.Context, client *Client, apiPath string, query url.Values) (json.RawMessage, *CorootRawRef, error) {
	var raw json.RawMessage
	rawRef, err := client.GetJSON(ctx, apiPath, query, &raw)
	return raw, rawRef, err
}

func corootStructuredError(tool string, rawRef *CorootRawRef, err error) corootErrorResult {
	payload := CorootErrorPayload{Kind: "tool_error", Message: err.Error()}
	if corootErr, ok := err.(*CorootError); ok {
		payload = CorootErrorPayload{
			Kind:       corootErr.Kind,
			StatusCode: corootErr.StatusCode,
			URI:        corootErr.URI,
			Message:    corootErr.Message,
		}
	}
	return corootErrorResult{
		SchemaVersion: corootSchemaVersion,
		Tool:          tool,
		Status:        "error",
		Error:         payload,
		RawRef:        rawRef,
	}
}

func applicationsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/applications"
}

func topologyPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/map"
}

func applicationPath(project, appID string) string {
	return "/api/project/" + url.PathEscape(project) + "/app/" + url.PathEscape(appID)
}

func incidentPath(project, incidentID string) string {
	return "/api/project/" + url.PathEscape(project) + "/incident/" + url.PathEscape(incidentID)
}

func rcaPath(project, appID string) string {
	return applicationPath(project, appID) + "/rca"
}

func alertRulesPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/alerting-rules"
}

func resolveApplicationID(ctx context.Context, client *Client, project, service string) (string, *CorootRawRef, error) {
	raw, rawRef, err := getCorootRaw(ctx, client, applicationsPath(project), nil)
	if err != nil {
		return "", rawRef, err
	}
	for _, app := range objectArray(raw, "applications") {
		id := stringFromAny(app["id"])
		if serviceMatches(id, service) {
			return id, rawRef, nil
		}
	}
	return service, rawRef, nil
}

func serviceSummaryFromObject(obj map[string]any) ServiceSummary {
	id := stringFromAny(obj["id"])
	return ServiceSummary{
		ID:       id,
		Name:     serviceName(id),
		Cluster:  stringFromAny(obj["cluster"]),
		Category: stringFromAny(obj["category"]),
		Status:   stringFromAny(obj["status"]),
	}
}

func metricsFromApplication(raw json.RawMessage, wanted []string) []MetricSummary {
	obj := firstObject(raw)
	appMap := objectField(obj, "app_map")
	app := objectField(appMap, "application")
	candidates := []MetricSummary{
		{Name: "status", Status: stringFromAny(app["status"]), Value: stringFromAny(app["status"])},
	}
	for _, key := range []string{"errors", "latency"} {
		if metric := metricFromParam(key, objectField(app, key)); metric.Name != "" {
			candidates = append(candidates, metric)
		}
	}
	for _, spec := range []struct {
		name       string
		reportName string
	}{
		{name: "cpu", reportName: "CPU"},
		{name: "memory", reportName: "Memory"},
	} {
		if metric := metricFromReport(spec.name, reportByName(obj, spec.reportName)); metric.Name != "" {
			candidates = append(candidates, metric)
			continue
		}
		if metric := metricFromParam(spec.name, objectField(app, spec.name)); metric.Name != "" {
			candidates = append(candidates, metric)
		}
	}
	for _, key := range []string{"instances", "restarts", "upstreams"} {
		if metric := metricFromParam(key, objectField(app, key)); metric.Name != "" {
			candidates = append(candidates, metric)
		}
	}
	if len(wanted) == 0 {
		return candidates
	}
	allowed := map[string]bool{}
	for _, name := range wanted {
		allowed[strings.ToLower(strings.TrimSpace(name))] = true
	}
	var out []MetricSummary
	for _, metric := range candidates {
		if allowed[strings.ToLower(metric.Name)] {
			out = append(out, metric)
		}
	}
	return out
}

func chartReportsFromApplication(raw json.RawMessage) []CorootChartReport {
	obj := firstObject(raw)
	var out []CorootChartReport
	for _, report := range objectSlice(obj["reports"]) {
		var widgets []map[string]any
		for _, widget := range objectSlice(report["widgets"]) {
			if chartWidget, ok := corootChartWidgetFromRaw(widget); ok {
				widgets = append(widgets, chartWidget)
			}
		}
		if len(widgets) == 0 {
			continue
		}
		name := firstNonBlank(stringFromAny(report["name"]), stringFromAny(report["title"]))
		if name == "" {
			name = "Report"
		}
		out = append(out, CorootChartReport{
			Name:    name,
			Status:  stringFromAny(report["status"]),
			Widgets: widgets,
		})
	}
	return out
}

func corootChartWidgetFromRaw(widget map[string]any) (map[string]any, bool) {
	if chart := objectField(widget, "chart"); len(chart) > 0 && corootChartHasData(chart) {
		out := corootWidgetMetadata(widget)
		out["chart"] = cloneCorootMap(chart)
		return out, true
	}
	group := objectField(widget, "chart_group")
	if len(group) == 0 {
		return nil, false
	}
	var charts []any
	for _, chart := range objectSlice(group["charts"]) {
		if corootChartHasData(chart) {
			charts = append(charts, cloneCorootMap(chart))
		}
	}
	if len(charts) == 0 {
		return nil, false
	}
	groupClone := cloneCorootMap(group)
	groupClone["charts"] = charts
	out := corootWidgetMetadata(widget)
	out["chart_group"] = groupClone
	return out, true
}

func corootWidgetMetadata(widget map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"title", "doc_link", "docLink"} {
		if value, ok := widget[key]; ok {
			out[key] = value
		}
	}
	return out
}

func corootChartHasData(chart map[string]any) bool {
	for _, series := range objectSlice(chart["series"]) {
		if corootSeriesDataHasPoints(series["data"]) {
			return true
		}
	}
	if threshold := objectField(chart, "threshold"); len(threshold) > 0 {
		return corootSeriesDataHasPoints(threshold["data"])
	}
	return false
}

func corootSeriesDataHasPoints(value any) bool {
	for _, item := range transportAnySliceForCoroot(value) {
		if item != nil {
			return true
		}
	}
	return false
}

func transportAnySliceForCoroot(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	default:
		return nil
	}
}

func cloneCorootMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func metricFromParam(name string, obj map[string]any) MetricSummary {
	if len(obj) == 0 {
		return MetricSummary{}
	}
	return MetricSummary{Name: name, Status: stringFromAny(obj["status"]), Value: stringFromAny(obj["value"])}
}

func reportByName(obj map[string]any, name string) map[string]any {
	for _, report := range objectSlice(obj["reports"]) {
		if strings.EqualFold(stringFromAny(report["name"]), name) || strings.EqualFold(stringFromAny(report["title"]), name) {
			return report
		}
	}
	return nil
}

func metricFromReport(name string, report map[string]any) MetricSummary {
	if len(report) == 0 {
		return MetricSummary{}
	}
	for _, widget := range objectSlice(report["widgets"]) {
		chart, chartTitle := firstChartFromWidget(widget)
		if len(chart) == 0 {
			continue
		}
		series := metricSeriesFromChart(chart)
		if len(series) == 0 {
			continue
		}
		values := series[0].Values
		return MetricSummary{
			Name:       name,
			Status:     stringFromAny(report["status"]),
			Value:      latestMetricValue(series),
			Unit:       metricUnitFromTitle(chartTitle),
			ChartTitle: chartTitle,
			Values:     values,
			Series:     series,
		}
	}
	return MetricSummary{}
}

func firstChartFromWidget(widget map[string]any) (map[string]any, string) {
	if chart := objectField(widget, "chart"); len(chart) > 0 {
		title := firstNonBlank(stringFromAny(widget["title"]), stringFromAny(chart["title"]))
		return chart, title
	}
	group := objectField(widget, "chart_group")
	if len(group) == 0 {
		return nil, ""
	}
	charts := objectSlice(group["charts"])
	if len(charts) == 0 {
		return nil, ""
	}
	title := firstNonBlank(stringFromAny(group["title"]), stringFromAny(charts[0]["title"]))
	return charts[0], title
}

func metricSeriesFromChart(chart map[string]any) []MetricSeries {
	ctx := objectField(chart, "ctx")
	from := int64FromAny(ctx["from"])
	step := int64FromAny(ctx["step"])
	var out []MetricSeries
	for _, rawSeries := range objectSlice(chart["series"]) {
		values := metricValuesFromData(rawSeries["data"], from, step)
		if len(values) == 0 {
			continue
		}
		out = append(out, MetricSeries{
			Name:   stringFromAny(rawSeries["name"]),
			Value:  stringFromAny(rawSeries["value"]),
			Values: values,
		})
	}
	return out
}

func metricValuesFromData(value any, from int64, step int64) [][]float64 {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([][]float64, 0, len(items))
	for idx, item := range items {
		switch point := item.(type) {
		case []any:
			if len(point) < 2 {
				continue
			}
			ts, okTS := floatFromAny(point[0])
			val, okVal := floatFromAny(point[1])
			if okTS && okVal {
				out = append(out, []float64{ts, val})
			}
		default:
			val, ok := floatFromAny(item)
			if !ok {
				continue
			}
			ts := float64(idx)
			if from != 0 || step != 0 {
				ts = float64(from + int64(idx)*step)
			}
			out = append(out, []float64{ts, val})
		}
	}
	return out
}

func latestMetricValue(series []MetricSeries) string {
	for _, item := range series {
		if strings.TrimSpace(item.Value) != "" {
			return strings.TrimSpace(item.Value)
		}
		if count := len(item.Values); count > 0 && len(item.Values[count-1]) > 1 {
			return strconv.FormatFloat(item.Values[count-1][1], 'f', -1, 64)
		}
	}
	return ""
}

func metricUnitFromTitle(title string) string {
	title = strings.TrimSpace(title)
	if idx := strings.LastIndex(title, ","); idx >= 0 {
		return strings.TrimSpace(title[idx+1:])
	}
	return ""
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed
	default:
		return 0
	}
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func sloFromParam(name string, obj map[string]any) []SLOStatus {
	if len(obj) == 0 {
		return nil
	}
	status := stringFromAny(obj["status"])
	return []SLOStatus{{
		Name:     name,
		Status:   status,
		Value:    stringFromAny(obj["value"]),
		Violated: isProblemStatus(status),
	}}
}

func topologyFromRaw(raw json.RawMessage, service string, depth int) ([]TopologyNode, []TopologyEdge) {
	apps := objectArray(raw, "map", "applications")
	byID := map[string]map[string]any{}
	for _, app := range apps {
		id := stringFromAny(app["id"])
		if id != "" {
			byID[id] = app
		}
	}

	start := ""
	for id := range byID {
		if serviceMatches(id, service) {
			start = id
			break
		}
	}
	if start == "" {
		start = service
	}

	selected := map[string]int{start: 0}
	queue := []string{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		currentDepth := selected[id]
		if currentDepth >= depth {
			continue
		}
		app := byID[id]
		for _, neighbor := range topologyNeighbors(app) {
			if neighbor == "" {
				continue
			}
			if _, seen := selected[neighbor]; seen {
				continue
			}
			selected[neighbor] = currentDepth + 1
			queue = append(queue, neighbor)
		}
	}

	var nodes []TopologyNode
	for id := range selected {
		app := byID[id]
		nodes = append(nodes, TopologyNode{
			ID:       id,
			Name:     serviceName(id),
			Cluster:  stringFromAny(app["cluster"]),
			Category: stringFromAny(app["category"]),
			Status:   stringFromAny(app["status"]),
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	edgeSet := map[string]TopologyEdge{}
	for id := range selected {
		app := byID[id]
		for _, upstream := range objectSlice(app["upstreams"]) {
			target := stringFromAny(upstream["id"])
			if _, ok := selected[target]; !ok {
				continue
			}
			edge := TopologyEdge{Source: id, Target: target, Direction: "upstream", Status: stringFromAny(upstream["status"]), Stats: stringSlice(upstream["stats"])}
			edgeSet[id+"->"+target] = edge
		}
		for _, downstream := range objectSlice(app["downstreams"]) {
			source := stringFromAny(downstream["id"])
			if _, ok := selected[source]; !ok {
				continue
			}
			edge := TopologyEdge{Source: source, Target: id, Direction: "downstream", Status: stringFromAny(downstream["status"]), Stats: stringSlice(downstream["stats"])}
			edgeSet[source+"->"+id] = edge
		}
	}
	var edges []TopologyEdge
	for _, edge := range edgeSet {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source == edges[j].Source {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Source < edges[j].Source
	})
	return nodes, edges
}

func topologyNeighbors(app map[string]any) []string {
	var out []string
	for _, upstream := range objectSlice(app["upstreams"]) {
		out = append(out, stringFromAny(upstream["id"]))
	}
	for _, downstream := range objectSlice(app["downstreams"]) {
		out = append(out, stringFromAny(downstream["id"]))
	}
	return out
}

func timelineFromIncident(incident map[string]any) []TimelineEvent {
	service := stringFromAny(incident["application_id"])
	severity := stringFromAny(incident["severity"])
	var events []TimelineEvent
	if opened := timestampString(incident["opened_at"]); opened != "" {
		events = append(events, TimelineEvent{Type: "incident.opened", Timestamp: opened, Message: firstNonBlank(stringFromAny(incident["short_description"]), "Incident opened"), Severity: severity, Service: service})
	}
	if rca := objectField(incident, "rca"); len(rca) > 0 {
		if summary := firstNonBlank(stringFromAny(rca["short_summary"]), stringFromAny(rca["root_cause"])); summary != "" {
			events = append(events, TimelineEvent{Type: "rca.summary", Timestamp: timestampString(incident["opened_at"]), Message: summary, Severity: severity, Service: service})
		}
		if fixes := stringFromAny(rca["immediate_fixes"]); fixes != "" {
			events = append(events, TimelineEvent{Type: "rca.remediation", Timestamp: timestampString(incident["opened_at"]), Message: fixes, Severity: severity, Service: service})
		}
	}
	if resolved := timestampString(incident["resolved_at"]); resolved != "" && resolved != "0" {
		events = append(events, TimelineEvent{Type: "incident.resolved", Timestamp: resolved, Message: "Incident resolved", Severity: severity, Service: service})
	}
	return events
}

func rcaFromRaw(project, service, incidentID string, raw json.RawMessage, rawRef *CorootRawRef) RCAReportResult {
	obj := firstObject(raw)
	rca := objectField(obj, "rca")
	if len(rca) == 0 {
		rca = obj
	}
	related := uniqueStrings([]string{service, stringFromAny(obj["application_id"])})
	return RCAReportResult{
		SchemaVersion:    corootSchemaVersion,
		Tool:             "coroot.rca_report",
		Status:           "ok",
		Project:          project,
		Service:          firstNonBlank(service, stringFromAny(obj["application_id"])),
		IncidentID:       incidentID,
		Summary:          stringFromAny(rca["short_summary"]),
		RootCause:        stringFromAny(rca["root_cause"]),
		Remediations:     stringFromAny(rca["immediate_fixes"]),
		DetailedAnalysis: stringFromAny(rca["detailed_root_cause_analysis"]),
		RelatedServices:  related,
		RawRef:           rawRef,
	}
}

func objectArray(raw json.RawMessage, keys ...string) []map[string]any {
	root := firstObject(raw)
	for _, key := range keys {
		if arr := objectSlice(root[key]); len(arr) > 0 {
			return arr
		}
	}
	if arr := objectSlice(any(root)); len(arr) > 0 {
		return arr
	}
	return nil
}

func firstObject(raw json.RawMessage) map[string]any {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil && obj != nil {
		return obj
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return arr[0]
	}
	return map[string]any{}
}

func objectField(obj map[string]any, key string) map[string]any {
	if obj == nil {
		return nil
	}
	if m, ok := obj[key].(map[string]any); ok {
		return m
	}
	return nil
}

func objectSlice(value any) []map[string]any {
	switch v := value.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok {
				out = append(out, obj)
			}
		}
		return out
	case []map[string]any:
		return v
	default:
		return nil
	}
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case map[string]any:
		for _, key := range []string{"id", "name", "key"} {
			if s := stringFromAny(v[key]); s != "" {
				return s
			}
		}
		data, _ := json.Marshal(v)
		return string(data)
	default:
		return ""
	}
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringFromAny(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}

func timestampString(value any) string {
	return stringFromAny(value)
}

func serviceMatches(id, service string) bool {
	id = strings.TrimSpace(id)
	service = strings.TrimSpace(service)
	if service == "" {
		return true
	}
	if strings.EqualFold(id, service) || strings.EqualFold(serviceName(id), service) {
		return true
	}
	return strings.Contains(strings.ToLower(id), strings.ToLower(service))
}

func serviceName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, sep := range []string{":", "/"} {
		if idx := strings.LastIndex(id, sep); idx >= 0 && idx < len(id)-1 {
			id = id[idx+1:]
		}
	}
	return id
}

func isProblemStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "warning", "critical", "error", "failed", "degraded":
		return true
	default:
		return false
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
