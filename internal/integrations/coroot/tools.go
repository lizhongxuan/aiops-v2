package coroot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"aiops-v2/internal/tooling"
)

const corootObservabilityReadonlyPack = "observability-readonly"

type corootInput struct {
	Project             string   `json:"project,omitempty"`
	AppID               string   `json:"appId,omitempty"`
	Namespace           string   `json:"namespace,omitempty"`
	Status              string   `json:"status,omitempty"`
	Service             string   `json:"service,omitempty"`
	TimeRange           string   `json:"timeRange,omitempty"`
	From                string   `json:"from,omitempty"`
	To                  string   `json:"to,omitempty"`
	FromTimestamp       int64    `json:"fromTimestamp,omitempty"`
	ToTimestamp         int64    `json:"toTimestamp,omitempty"`
	Metrics             []string `json:"metrics,omitempty"`
	IncidentID          string   `json:"incidentId,omitempty"`
	TraceID             string   `json:"traceId,omitempty"`
	NodeID              string   `json:"nodeId,omitempty"`
	DashboardID         string   `json:"dashboardId,omitempty"`
	PanelID             string   `json:"panelId,omitempty"`
	IntegrationType     string   `json:"integrationType,omitempty"`
	InspectionType      string   `json:"inspectionType,omitempty"`
	Depth               int      `json:"depth,omitempty"`
	Severity            string   `json:"severity,omitempty"`
	SLOName             string   `json:"sloName,omitempty"`
	Query               string   `json:"query,omitempty"`
	Limit               int      `json:"limit,omitempty"`
	ShowResolved        *bool    `json:"showResolved,omitempty"`
	IncludeRCA          *bool    `json:"includeRca,omitempty"`
	ApplicationCategory string   `json:"applicationCategory,omitempty"`
}

type ClientProvider interface {
	CorootClient(ctx context.Context) (*Client, error)
}

type ClientProviderFunc func(ctx context.Context) (*Client, error)

func (f ClientProviderFunc) CorootClient(ctx context.Context) (*Client, error) {
	return f(ctx)
}

func corootToolsWithClient(client *Client) []tooling.Tool {
	return corootToolsWithClientProvider(ClientProviderFunc(func(context.Context) (*Client, error) {
		return client, nil
	}))
}

func corootToolsWithClientProvider(provider ClientProvider) []tooling.Tool {
	visibility := tooling.Visibility{
		SessionTypes: []string{"host", "workspace"},
		Modes:        []string{"chat", "inspect", "plan", "execute"},
	}

	return []tooling.Tool{
		newCorootTool("coroot.list_services", "List monitored services from Coroot with normalized health status", listServicesSchema, visibility, executeWithCorootClient(provider, executeListServices)),
		newCorootTool("coroot.health_check", "Check whether the configured Coroot endpoint is reachable", healthCheckSchema, visibility, executeWithCorootClient(provider, executeHealthCheck)),
		newCorootTool("coroot.list_projects", "List Coroot projects visible to the configured credentials", emptyCorootSchema, visibility, executeWithCorootClient(provider, executeListProjects)),
		newCorootTool("coroot.get_project_status", "Read Coroot project agent and integration status", projectOnlySchema, visibility, executeWithCorootClient(provider, executeGetProjectStatus)),
		newCorootTool("coroot.collect_rca_context", "Collect an aggregated, model-safe Coroot RCA evidence context for a service or incident", collectRCAContextSchema, visibility, executeWithCorootClient(provider, executeCollectRCAContext)),
		newCorootTool("coroot.service_metrics", "Get normalized Coroot metric summaries and native chart widgets for a service", serviceMetricsSchema, visibility, executeWithCorootClient(provider, executeServiceMetrics)),
		newCorootTool("coroot.rca_report", "Read Coroot RCA summary for a service or incident", rcaReportSchema, visibility, executeWithCorootClient(provider, executeRCAReport)),
		newCorootTool("coroot.service_topology", "Read Coroot service dependency topology for a service", serviceTopologySchema, visibility, executeWithCorootClient(provider, executeServiceTopology)),
		newCorootTool("coroot.nodes_overview", "Read Coroot infrastructure node overview", overviewSchema, visibility, executeWithCorootClient(provider, executeNodesOverview)),
		newCorootTool("coroot.traces_overview", "Read Coroot distributed tracing overview", overviewSchema, visibility, executeWithCorootClient(provider, executeTracesOverview)),
		newCorootTool("coroot.deployments_overview", "Read Coroot deployment overview and recent deployment events", overviewSchema, visibility, executeWithCorootClient(provider, executeDeploymentsOverview)),
		newCorootTool("coroot.risks_overview", "Read Coroot risk overview", overviewSchema, visibility, executeWithCorootClient(provider, executeRisksOverview)),
		newCorootTool("coroot.application_logs", "Read summarized Coroot logs for an application", applicationLogsSchema, visibility, executeWithCorootClient(provider, executeApplicationLogs)),
		newCorootTool("coroot.application_traces", "Read summarized Coroot traces for an application", applicationTracesSchema, visibility, executeWithCorootClient(provider, executeApplicationTraces)),
		newCorootTool("coroot.application_profiling", "Read summarized Coroot profiling data for an application", applicationProfilingSchema, visibility, executeWithCorootClient(provider, executeApplicationProfiling)),
		newCorootTool("coroot.get_node", "Read details for a Coroot infrastructure node", getNodeSchema, visibility, executeWithCorootClient(provider, executeGetNode)),
		newCorootTool("coroot.alert_rules", "List Coroot alert rules as normalized read-only data", alertRulesSchema, visibility, executeWithCorootClient(provider, executeAlertRules)),
		newCorootTool("coroot.incidents", "List Coroot incidents as normalized read-only data", incidentsSchema, visibility, executeWithCorootClient(provider, executeIncidents)),
		newCorootTool("coroot.incident_timeline", "Read Coroot incident timeline and RCA milestones", incidentTimelineSchema, visibility, executeWithCorootClient(provider, executeIncidentTimeline)),
		newCorootTool("coroot.slo_status", "Read current Coroot SLO compliance status for services", sloStatusSchema, visibility, executeWithCorootClient(provider, executeSLOStatus)),
		newCorootTool("coroot.list_dashboards", "List Coroot dashboards", projectOnlySchema, visibility, executeWithCorootClient(provider, executeListDashboards)),
		newCorootTool("coroot.get_dashboard", "Read a Coroot dashboard definition", dashboardSchema, visibility, executeWithCorootClient(provider, executeGetDashboard)),
		newCorootTool("coroot.get_panel_data", "Read summarized data for a Coroot dashboard panel", panelDataSchema, visibility, executeWithCorootClient(provider, executeGetPanelData)),
		newCorootTool("coroot.list_integrations", "List Coroot integration configuration status with secrets redacted", projectOnlySchema, visibility, executeWithCorootClient(provider, executeListIntegrations)),
		newCorootTool("coroot.get_integration", "Read a Coroot integration configuration with secrets redacted", integrationSchema, visibility, executeWithCorootClient(provider, executeGetIntegration)),
		newCorootTool("coroot.list_inspections", "List Coroot inspection configuration with secrets redacted", projectOnlySchema, visibility, executeWithCorootClient(provider, executeListInspections)),
		newCorootTool("coroot.get_inspection_config", "Read a Coroot application inspection configuration", inspectionConfigSchema, visibility, executeWithCorootClient(provider, executeGetInspectionConfig)),
		newCorootTool("coroot.get_application_categories", "Read Coroot application category rules", projectOnlySchema, visibility, executeWithCorootClient(provider, executeGetApplicationCategories)),
		newCorootTool("coroot.get_custom_applications", "Read Coroot custom application definitions", projectOnlySchema, visibility, executeWithCorootClient(provider, executeGetCustomApplications)),
	}
}

func executeWithCorootClient(
	provider ClientProvider,
	build func(*Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error),
) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		if provider == nil {
			return nil, nil, &CorootError{Kind: "not_configured", Message: "coroot client provider is not configured"}
		}
		client, err := provider.CorootClient(ctx)
		if err != nil {
			return nil, nil, err
		}
		return build(client)(ctx, input)
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
			return corootToolPrompt(name)
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
			resultErr := ""
			if err != nil {
				payload = corootStructuredError(name, rawRef, err)
				resultErr = corootToolResultError(name, err)
			}
			modelPayload := withCorootEnvelopeFields(corootModelFacingPayload(payload))
			modelData, marshalErr := json.Marshal(modelPayload)
			if marshalErr != nil {
				return tooling.ToolResult{}, marshalErr
			}
			displayPayload := withCorootEnvelopeFields(payload)
			displayData, marshalErr := json.Marshal(displayPayload)
			if marshalErr != nil {
				return tooling.ToolResult{}, marshalErr
			}
			return tooling.ToolResult{
				Content: string(modelData),
				Error:   resultErr,
				Display: &tooling.ToolDisplayPayload{
					Type:  "coroot",
					Title: name,
					Data:  displayData,
				},
			}, nil
		},
	}
}

func corootToolResultError(toolName string, err error) string {
	errText := strings.TrimSpace(err.Error())
	if errText == "" {
		errText = "coroot tool failed"
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		toolName = "coroot"
	}
	return toolName + " failed: " + errText
}

func corootToolPrompt(name string) string {
	common := "Use the session-bound Coroot project from aiops.coroot.project when present; otherwise omit project so the configured Coroot project is used, and do not send default as a placeholder. If Coroot is unavailable, report that evidence as unavailable and continue with other evidence instead of asking the user whether Coroot evidence exists."
	switch name {
	case "coroot.list_services":
		return common + " Use coroot.list_services as the default read-only availability/service probe and service discovery probe. If the user needs Coroot details that are not visible in the current tool set, call tool_search with a narrow Coroot query such as metrics, logs, traces, topology, incidents, dashboards, project status, or configuration to reveal the relevant deferred pack."
	case "coroot.collect_rca_context":
		return common + " For service RCA, latency/error/SLO/resource/dependency analysis, prefer this aggregate evidence tool first because it summarizes metrics, SLOs, dependencies, recent incidents, logs, traces, profiling, deployments, and rawRefs into a model-safe evidence pack. Use narrower Coroot packs only for follow-up drill-down or when a section is missing. If edgeEvidence, evidenceGraph, dependencies, or hypotheses are present, treat them as primary RCA evidence and do not later claim topology evidence is missing just because a narrower follow-up tool failed. For a dependency chain X->Y->Z, include the chain as X->Y->Z, include the labels edgeEvidence and hypotheses when those fields support the RCA, and calibrate candidates as: Z依赖异常导致X异常, Y传播上游异常, X自身资源或发布异常. State confidence as high confidence only with Coroot edge evidence, and state the safety guardrail read-only Coroot evidence first. If it identifies an external dependency, IP, DNS name, or ExternalService edge, that edge is not the final root cause until its identity, endpoint, owner, port/protocol, and caller-to-dependency network path are investigated."
	case "coroot.service_metrics", "coroot.slo_status":
		return common + " Use these tools whenever service/application health, status, RCA, CPU, memory, network, SLO, or resource usage evidence would benefit from metric details or visual context. Do not require the user to say chart, metrics, CPU, or Agent-to-UI before calling this tool; decide from the task whether metric charts would help the answer. For service/application CPU or resource usage, do not use exec_command, top, ps, or host shell snapshots as substitutes because those commands inspect the selected host OS, not the Coroot service/application. Native chartReports render as Agent-to-UI coroot_chart artifacts in chat, so when chartReports are present say the chart card is attached or visible instead of claiming the chat cannot render Coroot-style charts."
	case "coroot.rca_report":
		return common + " Use this only when the user explicitly asks for Coroot's native RCA report or as reference evidence after aiops has collected its own evidence; do not treat it as the primary RCA source. If the optional native Coroot RCA returns 404, Application not found, or AI disabled after the service was found elsewhere, this means that RCA reference is unavailable and does not prove the service is absent; continue with coroot.collect_rca_context, metrics, topology, logs, traces, and incidents before finalizing."
	case "coroot.alert_rules", "coroot.incidents", "coroot.incident_timeline":
		return common + " When the user asks about incidents, alerts, incident ids, or the Coroot incidents page, call coroot.incidents before finalizing. For recent/latest incident lists, do not set status, severity, or applicationCategory unless the user explicitly asks for that filter."
	default:
		return common
	}
}

func corootToolMetadata(name, description string) tooling.ToolMetadata {
	meta := tooling.ToolMetadata{
		Name:        name,
		Description: description,
		Domain:      "coroot",
		RiskLevel:   tooling.ToolRiskLow,
		Discovery: tooling.ToolDiscoveryMetadata{
			DiscoveryGroup:     "observability",
			ToolPackIDs:        []string{corootObservabilityReadonlyPack},
			RequiresHealthyMCP: true,
			LoadingPolicy:      tooling.ToolLoadingPolicyDeferred,
			PermissionScope:    "read",
			PromptBudgetClass:  "compact",
			SchemaBudgetClass:  "on_demand",
		},
	}
	switch name {
	case "coroot.list_services":
		configureCorootDeferredTool(&meta, "mcp_dynamic_coroot", []string{"observability", "services", "service health", "应用", "服务", "健康状态"})
		setCorootDiscovery(&meta, "observability", []string{"service", "application"}, []string{"list", "read"})
	case "coroot.health_check", "coroot.list_projects", "coroot.get_project_status":
		configureCorootDeferredTool(&meta, "coroot_admin_read", []string{"coroot health", "health check", "project status", "projects", "项目", "项目状态", "agent status", "prometheus status"})
		setCorootDiscovery(&meta, "observability", []string{"observability_platform", "project"}, []string{"read", "list"})
	case "coroot.collect_rca_context":
		configureCorootDeferredTool(&meta, "coroot_rca", []string{"RCA", "root cause", "根因", "异常", "warning", "告警", "延迟升高", "error rate", "SLO", "topology", "依赖", "CPU", "memory", "内存", "net", "网络", "指标", "图表", "趋势", "时序", "metric", "metrics", "chart", "timeseries"})
		setCorootDiscovery(&meta, "observability", []string{"service", "application", "incident", "dependency"}, []string{"read", "summarize"})
	case "coroot.service_metrics", "coroot.slo_status":
		configureCorootDeferredTool(&meta, "coroot_metrics", []string{
			"metric", "metrics", "指标", "图表", "chart", "timeseries", "趋势", "时序", "SLO", "slo status", "service metrics", "latency", "error rate",
			"CPU", "CPU占用", "CPU 占用", "CPU 使用率", "cpu usage", "cpu utilization",
			"memory", "内存", "内存使用", "内存占用", "内存使用率", "memory usage", "memory utilization",
			"network", "网络", "resource usage", "resource utilization", "资源", "资源占用", "资源使用", "占用率", "使用率",
		})
		setCorootDiscovery(&meta, "metrics", []string{"service", "application", "slo", "resource"}, []string{"read", "query"})
	case "coroot.rca_report":
		configureCorootDeferredTool(&meta, "coroot_rca_reference", []string{"coroot rca", "RCA reference", "native RCA", "rca report", "root cause report", "Coroot 根因", "根因报告"})
		setCorootDiscovery(&meta, "observability", []string{"service", "application", "incident"}, []string{"read"})
	case "coroot.service_topology":
		configureCorootDeferredTool(&meta, "coroot_topology", []string{"topology", "service topology", "dependency", "dependencies", "拓扑", "依赖", "依赖图", "服务拓扑"})
		setCorootDiscovery(&meta, "topology", []string{"service", "application", "dependency"}, []string{"read"})
	case "coroot.nodes_overview", "coroot.get_node":
		configureCorootDeferredTool(&meta, "coroot_nodes", []string{"node", "nodes", "host", "hosts", "infrastructure", "infra", "主机", "节点", "机器", "基础设施"})
		setCorootDiscovery(&meta, "observability", []string{"host", "node", "infrastructure"}, []string{"read", "list"})
	case "coroot.traces_overview", "coroot.application_traces":
		configureCorootDeferredTool(&meta, "coroot_traces", []string{"trace", "traces", "tracing", "span", "spans", "distributed tracing", "链路", "调用链", "trace id"})
		setCorootDiscovery(&meta, "traces", []string{"service", "application", "trace"}, []string{"read", "query"})
	case "coroot.deployments_overview":
		configureCorootDeferredTool(&meta, "coroot_deployments", []string{"deployment", "deployments", "deploy", "release", "rollout", "rollback", "发布", "部署", "变更"})
		setCorootDiscovery(&meta, "observability", []string{"deployment", "change", "service"}, []string{"read", "list"})
	case "coroot.risks_overview":
		configureCorootDeferredTool(&meta, "coroot_risks", []string{"risk", "risks", "security risk", "availability risk", "风险", "隐患"})
		setCorootDiscovery(&meta, "observability", []string{"risk", "service", "application"}, []string{"read", "list"})
	case "coroot.application_logs":
		configureCorootDeferredTool(&meta, "coroot_logs", []string{"log", "logs", "logging", "error log", "日志", "错误日志"})
		setCorootDiscovery(&meta, "logs", []string{"service", "application", "log"}, []string{"read", "query"})
	case "coroot.application_profiling":
		configureCorootDeferredTool(&meta, "coroot_profiling", []string{"profile", "profiling", "flamegraph", "CPU profile", "memory profile", "pprof", "火焰图", "性能剖析"})
		setCorootDiscovery(&meta, "profiling", []string{"service", "application", "profile"}, []string{"read", "query"})
	case "coroot.alert_rules", "coroot.incidents", "coroot.incident_timeline":
		configureCorootDeferredTool(&meta, "coroot_incident", []string{"incident", "incidents", "alert", "alerts", "告警", "事件", "事故", "timeline", "时间线"})
		setCorootDiscovery(&meta, "incidents", []string{"incident", "alert", "service", "application"}, []string{"read", "list"})
	case "coroot.list_dashboards", "coroot.get_dashboard", "coroot.get_panel_data":
		configureCorootDeferredTool(&meta, "coroot_dashboard", []string{"dashboard", "dashboards", "panel", "chart", "coroot panel", "仪表盘", "看板", "面板"})
		setCorootDiscovery(&meta, "metrics", []string{"dashboard", "panel", "chart"}, []string{"read", "query"})
	case "coroot.list_integrations", "coroot.get_integration", "coroot.list_inspections", "coroot.get_inspection_config", "coroot.get_application_categories", "coroot.get_custom_applications":
		configureCorootDeferredTool(&meta, "coroot_config_read", []string{"integration", "integrations", "inspection", "inspections", "configuration", "config", "category", "custom application", "集成", "巡检", "配置", "应用分类", "自定义应用"})
		setCorootDiscovery(&meta, "observability", []string{"configuration", "integration", "inspection"}, []string{"read", "list"})
	}
	return meta
}

func configureCorootDeferredTool(meta *tooling.ToolMetadata, pack string, triggers []string) {
	meta.Layer = tooling.ToolLayerDeferred
	meta.Pack = pack
	meta.DeferByDefault = true
	meta.Triggers = append([]string(nil), triggers...)
	meta.SearchHint = strings.Join(triggers, " ")
	meta.Discovery.ToolPackIDs = append(meta.Discovery.ToolPackIDs, corootObservabilityReadonlyPack, pack)
	meta.Discovery.LoadingPolicy = tooling.ToolLoadingPolicyDeferred
	meta.Discovery.RequiresHealthyMCP = true
	meta.Discovery.PermissionScope = "read"
	meta.Discovery.PromptBudgetClass = "compact"
	meta.Discovery.SchemaBudgetClass = "on_demand"
	meta.Discovery.DiscoveryTags = append(meta.Discovery.DiscoveryTags, triggers...)
}

func setCorootDiscovery(meta *tooling.ToolMetadata, capability string, resources []string, operations []string) {
	meta.Discovery.CapabilityKind = capability
	meta.Discovery.ResourceTypes = append(meta.Discovery.ResourceTypes, resources...)
	meta.Discovery.OperationKinds = append(meta.Discovery.OperationKinds, operations...)
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

func corootModelFacingPayload(payload any) any {
	switch typed := payload.(type) {
	case ServiceMetricsResult:
		return serviceMetricsModelFacingPayload(typed)
	case ServiceTopologyResult:
		return serviceTopologyModelFacingPayload(typed)
	case RCAContextResult:
		return withCorootEvidencePack(typed, typed.Project)
	case corootErrorResult:
		return withCorootEvidencePack(typed, "")
	default:
		return payload
	}
}

func withCorootEvidencePack(payload any, project string) any {
	data, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return payload
	}
	out["observability_evidence"] = MapCorootEvidencePack(project, payload)
	return out
}

func serviceMetricsModelFacingPayload(result ServiceMetricsResult) ServiceMetricsModelResult {
	summary := result.ChartSummary
	if len(summary.MetricSummaries) == 0 && len(result.Metrics) > 0 {
		summary = chartSummaryFromServiceMetrics(result.Service, result.Metrics, result.ChartReports)
	}
	return ServiceMetricsModelResult{
		SchemaVersion:   result.SchemaVersion,
		Tool:            result.Tool,
		Status:          result.Status,
		Project:         result.Project,
		Service:         result.Service,
		MetricSummaries: append([]CorootMetricChartSummary(nil), summary.MetricSummaries...),
		ChartSummary:    summary,
		RawRef:          result.RawRef,
	}
}

const maxCorootModelTopologyItems = 24

func serviceTopologyModelFacingPayload(result ServiceTopologyResult) ServiceTopologyModelResult {
	centerID := topologyCenterID(result.Service, result.Nodes)
	nodeByID := map[string]TopologyNode{}
	for _, node := range result.Nodes {
		if node.ID != "" {
			nodeByID[node.ID] = node
		}
	}
	upstream, downstream := directTopologyDependencies(centerID, nodeByID, result.Edges)
	related, relatedTruncated := relatedTopologyDependencies(centerID, nodeByID, upstream, downstream)
	abnormal := abnormalTopologyDependencies(related)
	return ServiceTopologyModelResult{
		SchemaVersion:    result.SchemaVersion,
		Tool:             result.Tool,
		Status:           result.Status,
		Project:          result.Project,
		Service:          firstNonBlank(centerID, result.Service),
		ServiceName:      serviceName(firstNonBlank(centerID, result.Service)),
		Depth:            result.Depth,
		NodeCount:        len(result.Nodes),
		EdgeCount:        len(result.Edges),
		Dependencies:     TopologyDependencyGroups{Upstream: upstream, Downstream: downstream},
		RelatedServices:  related,
		AbnormalServices: abnormal,
		Truncated:        relatedTruncated,
		RawRef:           result.RawRef,
	}
}

func topologyCenterID(service string, nodes []TopologyNode) string {
	service = strings.TrimSpace(service)
	for _, node := range nodes {
		if serviceMatches(node.ID, service) {
			return node.ID
		}
	}
	return service
}

func directTopologyDependencies(centerID string, nodeByID map[string]TopologyNode, edges []TopologyEdge) ([]TopologyDependencySummary, []TopologyDependencySummary) {
	var upstream []TopologyDependencySummary
	var downstream []TopologyDependencySummary
	seenUpstream := map[string]struct{}{}
	seenDownstream := map[string]struct{}{}
	for _, edge := range edges {
		switch {
		case edge.Source == centerID:
			item := topologyDependencyFromNode(nodeByID[edge.Target], edge.Target, edge.Direction, edge.Status, edge.Stats)
			upstream = appendUniqueTopologyDependency(upstream, seenUpstream, item)
		case edge.Target == centerID:
			item := topologyDependencyFromNode(nodeByID[edge.Source], edge.Source, edge.Direction, edge.Status, edge.Stats)
			downstream = appendUniqueTopologyDependency(downstream, seenDownstream, item)
		}
		if len(upstream) >= maxCorootModelTopologyItems && len(downstream) >= maxCorootModelTopologyItems {
			break
		}
	}
	return upstream, downstream
}

func relatedTopologyDependencies(centerID string, nodeByID map[string]TopologyNode, groups ...[]TopologyDependencySummary) ([]TopologyDependencySummary, bool) {
	selected := map[string]struct{}{centerID: {}}
	for _, group := range groups {
		for _, item := range group {
			selected[item.ID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(nodeByID))
	for id := range nodeByID {
		if _, used := selected[id]; used {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]TopologyDependencySummary, 0, minInt(len(ids), maxCorootModelTopologyItems))
	for _, id := range ids {
		if len(out) >= maxCorootModelTopologyItems {
			return out, true
		}
		out = append(out, topologyDependencyFromNode(nodeByID[id], id, "", "", nil))
	}
	return out, false
}

func abnormalTopologyDependencies(items []TopologyDependencySummary) []TopologyDependencySummary {
	var out []TopologyDependencySummary
	for _, item := range items {
		if isProblemStatus(item.Status) {
			out = append(out, item)
		}
	}
	return out
}

func topologyDependencyFromNode(node TopologyNode, fallbackID, direction, edgeStatus string, stats []string) TopologyDependencySummary {
	id := firstNonBlank(node.ID, fallbackID)
	status := topologyDependencyStatus(edgeStatus, node.Status)
	return TopologyDependencySummary{
		ID:        id,
		Name:      firstNonBlank(node.Name, serviceName(id)),
		Cluster:   node.Cluster,
		Category:  node.Category,
		Status:    status,
		Direction: strings.TrimSpace(direction),
		Stats:     append([]string(nil), stats...),
	}
}

func topologyDependencyStatus(edgeStatus, nodeStatus string) string {
	edge := normalizedCorootHealthStatus(edgeStatus)
	node := normalizedCorootHealthStatus(nodeStatus)
	if isProblemStatus(edge) {
		return edge
	}
	if isProblemStatus(node) {
		return node
	}
	if edge != "" {
		return edge
	}
	return node
}

func normalizedCorootHealthStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "unknown", "undefined", "null", "none", "n/a", "na", "no data", "unavailable":
		return ""
	default:
		return status
	}
}

func appendUniqueTopologyDependency(items []TopologyDependencySummary, seen map[string]struct{}, item TopologyDependencySummary) []TopologyDependencySummary {
	if item.ID == "" {
		return items
	}
	if len(items) >= maxCorootModelTopologyItems {
		return items
	}
	if _, ok := seen[item.ID]; ok {
		return items
	}
	seen[item.ID] = struct{}{}
	return append(items, item)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func executeHealthCheck(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		if _, err := decodeCorootInput(input); err != nil {
			return nil, nil, err
		}
		rawRef, err := client.GetJSON(ctx, healthPath(), nil, nil)
		if err != nil {
			return nil, rawRef, err
		}
		return HealthCheckResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.health_check",
			Status:        "ok",
			Healthy:       true,
			Message:       "Coroot server is reachable",
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeListProjects(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		if _, err := decodeCorootInput(input); err != nil {
			return nil, nil, err
		}
		raw, rawRef, err := getCorootRaw(ctx, client, userPath(), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return ProjectsResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.list_projects",
			Status:        "ok",
			Projects:      projectSummariesFromRaw(raw),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeGetProjectStatus(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, projectStatusPath(project), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return GenericCorootDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.get_project_status",
			Status:        "ok",
			Project:       project,
			Data:          redactCorootSecrets(firstNonNil(rawDataMap(raw), firstObject(raw))),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

const rcaContextSchemaVersion = "coroot.rca_context/v1"

func executeCollectRCAContext(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		targetServiceInput := strings.TrimSpace(in.Service)
		incidentID := strings.TrimSpace(in.IncidentID)
		if targetServiceInput == "" && incidentID == "" {
			return nil, nil, fmt.Errorf("service or incidentId is required")
		}
		project := client.ResolveProject(in.Project)
		depth := normalizedTopologyDepth(in.Depth)
		incidentLimit := normalizedRCAContextIncidentLimit(in.Limit)

		rawRefs := make([]CorootRawRefSummary, 0, 4)
		limitations := make([]string, 0, 3)
		var incidentFromTarget *IncidentSummary
		var incidentObj map[string]any

		if incidentID != "" {
			incidentRaw, incidentRef, incidentErr := getCorootRaw(ctx, client, incidentPath(project, incidentID), nil)
			if incidentErr != nil {
				return nil, incidentRef, incidentErr
			}
			rawRefs = appendCorootRawRefSummary(rawRefs, "incident", incidentRef)
			incidentObj = firstObject(incidentRaw)
			incident := incidentSummaryFromObject(incidentObj)
			incidentFromTarget = &incident
			if targetServiceInput == "" {
				targetServiceInput = incident.ApplicationID
			}
		}
		if targetServiceInput == "" {
			return nil, nil, fmt.Errorf("service could not be resolved from incidentId")
		}
		timeWindow := rcaContextTimeWindow(in, incidentObj, time.Now().UTC())
		windowedInput := in
		applyRCAContextTimeWindowToInput(&windowedInput, timeWindow)

		applicationsRaw, applicationsRef, err := getCorootRaw(ctx, client, applicationsPath(project), nil)
		if err != nil {
			return nil, applicationsRef, err
		}
		rawRefs = appendCorootRawRefSummary(rawRefs, "applications", applicationsRef)
		targetService := serviceSummaryFromRaw(applicationsRaw, targetServiceInput)
		appID := firstNonBlank(targetService.ID, targetServiceInput)

		appRaw, appRef, err := getCorootRaw(ctx, client, applicationPath(project, appID), applicationQueryParams(windowedInput))
		if err != nil {
			return nil, appRef, err
		}
		rawRefs = appendCorootRawRefSummary(rawRefs, "service_metrics", appRef)
		metrics := metricsFromApplication(appRaw, in.Metrics)
		chartSummary := chartSummaryFromServiceMetrics(appID, metrics, chartReportsFromApplication(appRaw))

		slos := slosFromApplicationsRaw(applicationsRaw, appID, "")
		topologySummary := ServiceTopologyModelResult{
			Service:     appID,
			ServiceName: serviceName(appID),
		}
		var topologyNodes []TopologyNode
		var topologyEdges []TopologyEdge
		if topologyRaw, topologyRef, topologyErr := getCorootRaw(ctx, client, topologyPath(project), rcaContextWindowQueryParams(windowedInput)); topologyErr != nil {
			limitations = append(limitations, "Coroot topology is unavailable: "+topologyErr.Error())
		} else {
			rawRefs = appendCorootRawRefSummary(rawRefs, "topology", topologyRef)
			nodes, edges := topologyFromRaw(topologyRaw, appID, depth)
			topologyNodes = nodes
			topologyEdges = edges
			topologySummary = serviceTopologyModelFacingPayload(ServiceTopologyResult{
				SchemaVersion: corootSchemaVersion,
				Tool:          "coroot.service_topology",
				Status:        "ok",
				Project:       project,
				Service:       appID,
				Depth:         depth,
				Nodes:         nodes,
				Edges:         edges,
				RawRef:        topologyRef,
			})
		}
		edgeEvidence := rcaEdgeEvidenceFromTopology(appID, topologyNodes, topologyEdges)
		abnormalEdgeServices := rcaEdgeEvidenceDependencies(edgeEvidence, topologyNodes)
		evidenceGraph := rcaEvidenceGraphFromTopology(appID, topologyNodes, topologyEdges, edgeEvidence)

		var incidents []IncidentSummary
		incidentQuery := rcaContextWindowQueryParams(windowedInput)
		if incidentQuery == nil {
			incidentQuery = url.Values{}
		}
		incidentQuery.Set("limit", strconv.Itoa(incidentLimit))
		if incidentsRaw, incidentsRef, incidentsErr := getCorootRaw(ctx, client, incidentsPath(project), incidentQuery); incidentsErr != nil {
			limitations = append(limitations, "Coroot incidents are unavailable: "+incidentsErr.Error())
		} else {
			rawRefs = appendCorootRawRefSummary(rawRefs, "incidents", incidentsRef)
			incidents = incidentSummariesFromRaw(incidentsRaw, corootInput{
				Service:      appID,
				ShowResolved: in.ShowResolved,
				Limit:        incidentLimit,
			})
		}

		if incidentFromTarget != nil {
			incidents = prependIncidentSummaryIfMissing(incidents, *incidentFromTarget, incidentLimit)
		}

		var referenceRCA *RCAReportResult
		if in.IncludeRCA != nil && *in.IncludeRCA {
			if rcaRaw, rcaRef, rcaErr := getCorootRaw(ctx, client, rcaPath(project, appID), url.Values{"withSummary": {"true"}}); rcaErr != nil {
				limitations = append(limitations, "Coroot RCA reference is unavailable: "+rcaErr.Error())
			} else {
				rawRefs = appendCorootRawRefSummary(rawRefs, "reference_rca", rcaRef)
				report := rcaFromRaw(project, appID, incidentID, rcaRaw, rcaRef)
				referenceRCA = &report
			}
		}

		if len(slos) == 0 {
			limitations = append(limitations, "No SLO fields were found for the target service in Coroot overview applications.")
		}
		if len(topologySummary.Dependencies.Upstream) == 0 && len(topologySummary.Dependencies.Downstream) == 0 {
			limitations = append(limitations, "No direct upstream or downstream dependency was found for the target service.")
		}

		abnormal := abnormalRCAContextDependencies(topologySummary.Dependencies, topologySummary.RelatedServices, topologySummary.AbnormalServices)
		abnormal = mergeTopologyDependencySummaries(abnormal, abnormalEdgeServices)
		relevantServices := rcaContextRelevantServiceIDs(appID, topologySummary.Dependencies, abnormal)
		relevantServices = appendRCAContextEdgeServiceIDs(relevantServices, edgeEvidence)
		var logSummary *CorootLogSummary
		if logsRaw, logsRef, logsErr := getCorootRaw(ctx, client, logsOverviewPath(project), rcaContextWindowQueryParams(windowedInput)); logsErr != nil {
			limitations = append(limitations, "Coroot logs overview is unavailable: "+logsErr.Error())
		} else {
			rawRefs = appendCorootRawRefSummary(rawRefs, "logs", logsRef)
			logs := logSummaryFromRaw(logsRaw, relevantServices, 5)
			logSummary = &logs
		}
		var traceSummary *CorootTraceSummary
		if tracesRaw, tracesRef, tracesErr := getCorootRaw(ctx, client, tracingPath(project, appID), applicationQueryParams(windowedInput)); tracesErr != nil {
			limitations = append(limitations, "Coroot tracing is unavailable: "+tracesErr.Error())
		} else {
			rawRefs = appendCorootRawRefSummary(rawRefs, "tracing", tracesRef)
			traces := traceSummaryFromRaw(tracesRaw, 5)
			traceSummary = &traces
		}
		var profilingSummary *CorootProfilingSummary
		if profilingRaw, profilingRef, profilingErr := getCorootRaw(ctx, client, profilingPath(project, appID), applicationQueryParams(windowedInput)); profilingErr != nil {
			limitations = append(limitations, "Coroot profiling is unavailable: "+profilingErr.Error())
		} else {
			rawRefs = appendCorootRawRefSummary(rawRefs, "profiling", profilingRef)
			profiling := profilingSummaryFromRaw(profilingRaw, 8)
			profilingSummary = &profiling
		}
		var deploymentEvents []CorootDeploymentEvent
		if deploymentsRaw, deploymentsRef, deploymentsErr := getCorootRaw(ctx, client, deploymentsPath(project), rcaContextWindowQueryParams(windowedInput)); deploymentsErr != nil {
			limitations = append(limitations, "Coroot deployments overview is unavailable: "+deploymentsErr.Error())
		} else {
			rawRefs = appendCorootRawRefSummary(rawRefs, "deployments", deploymentsRef)
			deploymentEvents = deploymentEventsFromRaw(deploymentsRaw, appID, 5)
		}
		target := RCAContextTarget{
			Service:     appID,
			ServiceName: serviceName(appID),
			Cluster:     targetService.Cluster,
			Category:    targetService.Category,
			Status:      firstNonBlank(targetService.Status, metricStatusByName(metrics, "status")),
			TimeRange:   strings.TrimSpace(in.TimeRange),
			IncidentID:  incidentID,
		}
		hypotheses := rcaHypothesesFromEvidence(target, slos, edgeEvidence, evidenceGraph, logSummary, traceSummary, deploymentEvents, limitations)
		summary := summarizeRCAContext(target, slos, chartSummary, topologySummary.Dependencies, abnormal, logSummary, traceSummary, profilingSummary, deploymentEvents, incidents, limitations)
		summary = enrichRCAContextSummaryWithHypotheses(summary, hypotheses)

		return RCAContextResult{
			SchemaVersion:    rcaContextSchemaVersion,
			Tool:             "coroot.collect_rca_context",
			Status:           "ok",
			Project:          project,
			Target:           target,
			Summary:          summary,
			TimeWindow:       timeWindow,
			SLOs:             slos,
			MetricSummaries:  append([]CorootMetricChartSummary(nil), chartSummary.MetricSummaries...),
			ReportSummaries:  append([]CorootReportChartSummary(nil), chartSummary.Reports...),
			Dependencies:     topologySummary.Dependencies,
			RelatedServices:  append([]TopologyDependencySummary(nil), topologySummary.RelatedServices...),
			AbnormalServices: abnormal,
			EvidenceGraph:    evidenceGraph,
			EdgeEvidence:     edgeEvidence,
			Hypotheses:       hypotheses,
			LogSummary:       logSummary,
			TraceSummary:     traceSummary,
			ProfilingSummary: profilingSummary,
			DeploymentEvents: deploymentEvents,
			RecentIncidents:  incidents,
			ReferenceRCA:     referenceRCA,
			Limitations:      limitations,
			RawRefs:          rawRefs,
		}, appRef, nil
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
			ChartSummary:  chartSummaryFromServiceMetrics(appID, metrics, chartReports),
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

func executeNodesOverview(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeProjectOverview(client, "coroot.nodes_overview", nodesOverviewPath)
}

func executeTracesOverview(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeProjectOverview(client, "coroot.traces_overview", tracesOverviewPath)
}

func executeRisksOverview(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeProjectOverview(client, "coroot.risks_overview", risksOverviewPath)
}

func executeProjectOverview(client *Client, toolName string, pathForProject func(string) string) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, pathForProject(project), corootQueryParams(in.Query))
		if err != nil {
			return nil, rawRef, err
		}
		return GenericCorootDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          toolName,
			Status:        "ok",
			Project:       project,
			Data:          sanitizeCorootData(raw),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeDeploymentsOverview(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, deploymentsPath(project), corootQueryParams(in.Query))
		if err != nil {
			return nil, rawRef, err
		}
		data := map[string]any{
			"deployments": deploymentEventsFromRaw(raw, in.Service, normalizedSmallLimit(in.Limit, 20)),
		}
		return GenericCorootDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.deployments_overview",
			Status:        "ok",
			Project:       project,
			Data:          data,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeApplicationLogs(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		appID, rawRef, err := resolveCorootApplicationInput(ctx, client, project, in)
		if err != nil {
			return nil, rawRef, err
		}
		raw, rawRef, err := getCorootRaw(ctx, client, applicationLogsPath(project, appID), applicationQueryParams(in))
		if err != nil {
			return nil, rawRef, err
		}
		return ApplicationLogsResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.application_logs",
			Status:        "ok",
			Project:       project,
			Service:       appID,
			Summary:       logSummaryFromRaw(raw, []string{appID, in.Service}, normalizedSmallLimit(in.Limit, 10)),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeApplicationTraces(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		appID, rawRef, err := resolveCorootApplicationInput(ctx, client, project, in)
		if err != nil {
			return nil, rawRef, err
		}
		raw, rawRef, err := getCorootRaw(ctx, client, tracingPath(project, appID), applicationQueryParams(in))
		if err != nil {
			return nil, rawRef, err
		}
		return ApplicationTracesResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.application_traces",
			Status:        "ok",
			Project:       project,
			Service:       appID,
			TraceID:       in.TraceID,
			Summary:       traceSummaryFromRaw(raw, normalizedSmallLimit(in.Limit, 10)),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeApplicationProfiling(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		appID, rawRef, err := resolveCorootApplicationInput(ctx, client, project, in)
		if err != nil {
			return nil, rawRef, err
		}
		raw, rawRef, err := getCorootRaw(ctx, client, profilingPath(project, appID), applicationQueryParams(in))
		if err != nil {
			return nil, rawRef, err
		}
		return ApplicationProfilingResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.application_profiling",
			Status:        "ok",
			Project:       project,
			Service:       appID,
			Summary:       profilingSummaryFromRaw(raw, normalizedSmallLimit(in.Limit, 10)),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeGetNode(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.NodeID) == "" {
			return nil, nil, fmt.Errorf("nodeId is required")
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, nodePath(project, in.NodeID), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return NodeResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.get_node",
			Status:        "ok",
			Project:       project,
			NodeID:        in.NodeID,
			Node:          nodeSummaryFromRaw(raw, in.NodeID),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeIncidents(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		limit := normalizedIncidentLimit(in.Limit)
		fetchLimit := limit
		if corootIncidentHasClientSideFilters(in) && fetchLimit < 200 {
			fetchLimit = 200
		}
		raw, rawRef, err := getCorootRaw(ctx, client, incidentsPath(project), url.Values{"limit": {strconv.Itoa(fetchLimit)}})
		if err != nil {
			return nil, rawRef, err
		}
		incidents := make([]IncidentSummary, 0)
		for _, obj := range objectArray(raw) {
			incident := incidentSummaryFromObject(obj)
			if !corootIncidentMatches(incident, in) {
				continue
			}
			incidents = append(incidents, incident)
			if len(incidents) >= limit {
				break
			}
		}
		return IncidentsResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.incidents",
			Status:        "ok",
			Project:       project,
			Incidents:     incidents,
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func normalizedIncidentLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizedRCAContextIncidentLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func normalizedTopologyDepth(depth int) int {
	if depth <= 0 {
		return 3
	}
	if depth > 4 {
		return 4
	}
	return depth
}

func corootIncidentHasClientSideFilters(in corootInput) bool {
	return strings.TrimSpace(in.Service) != "" ||
		strings.TrimSpace(in.Query) != "" ||
		strings.TrimSpace(in.Status) != "" ||
		strings.TrimSpace(in.Severity) != "" ||
		strings.TrimSpace(in.ApplicationCategory) != "" ||
		in.ShowResolved != nil
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
			var matched bool
			appID, rawRef, matched, err = resolveApplicationIDWithMatch(ctx, client, project, in.Service)
			if err == nil {
				var rcaRef *CorootRawRef
				raw, rcaRef, err = getCorootRaw(ctx, client, rcaPath(project, appID), url.Values{"withSummary": {"true"}})
				if err != nil {
					if matched && isCorootApplicationNotFound(err) {
						return unavailableNativeRCAReport(project, appID, in.IncidentID, rawRef), rawRef, nil
					}
					return nil, rcaRef, err
				}
				rawRef = rcaRef
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

func executeListDashboards(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, dashboardsPath(project), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return DashboardsResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.list_dashboards",
			Status:        "ok",
			Project:       project,
			Dashboards:    dashboardSummariesFromRaw(raw),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeGetDashboard(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.DashboardID) == "" {
			return nil, nil, fmt.Errorf("dashboardId is required")
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, dashboardPath(project, in.DashboardID), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return DashboardResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.get_dashboard",
			Status:        "ok",
			Project:       project,
			DashboardID:   in.DashboardID,
			Dashboard:     dashboardSummaryFromObject(firstObject(raw)),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeGetPanelData(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.DashboardID) == "" {
			return nil, nil, fmt.Errorf("dashboardId is required")
		}
		if strings.TrimSpace(in.PanelID) == "" {
			return nil, nil, fmt.Errorf("panelId is required")
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, panelDataPath(project), panelDataQueryParams(in))
		if err != nil {
			return nil, rawRef, err
		}
		return PanelDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.get_panel_data",
			Status:        "ok",
			Project:       project,
			DashboardID:   in.DashboardID,
			PanelID:       in.PanelID,
			ChartSummary:  panelChartSummaryFromRaw(raw, normalizedSmallLimit(in.Limit, 10)),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeListIntegrations(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeRedactedProjectData(client, "coroot.list_integrations", integrationsPath)
}

func executeGetIntegration(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.IntegrationType) == "" {
			return nil, nil, fmt.Errorf("integrationType is required")
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, integrationPath(project, in.IntegrationType), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return GenericCorootDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.get_integration",
			Status:        "ok",
			Project:       project,
			Data:          sanitizeCorootData(raw),
			RawRef:        rawRef,
		}, rawRef, nil
	}
}

func executeListInspections(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeRedactedProjectData(client, "coroot.list_inspections", inspectionsPath)
}

func executeGetInspectionConfig(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.InspectionType) == "" {
			return nil, nil, fmt.Errorf("inspectionType is required")
		}
		project := client.ResolveProject(in.Project)
		appID, rawRef, err := resolveCorootApplicationInput(ctx, client, project, in)
		if err != nil {
			return nil, rawRef, err
		}
		raw, rawRef, err := getCorootRaw(ctx, client, inspectionConfigPath(project, appID, in.InspectionType), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return GenericCorootDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          "coroot.get_inspection_config",
			Status:        "ok",
			Project:       project,
			Data: map[string]any{
				"service":        appID,
				"inspectionType": in.InspectionType,
				"config":         sanitizeCorootData(raw),
			},
			RawRef: rawRef,
		}, rawRef, nil
	}
}

func executeGetApplicationCategories(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeRedactedProjectData(client, "coroot.get_application_categories", applicationCategoriesPath)
}

func executeGetCustomApplications(client *Client) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return executeRedactedProjectData(client, "coroot.get_custom_applications", customApplicationsPath)
}

func executeRedactedProjectData(client *Client, toolName string, pathForProject func(string) string) func(context.Context, json.RawMessage) (any, *CorootRawRef, error) {
	return func(ctx context.Context, input json.RawMessage) (any, *CorootRawRef, error) {
		in, err := decodeCorootInput(input)
		if err != nil {
			return nil, nil, err
		}
		project := client.ResolveProject(in.Project)
		raw, rawRef, err := getCorootRaw(ctx, client, pathForProject(project), nil)
		if err != nil {
			return nil, rawRef, err
		}
		return GenericCorootDataResult{
			SchemaVersion: corootSchemaVersion,
			Tool:          toolName,
			Status:        "ok",
			Project:       project,
			Data:          sanitizeCorootData(raw),
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
		SkipReason:    corootSkipReasonForError(payload),
		Error:         payload,
		RawRef:        rawRef,
	}
}

func corootSkipReasonForError(payload CorootErrorPayload) string {
	message := strings.ToLower(strings.TrimSpace(payload.Message))
	switch strings.TrimSpace(payload.Kind) {
	case "empty_response":
		return "empty_data"
	case "not_configured", "transport_error", "timeout", "upstream_server_error", "read_error", "decode_error":
		return "coroot_mcp_unavailable"
	case "upstream_client_error":
		if payload.StatusCode == 404 {
			return "target_not_matched"
		}
		if strings.Contains(message, "time window") ||
			strings.Contains(message, "time_window") ||
			(strings.Contains(message, "from") && strings.Contains(message, "to") && strings.Contains(message, "time")) {
			return "time_window_not_matched"
		}
		return "coroot_mcp_unavailable"
	default:
		if strings.Contains(message, "no data") || strings.Contains(message, "empty data") {
			return "empty_data"
		}
		return ""
	}
}

func isCorootApplicationNotFound(err error) bool {
	corootErr, ok := err.(*CorootError)
	if !ok || corootErr.StatusCode != 404 {
		return false
	}
	return strings.Contains(strings.ToLower(corootErr.Message), "application not found")
}

func applicationsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/applications"
}

func userPath() string {
	return "/api/user"
}

func healthPath() string {
	return "/health"
}

func projectStatusPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/status"
}

func nodesOverviewPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/nodes"
}

func tracesOverviewPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/traces"
}

func risksOverviewPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/risks"
}

func topologyPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/map"
}

func applicationPath(project, appID string) string {
	return "/api/project/" + url.PathEscape(project) + "/app/" + url.PathEscape(appID)
}

func applicationLogsPath(project, appID string) string {
	return applicationPath(project, appID) + "/logs"
}

func incidentPath(project, incidentID string) string {
	return "/api/project/" + url.PathEscape(project) + "/incident/" + url.PathEscape(incidentID)
}

func incidentsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/incidents"
}

func rcaPath(project, appID string) string {
	return applicationPath(project, appID) + "/rca"
}

func alertRulesPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/alerting-rules"
}

func logsOverviewPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/logs"
}

func deploymentsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/overview/deployments"
}

func tracingPath(project, appID string) string {
	return applicationPath(project, appID) + "/tracing"
}

func profilingPath(project, appID string) string {
	return applicationPath(project, appID) + "/profiling"
}

func nodePath(project, nodeID string) string {
	return "/api/project/" + url.PathEscape(project) + "/node/" + url.PathEscape(nodeID)
}

func dashboardsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/dashboards"
}

func dashboardPath(project, dashboardID string) string {
	return dashboardsPath(project) + "/" + url.PathEscape(dashboardID)
}

func panelDataPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/panel/data"
}

func integrationsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/integrations"
}

func integrationPath(project, integrationType string) string {
	return integrationsPath(project) + "/" + url.PathEscape(integrationType)
}

func inspectionsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/inspections"
}

func inspectionConfigPath(project, appID, inspectionType string) string {
	return applicationPath(project, appID) + "/inspection/" + url.PathEscape(inspectionType) + "/config"
}

func applicationCategoriesPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/application_categories"
}

func customApplicationsPath(project string) string {
	return "/api/project/" + url.PathEscape(project) + "/custom_applications"
}

func resolveApplicationID(ctx context.Context, client *Client, project, service string) (string, *CorootRawRef, error) {
	id, rawRef, _, err := resolveApplicationIDWithMatch(ctx, client, project, service)
	return id, rawRef, err
}

func resolveApplicationIDWithMatch(ctx context.Context, client *Client, project, service string) (string, *CorootRawRef, bool, error) {
	raw, rawRef, err := getCorootRaw(ctx, client, applicationsPath(project), nil)
	if err != nil {
		return "", rawRef, false, err
	}
	for _, app := range objectArray(raw, "applications") {
		id := stringFromAny(app["id"])
		if serviceMatches(id, service) {
			return id, rawRef, true, nil
		}
	}
	return service, rawRef, false, nil
}

func resolveCorootApplicationInput(ctx context.Context, client *Client, project string, in corootInput) (string, *CorootRawRef, error) {
	if appID := strings.TrimSpace(in.AppID); appID != "" {
		return appID, nil, nil
	}
	service := strings.TrimSpace(in.Service)
	if service == "" {
		return "", nil, fmt.Errorf("service or appId is required")
	}
	return resolveApplicationID(ctx, client, project, service)
}

func corootQueryParams(query string) url.Values {
	values := url.Values{}
	if strings.TrimSpace(query) != "" {
		values.Set("query", strings.TrimSpace(query))
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func applicationQueryParams(in corootInput) url.Values {
	values := url.Values{}
	if in.FromTimestamp > 0 {
		values.Set("from", corootUnixMillisQueryValue(in.FromTimestamp))
	}
	if in.ToTimestamp > 0 {
		values.Set("to", corootUnixMillisQueryValue(in.ToTimestamp))
	}
	if strings.TrimSpace(in.TraceID) != "" {
		values.Set("trace_id", strings.TrimSpace(in.TraceID))
	}
	if strings.TrimSpace(in.Query) != "" {
		values.Set("query", strings.TrimSpace(in.Query))
	}
	if strings.TrimSpace(in.Severity) != "" {
		values.Set("severity", strings.TrimSpace(in.Severity))
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func panelDataQueryParams(in corootInput) url.Values {
	values := url.Values{
		"dashboard": {strings.TrimSpace(in.DashboardID)},
		"panel":     {strings.TrimSpace(in.PanelID)},
	}
	if strings.TrimSpace(in.From) != "" {
		values.Set("from", strings.TrimSpace(in.From))
	}
	if strings.TrimSpace(in.To) != "" {
		values.Set("to", strings.TrimSpace(in.To))
	}
	return values
}

func normalizedSmallLimit(limit, fallback int) int {
	if fallback <= 0 {
		fallback = 10
	}
	if limit <= 0 {
		return fallback
	}
	if limit > 50 {
		return 50
	}
	return limit
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

func serviceSummaryFromRaw(raw json.RawMessage, service string) ServiceSummary {
	for _, app := range objectArray(raw, "applications") {
		id := stringFromAny(app["id"])
		if serviceMatches(id, service) {
			return serviceSummaryFromObject(app)
		}
	}
	return ServiceSummary{ID: strings.TrimSpace(service), Name: serviceName(service)}
}

func projectSummariesFromRaw(raw json.RawMessage) []ProjectSummary {
	root := firstObject(raw)
	var projects []ProjectSummary
	for _, obj := range objectSlice(firstNonNil(root["projects"], root["accessible_projects"], root["items"])) {
		project := ProjectSummary{
			ID:   firstNonBlank(stringFromAny(obj["id"]), stringFromAny(obj["name"]), stringFromAny(obj["key"])),
			Name: firstNonBlank(stringFromAny(obj["name"]), stringFromAny(obj["id"])),
			Role: stringFromAny(obj["role"]),
		}
		if project.ID != "" || project.Name != "" {
			projects = append(projects, project)
		}
	}
	return projects
}

func nodeSummaryFromRaw(raw json.RawMessage, fallbackID string) NodeSummary {
	obj := firstObject(raw)
	node := objectField(obj, "node")
	if len(node) == 0 {
		node = obj
	}
	id := firstNonBlank(stringFromAny(node["id"]), stringFromAny(node["name"]), fallbackID)
	summary := NodeSummary{
		ID:           id,
		Name:         firstNonBlank(stringFromAny(node["name"]), serviceName(id), id),
		Status:       firstNonBlank(stringFromAny(node["status"]), stringFromAny(node["health"])),
		Cluster:      stringFromAny(node["cluster"]),
		Region:       firstNonBlank(stringFromAny(node["region"]), stringFromAny(node["availability_zone"]), stringFromAny(node["zone"])),
		InstanceType: firstNonBlank(stringFromAny(node["instance_type"]), stringFromAny(node["instanceType"])),
	}
	for _, app := range objectSlice(firstNonNil(node["applications"], node["apps"], obj["applications"])) {
		id := firstNonBlank(stringFromAny(app["id"]), stringFromAny(app["name"]))
		summary.Applications = appendCorootUniqueString(summary.Applications, firstNonBlank(serviceName(id), id), 12)
	}
	for _, key := range []string{"cpu", "memory", "disk", "network", "net"} {
		if signal := nodeSignalFromObject(key, objectField(node, key)); signal != "" {
			summary.ResourceSignals = appendCorootUniqueString(summary.ResourceSignals, signal, 12)
		}
	}
	for _, report := range objectSlice(firstNonNil(node["reports"], obj["reports"])) {
		name := firstNonBlank(stringFromAny(report["name"]), stringFromAny(report["title"]))
		status := stringFromAny(report["status"])
		if name == "" && status == "" {
			continue
		}
		summary.Summary = appendCorootUniqueString(summary.Summary, strings.TrimSpace(strings.Join([]string{name, status}, " ")), 12)
	}
	return summary
}

func nodeSignalFromObject(name string, obj map[string]any) string {
	if len(obj) == 0 {
		return ""
	}
	parts := []string{name}
	if status := stringFromAny(obj["status"]); status != "" {
		parts = append(parts, status)
	}
	if value := firstNonBlank(stringFromAny(obj["value"]), stringFromAny(obj["usage"]), stringFromAny(obj["utilization"])); value != "" {
		parts = append(parts, value)
	}
	return strings.Join(parts, " ")
}

func dashboardSummariesFromRaw(raw json.RawMessage) []DashboardSummary {
	var out []DashboardSummary
	for _, obj := range objectArray(raw, "dashboards", "items") {
		summary := dashboardSummaryFromObject(obj)
		if summary.ID != "" || summary.Name != "" {
			out = append(out, summary)
		}
	}
	return out
}

func dashboardSummaryFromObject(obj map[string]any) DashboardSummary {
	if dashboard := objectField(obj, "dashboard"); len(dashboard) > 0 {
		obj = dashboard
	}
	id := firstNonBlank(stringFromAny(obj["id"]), stringFromAny(obj["key"]), stringFromAny(obj["uid"]))
	return DashboardSummary{
		ID:          id,
		Name:        firstNonBlank(stringFromAny(obj["name"]), stringFromAny(obj["title"]), id),
		Description: truncateCorootText(stringFromAny(obj["description"]), 240),
		PanelCount:  len(objectSlice(firstNonNil(obj["panels"], obj["widgets"]))),
		Tags:        stringSlice(obj["tags"]),
	}
}

func panelChartSummaryFromRaw(raw json.RawMessage, limit int) CorootChartSummary {
	if limit <= 0 {
		limit = 10
	}
	root := firstObject(raw)
	summary := CorootChartSummary{}
	var reports []CorootChartReport
	appendReport := func(name, status string, widgets []map[string]any) {
		if len(widgets) == 0 || len(reports) >= limit {
			return
		}
		if name == "" {
			name = "Panel"
		}
		reports = append(reports, CorootChartReport{Name: name, Status: status, Widgets: widgets})
	}
	for _, report := range objectSlice(root["reports"]) {
		var widgets []map[string]any
		for _, widget := range objectSlice(report["widgets"]) {
			if chartWidget, ok := corootChartWidgetFromRaw(widget); ok {
				widgets = append(widgets, chartWidget)
			}
		}
		appendReport(firstNonBlank(stringFromAny(report["name"]), stringFromAny(report["title"])), stringFromAny(report["status"]), widgets)
	}
	for _, widget := range objectSlice(firstNonNil(root["widgets"], root["panels"])) {
		if chartWidget, ok := corootChartWidgetFromRaw(widget); ok {
			appendReport(firstNonBlank(stringFromAny(widget["name"]), stringFromAny(widget["title"])), stringFromAny(widget["status"]), []map[string]any{chartWidget})
		}
	}
	if chartWidget, ok := corootChartWidgetFromRaw(root); ok {
		appendReport(firstNonBlank(stringFromAny(root["name"]), stringFromAny(root["title"])), stringFromAny(root["status"]), []map[string]any{chartWidget})
	}
	summary = chartSummaryFromServiceMetrics("", nil, reports)
	if len(summary.Reports) > limit {
		summary.Reports = summary.Reports[:limit]
	}
	return summary
}

func slosFromApplicationsRaw(raw json.RawMessage, service string, sloName string) []SLOStatus {
	var slos []SLOStatus
	for _, app := range objectArray(raw, "applications") {
		id := stringFromAny(app["id"])
		if service != "" && !serviceMatches(id, service) {
			continue
		}
		slos = append(slos, sloFromParam("availability", objectField(app, "errors"))...)
		slos = append(slos, sloFromParam("latency", objectField(app, "latency"))...)
	}
	if strings.TrimSpace(sloName) == "" {
		return slos
	}
	filtered := slos[:0]
	for _, slo := range slos {
		if strings.EqualFold(slo.Name, sloName) {
			filtered = append(filtered, slo)
		}
	}
	return filtered
}

func incidentSummariesFromRaw(raw json.RawMessage, in corootInput) []IncidentSummary {
	limit := normalizedRCAContextIncidentLimit(in.Limit)
	incidents := make([]IncidentSummary, 0, limit)
	for _, obj := range objectArray(raw) {
		incident := incidentSummaryFromObject(obj)
		if !corootIncidentMatches(incident, in) {
			continue
		}
		incidents = append(incidents, incident)
		if len(incidents) >= limit {
			break
		}
	}
	return incidents
}

func prependIncidentSummaryIfMissing(incidents []IncidentSummary, incident IncidentSummary, limit int) []IncidentSummary {
	limit = normalizedRCAContextIncidentLimit(limit)
	key := firstNonBlank(incident.Key, incident.ID)
	for _, existing := range incidents {
		if firstNonBlank(existing.Key, existing.ID) == key {
			return incidents
		}
	}
	out := make([]IncidentSummary, 0, minInt(limit, len(incidents)+1))
	out = append(out, incident)
	for _, existing := range incidents {
		if len(out) >= limit {
			break
		}
		out = append(out, existing)
	}
	return out
}

func appendCorootRawRefSummary(items []CorootRawRefSummary, purpose string, rawRef *CorootRawRef) []CorootRawRefSummary {
	if rawRef == nil {
		return items
	}
	purpose = strings.TrimSpace(purpose)
	for _, item := range items {
		if strings.TrimSpace(item.Purpose) == purpose && item.RawRef != nil && item.RawRef.Digest == rawRef.Digest {
			return items
		}
	}
	return append(items, CorootRawRefSummary{Purpose: purpose, RawRef: rawRef})
}

func metricStatusByName(metrics []MetricSummary, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, metric := range metrics {
		if strings.ToLower(strings.TrimSpace(metric.Name)) == name {
			return firstNonBlank(metric.Status, metric.Value)
		}
	}
	return ""
}

func abnormalRCAContextDependencies(groups TopologyDependencyGroups, related []TopologyDependencySummary, additional []TopologyDependencySummary) []TopologyDependencySummary {
	seen := map[string]struct{}{}
	var out []TopologyDependencySummary
	for _, list := range [][]TopologyDependencySummary{groups.Upstream, groups.Downstream, related, additional} {
		for _, item := range list {
			if !topologyDependencyLooksAbnormal(item) {
				continue
			}
			out = appendUniqueTopologyDependency(out, seen, item)
		}
	}
	return out
}

func topologyDependencyLooksAbnormal(item TopologyDependencySummary) bool {
	if isProblemStatus(item.Status) {
		return true
	}
	for _, stat := range item.Stats {
		normalized := strings.ToLower(strings.TrimSpace(stat))
		if strings.Contains(normalized, "fail") ||
			strings.Contains(normalized, "error") ||
			strings.Contains(normalized, "timeout") ||
			strings.Contains(normalized, "retrans") ||
			strings.Contains(normalized, "5xx") {
			return true
		}
	}
	return false
}

func rcaContextRelevantServiceIDs(appID string, groups TopologyDependencyGroups, abnormal []TopologyDependencySummary) []string {
	var ids []string
	ids = appendRCAContextServiceID(ids, appID)
	for _, list := range [][]TopologyDependencySummary{groups.Upstream, groups.Downstream, abnormal} {
		for _, item := range list {
			ids = appendRCAContextServiceID(ids, item.ID)
			ids = appendRCAContextServiceID(ids, item.Name)
		}
	}
	return ids
}

func appendRCAContextServiceID(ids []string, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ids
	}
	for _, existing := range ids {
		if strings.EqualFold(existing, id) {
			return ids
		}
	}
	return append(ids, id)
}

func appendRCAContextEdgeServiceIDs(ids []string, edges []RCAEdgeEvidence) []string {
	for _, edge := range edges {
		ids = appendRCAContextServiceID(ids, edge.Source)
		ids = appendRCAContextServiceID(ids, edge.SourceName)
		ids = appendRCAContextServiceID(ids, edge.Target)
		ids = appendRCAContextServiceID(ids, edge.TargetName)
	}
	return ids
}

func mergeTopologyDependencySummaries(base []TopologyDependencySummary, extra []TopologyDependencySummary) []TopologyDependencySummary {
	seen := map[string]struct{}{}
	out := make([]TopologyDependencySummary, 0, len(base)+len(extra))
	for _, item := range base {
		out = appendUniqueTopologyDependency(out, seen, item)
	}
	for _, item := range extra {
		out = appendUniqueTopologyDependency(out, seen, item)
	}
	return out
}

func rcaContextTimeWindow(in corootInput, incident map[string]any, now time.Time) RCAContextTimeWindow {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if len(incident) > 0 {
		if opened, ok := timeFromCorootAny(incident["opened_at"]); ok {
			from := opened.Add(-(time.Hour + 5*time.Minute))
			to := now
			if resolved, ok := timeFromCorootAny(incident["resolved_at"]); ok && resolved.After(opened) {
				to = resolved
			}
			return RCAContextTimeWindow{
				From:   from.UTC().Format(time.RFC3339),
				To:     to.UTC().Format(time.RFC3339),
				Source: "incident",
				Reason: "incident opened minus 1h5m through resolved time or now",
			}
		}
	}
	from, hasFrom := inputTimeFrom(in.FromTimestamp, in.From)
	to, hasTo := inputTimeFrom(in.ToTimestamp, in.To)
	if hasFrom || hasTo {
		if !hasTo {
			to = now
		}
		if !hasFrom {
			from = to.Add(-time.Hour)
		}
		if !from.Before(to) {
			from = to.Add(-time.Hour)
		}
		return RCAContextTimeWindow{
			From:   from.UTC().Format(time.RFC3339),
			To:     to.UTC().Format(time.RFC3339),
			Source: "user",
			Reason: "explicit from/to analysis window",
		}
	}
	if d, ok := parseCorootDuration(in.TimeRange); ok {
		return RCAContextTimeWindow{
			From:   now.Add(-d).UTC().Format(time.RFC3339),
			To:     now.Format(time.RFC3339),
			Source: "user",
			Reason: "timeRange " + strings.TrimSpace(in.TimeRange),
		}
	}
	return RCAContextTimeWindow{
		From:   now.Add(-time.Hour).UTC().Format(time.RFC3339),
		To:     now.Format(time.RFC3339),
		Source: "default",
		Reason: "default last 1h RCA window",
	}
}

func applyRCAContextTimeWindowToInput(in *corootInput, window RCAContextTimeWindow) {
	if in == nil {
		return
	}
	if from, ok := parseRFC3339Time(window.From); ok {
		in.FromTimestamp = from.Unix()
		in.From = window.From
	}
	if to, ok := parseRFC3339Time(window.To); ok {
		in.ToTimestamp = to.Unix()
		in.To = window.To
	}
}

func rcaContextWindowQueryParams(in corootInput) url.Values {
	values := url.Values{}
	if in.FromTimestamp > 0 {
		values.Set("from", corootUnixMillisQueryValue(in.FromTimestamp))
	} else if strings.TrimSpace(in.From) != "" {
		values.Set("from", strings.TrimSpace(in.From))
	}
	if in.ToTimestamp > 0 {
		values.Set("to", corootUnixMillisQueryValue(in.ToTimestamp))
	} else if strings.TrimSpace(in.To) != "" {
		values.Set("to", strings.TrimSpace(in.To))
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func corootUnixMillisQueryValue(timestamp int64) string {
	if timestamp > 1_000_000_000_000 {
		return strconv.FormatInt(timestamp, 10)
	}
	return strconv.FormatInt(timestamp*1000, 10)
}

func inputTimeFrom(timestamp int64, value string) (time.Time, bool) {
	if timestamp > 0 {
		if timestamp > 1_000_000_000_000 {
			return time.UnixMilli(timestamp).UTC(), true
		}
		return time.Unix(timestamp, 0).UTC(), true
	}
	return parseRFC3339Time(value)
}

func parseRFC3339Time(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func timeFromCorootAny(value any) (time.Time, bool) {
	switch v := value.(type) {
	case string:
		if t, ok := parseRFC3339Time(v); ok {
			return t, true
		}
		ts := int64FromAny(v)
		if ts <= 0 {
			return time.Time{}, false
		}
		return unixTimeFromCorootTimestamp(ts), true
	default:
		ts := int64FromAny(value)
		if ts <= 0 {
			return time.Time{}, false
		}
		return unixTimeFromCorootTimestamp(ts), true
	}
}

func unixTimeFromCorootTimestamp(ts int64) time.Time {
	if ts > 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC()
	}
	return time.Unix(ts, 0).UTC()
}

func parseCorootDuration(value string) (time.Duration, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(value); err == nil && d > 0 {
		return d, true
	}
	multiplier := time.Duration(0)
	switch {
	case strings.HasSuffix(value, "d"):
		multiplier = 24 * time.Hour
	case strings.HasSuffix(value, "w"):
		multiplier = 7 * 24 * time.Hour
	default:
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value[:len(value)-1]))
	if err != nil || n <= 0 {
		return 0, false
	}
	return time.Duration(n) * multiplier, true
}

func logSummaryFromRaw(raw json.RawMessage, services []string, limit int) CorootLogSummary {
	if limit <= 0 {
		limit = 5
	}
	root := firstObject(raw)
	logs := objectField(root, "logs")
	entries := objectSlice(logs["entries"])
	if len(entries) == 0 {
		entries = objectSlice(root["entries"])
	}
	summary := CorootLogSummary{
		TotalCount: len(entries),
		Severities: map[string]int{},
	}
	fallbackEntries := make([]CorootLogEntry, 0, limit)
	for _, entry := range entries {
		application := firstNonBlank(stringFromAny(entry["application_id"]), stringFromAny(entry["application"]))
		message := firstNonBlank(stringFromAny(entry["message"]), stringFromAny(entry["body"]))
		severity := strings.ToLower(firstNonBlank(stringFromAny(entry["severity"]), stringFromAny(entry["level"])))
		if !logEntryMatchesServices(application, services) {
			continue
		}
		summary.MatchedCount++
		if severity != "" {
			summary.Severities[severity]++
		}
		summary.Applications = appendCorootUniqueString(summary.Applications, firstNonBlank(serviceName(application), application), 8)
		entrySummary := CorootLogEntry{
			Application: firstNonBlank(serviceName(application), application),
			Severity:    severity,
			Message:     truncateCorootText(message, 240),
			Timestamp:   corootTimestampString(entry["timestamp"]),
		}
		if isLogErrorLike(severity, message) {
			summary.ErrorLikeCount++
			if len(summary.Entries) < limit {
				summary.Entries = append(summary.Entries, entrySummary)
			}
			continue
		}
		if len(fallbackEntries) < limit {
			fallbackEntries = append(fallbackEntries, entrySummary)
		}
	}
	if len(summary.Entries) == 0 {
		summary.Entries = fallbackEntries
	}
	if len(summary.Severities) == 0 {
		summary.Severities = nil
	}
	return summary
}

func logEntryMatchesServices(application string, services []string) bool {
	if len(services) == 0 {
		return true
	}
	for _, service := range services {
		if serviceMatches(application, service) {
			return true
		}
	}
	return false
}

func isLogErrorLike(severity, message string) bool {
	normalizedSeverity := strings.ToLower(strings.TrimSpace(severity))
	if normalizedSeverity == "error" || normalizedSeverity == "fatal" || normalizedSeverity == "panic" || normalizedSeverity == "critical" {
		return true
	}
	normalizedMessage := strings.ToLower(message)
	for _, marker := range []string{"error", "failed", "failure", "timeout", "exception", "panic", "oom", "crash", "unavailable", "refused"} {
		if strings.Contains(normalizedMessage, marker) {
			return true
		}
	}
	return false
}

func traceSummaryFromRaw(raw json.RawMessage, limit int) CorootTraceSummary {
	if limit <= 0 {
		limit = 5
	}
	root := firstObject(raw)
	summary := CorootTraceSummary{
		Status:  stringFromAny(root["status"]),
		Message: truncateCorootText(stringFromAny(root["message"]), 240),
		Limit:   intFromAny(root["limit"]),
	}
	for _, src := range objectSlice(root["sources"]) {
		name := firstNonBlank(stringFromAny(src["name"]), stringFromAny(src["type"]))
		if stringFromAny(src["selected"]) == "true" && name != "" {
			name += " selected"
		}
		summary.Sources = appendCorootUniqueString(summary.Sources, name, 8)
	}
	for _, svc := range objectSlice(root["services"]) {
		if stringFromAny(svc["linked"]) == "false" {
			continue
		}
		summary.LinkedServices = appendCorootUniqueString(summary.LinkedServices, stringFromAny(svc["name"]), 8)
	}
	spans := objectSlice(root["spans"])
	summary.SpanCount = len(spans)
	for _, span := range spans {
		status := strings.ToLower(stringFromAny(span["status"]))
		if isTraceProblemStatus(status) {
			summary.ErrorSpanCount++
		}
		duration, _ := floatFromAny(span["duration"])
		item := CorootTraceSpan{
			Service:    stringFromAny(span["service"]),
			Client:     stringFromAny(span["client"]),
			Name:       truncateCorootText(stringFromAny(span["name"]), 160),
			Status:     status,
			TraceID:    stringFromAny(span["trace_id"]),
			DurationMS: duration,
		}
		summary.SlowestSpans = append(summary.SlowestSpans, item)
	}
	sort.Slice(summary.SlowestSpans, func(i, j int) bool {
		return summary.SlowestSpans[i].DurationMS > summary.SlowestSpans[j].DurationMS
	})
	if len(summary.SlowestSpans) > limit {
		summary.SlowestSpans = summary.SlowestSpans[:limit]
	}
	return summary
}

func isTraceProblemStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" || status == "ok" || status == "success" || status == "unset" {
		return false
	}
	if code, err := strconv.Atoi(status); err == nil {
		return code >= 400
	}
	return true
}

func profilingSummaryFromRaw(raw json.RawMessage, limit int) CorootProfilingSummary {
	if limit <= 0 {
		limit = 8
	}
	root := firstObject(raw)
	summary := CorootProfilingSummary{
		Status:  stringFromAny(root["status"]),
		Message: truncateCorootText(stringFromAny(root["message"]), 240),
	}
	for _, svc := range objectSlice(root["services"]) {
		if stringFromAny(svc["linked"]) == "false" {
			continue
		}
		summary.LinkedServices = appendCorootUniqueString(summary.LinkedServices, stringFromAny(svc["name"]), limit)
	}
	for _, profile := range objectSlice(root["profiles"]) {
		name := firstNonBlank(stringFromAny(profile["name"]), stringFromAny(profile["type"]))
		if typ := stringFromAny(profile["type"]); typ != "" && name != typ {
			name += " (" + typ + ")"
		}
		summary.Profiles = appendCorootUniqueString(summary.Profiles, name, limit)
	}
	summary.ProfileCount = len(objectSlice(root["profiles"]))
	for _, instance := range anyStringSlice(root["instances"]) {
		summary.Instances = appendCorootUniqueString(summary.Instances, instance, limit)
	}
	summary.InstanceCount = len(anyStringSlice(root["instances"]))
	return summary
}

func deploymentEventsFromRaw(raw json.RawMessage, service string, limit int) []CorootDeploymentEvent {
	if limit <= 0 {
		limit = 5
	}
	var out []CorootDeploymentEvent
	for _, item := range objectArray(raw, "deployments") {
		application := objectField(item, "application")
		appID := firstNonBlank(stringFromAny(application["id"]), stringFromAny(item["application_id"]))
		if service != "" && !serviceMatches(appID, service) {
			continue
		}
		event := CorootDeploymentEvent{
			ApplicationID: appID,
			Application:   serviceName(appID),
			Category:      firstNonBlank(stringFromAny(application["category"]), stringFromAny(item["category"])),
			Version:       stringFromAny(item["version"]),
			Deployed:      firstNonBlank(stringFromAny(item["deployed"]), stringFromAny(item["timestamp"])),
			Status:        stringFromAny(item["status"]),
			Age:           stringFromAny(item["age"]),
		}
		for _, summary := range objectSlice(item["summary"]) {
			message := stringFromAny(summary["message"])
			if status := stringFromAny(summary["status"]); status != "" {
				message = status + " " + message
			}
			event.Summary = appendCorootUniqueString(event.Summary, truncateCorootText(message, 180), 5)
		}
		out = append(out, event)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func rcaEdgeEvidenceFromTopology(appID string, nodes []TopologyNode, edges []TopologyEdge) []RCAEdgeEvidence {
	nodeByID := topologyNodeByID(nodes)
	var out []RCAEdgeEvidence
	for _, edge := range edges {
		if edge.Source == "" || edge.Target == "" {
			continue
		}
		source := nodeByID[edge.Source]
		target := nodeByID[edge.Target]
		status := topologyDependencyStatus(edge.Status, target.Status)
		connectivity, message := rcaConnectivityFromEdge(edge)
		signals := rcaEdgeSignals(edge, connectivity, message)
		score := rcaEdgeScore(status, connectivity, message, signals)
		if score == 0 && !rcaEdgeNearTarget(appID, edge) {
			continue
		}
		targetName := firstNonBlank(target.Name, serviceName(edge.Target))
		targetKind := rcaDependencyTargetKind(edge.Target, targetName, target.Category)
		out = append(out, RCAEdgeEvidence{
			Source:              edge.Source,
			Target:              edge.Target,
			SourceName:          firstNonBlank(source.Name, serviceName(edge.Source)),
			TargetName:          targetName,
			TargetKind:          targetKind,
			TargetEndpoint:      rcaExternalDependencyEndpoint(edge.Target, targetName, targetKind),
			Status:              status,
			Connectivity:        connectivity,
			ConnectivityMessage: message,
			Stats:               append([]string(nil), edge.Stats...),
			Signals:             signals,
			Score:               score,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Source+"->"+out[i].Target < out[j].Source+"->"+out[j].Target
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > maxCorootModelTopologyItems {
		out = out[:maxCorootModelTopologyItems]
	}
	return out
}

func rcaDependencyTargetKind(id, name, category string) string {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	category = strings.TrimSpace(category)
	lowerID := strings.ToLower(id)
	haystack := strings.ToLower(strings.Join([]string{id, name, category}, " "))
	if strings.HasPrefix(lowerID, "external:") ||
		strings.Contains(lowerID, ":externalservice:") ||
		strings.EqualFold(category, "external") ||
		strings.Contains(haystack, "externalservice") {
		return "external"
	}
	return ""
}

func rcaExternalDependencyEndpoint(id, name, targetKind string) string {
	if targetKind != "external" {
		return ""
	}
	lowerID := strings.ToLower(id)
	marker := ":externalservice:"
	if idx := strings.Index(lowerID, marker); idx >= 0 {
		endpoint := strings.TrimSpace(id[idx+len(marker):])
		if endpoint != "" {
			return endpoint
		}
	}
	return firstNonBlank(name, serviceName(id), id)
}

func topologyNodeByID(nodes []TopologyNode) map[string]TopologyNode {
	out := map[string]TopologyNode{}
	for _, node := range nodes {
		if node.ID != "" {
			out[node.ID] = node
		}
	}
	return out
}

func strongestStatus(values ...string) string {
	best := ""
	bestScore := 0
	for _, value := range values {
		status := normalizedCorootHealthStatus(value)
		score := statusSeverityScore(status)
		if score > bestScore || (best == "" && status != "") {
			best = status
			bestScore = score
		}
	}
	return best
}

func statusSeverityScore(status string) int {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "critical", "error", "failed":
		return 3
	case "warning", "degraded":
		return 2
	case "ok", "healthy", "success":
		return 1
	default:
		return 0
	}
}

func rcaConnectivityFromEdge(edge TopologyEdge) (string, string) {
	status := normalizedCorootHealthStatus(edge.Status)
	message := ""
	connectivity := ""
	for _, stat := range edge.Stats {
		normalized := strings.ToLower(strings.TrimSpace(stat))
		switch {
		case strings.Contains(normalized, "connectivity"):
			connectivity = "critical"
			message = firstNonBlank(message, stat)
		case strings.Contains(normalized, "failed connection"), strings.Contains(normalized, "connection refused"), strings.Contains(normalized, "connection reset"):
			connectivity = "critical"
			message = firstNonBlank(message, stat)
		case strings.Contains(normalized, "timeout"), strings.Contains(normalized, "timed out"):
			if connectivity == "" {
				connectivity = "critical"
			}
			message = firstNonBlank(message, stat)
		case strings.Contains(normalized, "retrans"):
			if connectivity == "" {
				connectivity = "warning"
			}
			message = firstNonBlank(message, stat)
		}
	}
	if connectivity == "" && isProblemStatus(status) {
		connectivity = status
	}
	return connectivity, truncateCorootText(message, 180)
}

func rcaEdgeSignals(edge TopologyEdge, connectivity, message string) []string {
	var signals []string
	if connectivity != "" {
		signal := "connectivity " + connectivity
		if message != "" {
			signal += ": " + message
		}
		signals = appendCorootUniqueString(signals, signal, 8)
	}
	if status := normalizedCorootHealthStatus(edge.Status); isProblemStatus(status) {
		signals = appendCorootUniqueString(signals, "edge status "+status, 8)
	}
	for _, stat := range edge.Stats {
		normalized := strings.ToLower(strings.TrimSpace(stat))
		if strings.Contains(normalized, "fail") ||
			strings.Contains(normalized, "error") ||
			strings.Contains(normalized, "timeout") ||
			strings.Contains(normalized, "retrans") ||
			strings.Contains(normalized, "5xx") ||
			strings.Contains(normalized, "refused") ||
			strings.Contains(normalized, "reset") {
			signals = appendCorootUniqueString(signals, truncateCorootText(stat, 180), 8)
		}
	}
	return signals
}

func rcaEdgeScore(status, connectivity, message string, signals []string) int {
	score := 0
	switch strings.ToLower(strings.TrimSpace(connectivity)) {
	case "critical", "error", "failed":
		score += 40
	case "warning", "degraded":
		score += 25
	}
	if isProblemStatus(status) {
		score += 20
	}
	normalized := strings.ToLower(strings.Join(append(append([]string{}, signals...), message), " "))
	for _, marker := range []struct {
		text  string
		score int
	}{
		{text: "failed connection", score: 35},
		{text: "connection refused", score: 35},
		{text: "connection reset", score: 35},
		{text: "timeout", score: 30},
		{text: "error", score: 20},
		{text: "5xx", score: 20},
		{text: "retrans", score: 15},
	} {
		if strings.Contains(normalized, marker.text) {
			score += marker.score
			break
		}
	}
	if score == 0 && len(signals) > 0 {
		score = 10
	}
	return score
}

func rcaEdgeNearTarget(appID string, edge TopologyEdge) bool {
	return serviceMatches(edge.Source, appID) || serviceMatches(edge.Target, appID)
}

func rcaEdgeEvidenceDependencies(edges []RCAEdgeEvidence, nodes []TopologyNode) []TopologyDependencySummary {
	nodeByID := topologyNodeByID(nodes)
	seen := map[string]struct{}{}
	var out []TopologyDependencySummary
	for _, edge := range edges {
		if edge.Score <= 0 {
			continue
		}
		target := nodeByID[edge.Target]
		item := topologyDependencyFromNode(target, edge.Target, "edge", firstNonBlank(edge.Connectivity, edge.Status), edge.Stats)
		out = appendUniqueTopologyDependency(out, seen, item)
	}
	return out
}

func rcaEvidenceGraphFromTopology(appID string, nodes []TopologyNode, edges []TopologyEdge, evidence []RCAEdgeEvidence) *RCAEvidenceGraph {
	if len(nodes) == 0 && len(edges) == 0 {
		return nil
	}
	distances, parents := topologyDistancesAndParents(appID, nodes, edges)
	nodeLimit := maxCorootModelTopologyItems
	graph := &RCAEvidenceGraph{}
	for _, node := range nodes {
		if len(graph.Nodes) >= nodeLimit {
			graph.Truncated = true
			break
		}
		graph.Nodes = append(graph.Nodes, RCAEvidenceNode{
			ID:       node.ID,
			Name:     firstNonBlank(node.Name, serviceName(node.ID)),
			Cluster:  node.Cluster,
			Category: node.Category,
			Status:   normalizedCorootHealthStatus(node.Status),
			Distance: distances[node.ID],
		})
	}
	edgeLimit := maxCorootModelTopologyItems
	for _, edge := range edges {
		if len(graph.Edges) >= edgeLimit {
			graph.Truncated = true
			break
		}
		graph.Edges = append(graph.Edges, rcaEvidenceEdgeFromTopologyEdge(edge, nodes, distances))
	}
	for _, item := range evidence {
		path := rcaPathToTarget(appID, item.Target, parents)
		if len(path) == 0 {
			path = rcaPathToTarget(appID, item.Source, parents)
		}
		if len(path) == 0 {
			path = []string{item.Source, item.Target}
		}
		graph.Paths = append(graph.Paths, RCAEvidencePath{
			From:         item.Source,
			To:           appID,
			Reason:       firstNonBlank(item.ConnectivityMessage, strings.Join(item.Signals, "; "), "suspect dependency edge"),
			Services:     path,
			ServiceNames: rcaServiceNames(path),
		})
		if len(graph.Paths) >= 8 {
			break
		}
	}
	return graph
}

func rcaEvidenceEdgeFromTopologyEdge(edge TopologyEdge, nodes []TopologyNode, distances map[string]int) RCAEvidenceEdge {
	nodeByID := topologyNodeByID(nodes)
	target := nodeByID[edge.Target]
	source := nodeByID[edge.Source]
	status := topologyDependencyStatus(edge.Status, target.Status)
	connectivity, message := rcaConnectivityFromEdge(edge)
	depth := distances[edge.Source]
	if targetDepth := distances[edge.Target]; depth == 0 || (targetDepth > 0 && targetDepth < depth) {
		depth = targetDepth
	}
	return RCAEvidenceEdge{
		Source:              edge.Source,
		Target:              edge.Target,
		SourceName:          firstNonBlank(source.Name, serviceName(edge.Source)),
		TargetName:          firstNonBlank(target.Name, serviceName(edge.Target)),
		Direction:           edge.Direction,
		Status:              status,
		Connectivity:        connectivity,
		ConnectivityMessage: message,
		Stats:               append([]string(nil), edge.Stats...),
		Depth:               depth,
	}
}

func topologyDistancesAndParents(appID string, nodes []TopologyNode, edges []TopologyEdge) (map[string]int, map[string]string) {
	start := topologyCenterID(appID, nodes)
	if start == "" {
		start = appID
	}
	adj := map[string][]string{}
	for _, edge := range edges {
		if edge.Source == "" || edge.Target == "" {
			continue
		}
		adj[edge.Source] = append(adj[edge.Source], edge.Target)
		adj[edge.Target] = append(adj[edge.Target], edge.Source)
	}
	distances := map[string]int{start: 0}
	parents := map[string]string{}
	queue := []string{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, next := range adj[id] {
			if _, seen := distances[next]; seen {
				continue
			}
			distances[next] = distances[id] + 1
			parents[next] = id
			queue = append(queue, next)
		}
	}
	for _, node := range nodes {
		if _, ok := distances[node.ID]; !ok {
			distances[node.ID] = -1
		}
	}
	return distances, parents
}

func rcaPathToTarget(appID, suspect string, parents map[string]string) []string {
	appID = strings.TrimSpace(appID)
	suspect = strings.TrimSpace(suspect)
	if appID == "" || suspect == "" {
		return nil
	}
	path := []string{suspect}
	for current := suspect; current != appID; {
		parent := parents[current]
		if parent == "" {
			return nil
		}
		path = append(path, parent)
		current = parent
		if len(path) > maxCorootModelTopologyItems {
			return nil
		}
	}
	return path
}

func rcaServiceNames(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, firstNonBlank(serviceName(id), id))
	}
	return out
}

func rcaHypothesesFromEvidence(
	target RCAContextTarget,
	slos []SLOStatus,
	edgeEvidence []RCAEdgeEvidence,
	graph *RCAEvidenceGraph,
	logSummary *CorootLogSummary,
	traceSummary *CorootTraceSummary,
	deploymentEvents []CorootDeploymentEvent,
	limitations []string,
) []RCAHypothesis {
	var hypotheses []RCAHypothesis
	targetName := firstNonBlank(target.ServiceName, serviceName(target.Service), target.Service)
	for _, edge := range edgeEvidence {
		if edge.Score <= 0 {
			continue
		}
		score := edge.Score
		evidence := append([]string(nil), edge.Signals...)
		rootCauseStatus := "candidate"
		if edge.Status != "" {
			evidence = appendCorootUniqueString(evidence, "edge status "+edge.Status, 10)
		}
		if edge.TargetKind == "external" {
			rootCauseStatus = "requires_external_dependency_drilldown"
			endpoint := firstNonBlank(edge.TargetEndpoint, edge.TargetName, serviceName(edge.Target), edge.Target)
			evidence = appendCorootUniqueString(evidence, "external dependency "+endpoint+" has not been resolved to its owner, endpoint health, port/protocol, or network path cause", 10)
		}
		if hasViolatedSLO(slos) {
			score += 15
			evidence = appendCorootUniqueString(evidence, "target service has violated SLO", 10)
		}
		if logEvidenceForEdge(logSummary, edge) {
			score += 20
			evidence = appendCorootUniqueString(evidence, "matched logs mention "+firstNonBlank(edge.TargetName, edge.Target), 10)
		}
		if traceEvidenceForEdge(traceSummary, edge) {
			score += 25
			evidence = appendCorootUniqueString(evidence, "trace spans link "+firstNonBlank(edge.SourceName, edge.Source)+" and "+firstNonBlank(edge.TargetName, edge.Target), 10)
		}
		path := rcaHypothesisPath(graph, edge)
		title := fmt.Sprintf("%s -> %s dependency is a likely root cause for %s symptoms", firstNonBlank(edge.SourceName, serviceName(edge.Source), edge.Source), firstNonBlank(edge.TargetName, serviceName(edge.Target), edge.Target), targetName)
		if edge.TargetKind == "external" {
			title = fmt.Sprintf("%s -> external dependency %s is failing and requires deeper cause analysis for %s symptoms", firstNonBlank(edge.SourceName, serviceName(edge.Source), edge.Source), firstNonBlank(edge.TargetEndpoint, edge.TargetName, serviceName(edge.Target), edge.Target), targetName)
		}
		hypotheses = append(hypotheses, RCAHypothesis{
			Title:           title,
			SuspectService:  edge.Target,
			SuspectEdge:     edge.Source + "->" + edge.Target,
			RootCauseStatus: rootCauseStatus,
			Confidence:      rcaConfidence(score, true),
			Score:           score,
			Evidence:        evidence,
			PropagationPath: path,
			NextDrilldowns:  rcaNextDrilldowns(edge),
		})
	}
	for _, event := range deploymentEvents {
		if !isProblemStatus(event.Status) && len(event.Summary) == 0 {
			continue
		}
		score := 25
		evidence := []string{}
		if event.Version != "" {
			evidence = append(evidence, "deployment version "+event.Version)
		}
		if event.Status != "" {
			evidence = append(evidence, "deployment status "+event.Status)
		}
		evidence = append(evidence, event.Summary...)
		hypotheses = append(hypotheses, RCAHypothesis{
			Title:          "recent deployment on " + firstNonBlank(event.Application, serviceName(event.ApplicationID), targetName) + " may explain the regression",
			SuspectService: firstNonBlank(event.ApplicationID, target.Service),
			Confidence:     rcaConfidence(score, false),
			Score:          score,
			Evidence:       evidence,
			NextDrilldowns: []string{"coroot.deployments_overview", "coroot.service_metrics"},
		})
	}
	if len(hypotheses) == 0 && hasViolatedSLO(slos) {
		hypotheses = append(hypotheses, RCAHypothesis{
			Title:           "target service SLO is violated but Coroot did not expose a stronger dependency root cause",
			SuspectService:  target.Service,
			Confidence:      "low",
			Score:           20,
			Evidence:        []string{"target service has SLO or health symptoms"},
			CounterEvidence: append([]string(nil), limitations...),
			NextDrilldowns:  []string{"coroot.service_metrics", "coroot.application_logs", "coroot.application_traces"},
		})
	}
	sort.Slice(hypotheses, func(i, j int) bool {
		if hypotheses[i].Score == hypotheses[j].Score {
			return hypotheses[i].Title < hypotheses[j].Title
		}
		return hypotheses[i].Score > hypotheses[j].Score
	})
	if len(hypotheses) > 5 {
		hypotheses = hypotheses[:5]
	}
	for i := range hypotheses {
		hypotheses[i].Rank = i + 1
		hypotheses[i].Confidence = rcaConfidence(hypotheses[i].Score, hypotheses[i].SuspectEdge != "")
	}
	return hypotheses
}

func hasViolatedSLO(slos []SLOStatus) bool {
	for _, slo := range slos {
		if slo.Violated {
			return true
		}
	}
	return false
}

func logEvidenceForEdge(summary *CorootLogSummary, edge RCAEdgeEvidence) bool {
	if summary == nil {
		return false
	}
	for _, entry := range summary.Entries {
		haystack := strings.ToLower(strings.Join([]string{entry.Application, entry.Message}, " "))
		for _, needle := range []string{edge.Source, edge.SourceName, edge.Target, edge.TargetName} {
			needle = strings.ToLower(strings.TrimSpace(firstNonBlank(serviceName(needle), needle)))
			if needle != "" && strings.Contains(haystack, needle) {
				return true
			}
		}
	}
	return false
}

func traceEvidenceForEdge(summary *CorootTraceSummary, edge RCAEdgeEvidence) bool {
	if summary == nil {
		return false
	}
	linkedSource := false
	linkedTarget := false
	for _, service := range summary.LinkedServices {
		if serviceMatches(edge.Source, service) || serviceMatches(edge.SourceName, service) || serviceMatches(service, edge.SourceName) {
			linkedSource = true
		}
		if serviceMatches(edge.Target, service) || serviceMatches(edge.TargetName, service) || serviceMatches(service, edge.TargetName) {
			linkedTarget = true
		}
	}
	if linkedSource && linkedTarget {
		return true
	}
	for _, span := range summary.SlowestSpans {
		haystack := strings.ToLower(strings.Join([]string{span.Service, span.Client, span.Name}, " "))
		if strings.Contains(haystack, strings.ToLower(edge.SourceName)) && strings.Contains(haystack, strings.ToLower(edge.TargetName)) {
			return true
		}
	}
	return false
}

func rcaHypothesisPath(graph *RCAEvidenceGraph, edge RCAEdgeEvidence) []string {
	if graph == nil {
		return nil
	}
	for _, path := range graph.Paths {
		if path.From == edge.Source || path.From == edge.Target {
			return append([]string(nil), path.ServiceNames...)
		}
	}
	return nil
}

func rcaConfidence(score int, hasEdge bool) string {
	switch {
	case score >= 70 && hasEdge:
		return "high"
	case score >= 40:
		return "medium"
	default:
		return "low"
	}
}

func rcaNextDrilldowns(edge RCAEdgeEvidence) []string {
	if edge.TargetKind == "external" {
		endpoint := firstNonBlank(edge.TargetEndpoint, edge.TargetName, serviceName(edge.Target), edge.Target)
		source := firstNonBlank(edge.SourceName, serviceName(edge.Source), edge.Source)
		return []string{
			"resolve external dependency " + endpoint + " to the actual owner/service before treating it as the root cause",
			"check whether " + endpoint + " is a Kubernetes Service or Endpoint and inspect its backing endpoints/readiness",
			"identify the expected port/protocol for " + endpoint + " and compare it with the failed connection stats",
			"verify caller-to-dependency network path from " + source + " to " + endpoint + " (DNS, route, NetworkPolicy/firewall, connection refused/reset/timeout)",
			"coroot.service_topology",
			"coroot.service_metrics",
			"coroot.application_logs",
			"coroot.application_traces",
		}
	}
	return []string{
		"coroot.service_topology",
		"coroot.service_metrics",
		"coroot.application_logs",
		"coroot.application_traces",
	}
}

func enrichRCAContextSummaryWithHypotheses(summary RCAContextSummary, hypotheses []RCAHypothesis) RCAContextSummary {
	for _, hypothesis := range hypotheses {
		name := firstNonBlank(serviceName(hypothesis.SuspectService), hypothesis.SuspectService)
		if name != "" {
			summary.PrimarySuspects = appendCorootUniqueString(summary.PrimarySuspects, name, 5)
		}
		if hypothesis.Title != "" {
			summary.TopSignals = appendCorootUniqueString(summary.TopSignals, hypothesis.Title, 12)
		}
		if hypothesis.RootCauseStatus == "requires_external_dependency_drilldown" {
			summary.MissingEvidence = appendCorootUniqueString(
				summary.MissingEvidence,
				"external dependency "+name+" identity, endpoint health, port/protocol, and underlying cause is unresolved; do not treat the dependency edge alone as the final root cause",
				6,
			)
		}
	}
	return summary
}

func summarizeRCAContext(
	target RCAContextTarget,
	slos []SLOStatus,
	chartSummary CorootChartSummary,
	deps TopologyDependencyGroups,
	abnormal []TopologyDependencySummary,
	logSummary *CorootLogSummary,
	traceSummary *CorootTraceSummary,
	profilingSummary *CorootProfilingSummary,
	deploymentEvents []CorootDeploymentEvent,
	incidents []IncidentSummary,
	limitations []string,
) RCAContextSummary {
	const maxRCAContextTopSignals = 12
	summary := RCAContextSummary{Health: firstNonBlank(target.Status, "unknown")}
	if isProblemStatus(target.Status) {
		summary.TopSignals = append(summary.TopSignals, fmt.Sprintf("target service %s status is %s", firstNonBlank(target.ServiceName, target.Service), target.Status))
	}
	for _, slo := range slos {
		if !slo.Violated {
			continue
		}
		signal := fmt.Sprintf("%s SLO is %s", slo.Name, firstNonBlank(slo.Status, "violated"))
		if slo.Value != "" {
			signal += " (" + slo.Value + ")"
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	for _, metric := range chartSummary.MetricSummaries {
		if !isProblemStatus(metric.Status) {
			continue
		}
		signal := fmt.Sprintf("%s metric is %s", firstNonBlank(metric.Name, metric.ChartTitle), metric.Status)
		if metric.Value != "" {
			signal += " (" + metric.Value + ")"
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	for _, report := range chartSummary.Reports {
		if !isProblemStatus(report.Status) {
			continue
		}
		signal := fmt.Sprintf("%s report is %s", report.Name, report.Status)
		if len(report.Titles) > 0 {
			signal += " (" + strings.Join(report.Titles, "; ") + ")"
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	for _, item := range abnormal {
		name := firstNonBlank(item.Name, serviceName(item.ID), item.ID)
		relation := topologyRelationLabel(item, deps)
		signal := fmt.Sprintf("%s dependency %s is %s", relation, name, firstNonBlank(item.Status, "abnormal"))
		if len(item.Stats) > 0 {
			signal += " (" + strings.Join(item.Stats, ", ") + ")"
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
		summary.PrimarySuspects = appendCorootUniqueString(summary.PrimarySuspects, name, 5)
	}
	if logSummary != nil && logSummary.ErrorLikeCount > 0 {
		signal := fmt.Sprintf("matched %d error-like logs", logSummary.ErrorLikeCount)
		if len(logSummary.Entries) > 0 && logSummary.Entries[0].Message != "" {
			signal += ": " + logSummary.Entries[0].Message
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	if traceSummary != nil && traceSummary.ErrorSpanCount > 0 {
		signal := fmt.Sprintf("trace errors found in %d of %d spans", traceSummary.ErrorSpanCount, traceSummary.SpanCount)
		if len(traceSummary.SlowestSpans) > 0 && traceSummary.SlowestSpans[0].Name != "" {
			signal += fmt.Sprintf("; slowest span %s %.0fms", traceSummary.SlowestSpans[0].Name, traceSummary.SlowestSpans[0].DurationMS)
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	if profilingSummary != nil && isProblemStatus(profilingSummary.Status) {
		signal := "profiling status is " + profilingSummary.Status
		if profilingSummary.Message != "" {
			signal += ": " + profilingSummary.Message
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	for _, event := range deploymentEvents {
		if !isProblemStatus(event.Status) && len(event.Summary) == 0 {
			continue
		}
		signal := "deployment"
		if event.Version != "" {
			signal += " " + event.Version
		}
		if event.Status != "" {
			signal += " is " + event.Status
		}
		if len(event.Summary) > 0 {
			signal += ": " + strings.Join(event.Summary, "; ")
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	for _, incident := range incidents {
		description := firstNonBlank(incident.Description, incident.RootCause)
		if description == "" {
			continue
		}
		signal := "recent incident " + incident.ID + ": " + description
		if incident.Severity != "" {
			signal += " (" + incident.Severity + ")"
		}
		summary.TopSignals = appendCorootUniqueString(summary.TopSignals, signal, maxRCAContextTopSignals)
	}
	for _, limitation := range limitations {
		summary.MissingEvidence = appendCorootUniqueString(summary.MissingEvidence, limitation, 6)
	}
	if len(summary.TopSignals) == 0 {
		summary.TopSignals = append(summary.TopSignals, "Coroot aggregate context did not find a strong abnormal signal for the target service.")
	}
	return summary
}

func topologyRelationLabel(item TopologyDependencySummary, deps TopologyDependencyGroups) string {
	for _, dep := range deps.Upstream {
		if dep.ID == item.ID {
			return "upstream"
		}
	}
	for _, dep := range deps.Downstream {
		if dep.ID == item.ID {
			return "downstream"
		}
	}
	if item.Direction != "" {
		return item.Direction
	}
	return "related"
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

func chartSummaryFromServiceMetrics(service string, metrics []MetricSummary, reports []CorootChartReport) CorootChartSummary {
	summary := CorootChartSummary{Service: strings.TrimSpace(service)}
	for _, metric := range metrics {
		item := CorootMetricChartSummary{
			Name:       metric.Name,
			Topic:      corootChartTopicFromName(firstNonBlank(metric.Name, metric.ChartTitle)),
			Status:     metric.Status,
			Value:      firstNonBlank(metric.Value, latestMetricValue(metric.Series), latestMetricValue([]MetricSeries{{Values: metric.Values}})),
			Unit:       metric.Unit,
			ChartTitle: metric.ChartTitle,
		}
		if len(metric.Series) > 0 {
			item.SeriesCount = len(metric.Series)
			for _, series := range metric.Series {
				item.PointCount += len(series.Values)
				item.SeriesNames = appendCorootUniqueString(item.SeriesNames, series.Name, 5)
			}
		} else if len(metric.Values) > 0 {
			item.SeriesCount = 1
			item.PointCount = len(metric.Values)
		}
		summary.MetricSummaries = append(summary.MetricSummaries, item)
	}
	for _, report := range reports {
		item := CorootReportChartSummary{
			Name:   report.Name,
			Topic:  corootChartTopicFromName(report.Name),
			Status: report.Status,
		}
		for _, widget := range report.Widgets {
			if chart := objectField(widget, "chart"); len(chart) > 0 {
				addCorootChartToReportSummary(&item, chart, firstNonBlank(stringFromAny(widget["title"]), stringFromAny(chart["title"])))
			}
			group := objectField(widget, "chart_group")
			if len(group) == 0 {
				continue
			}
			groupTitle := stringFromAny(group["title"])
			for _, chart := range objectSlice(group["charts"]) {
				addCorootChartToReportSummary(&item, chart, firstNonBlank(groupTitle, stringFromAny(chart["title"])))
			}
		}
		summary.Reports = append(summary.Reports, item)
	}
	return summary
}

func addCorootChartToReportSummary(summary *CorootReportChartSummary, chart map[string]any, title string) {
	if summary == nil || len(chart) == 0 {
		return
	}
	if summary.Topic == "" {
		summary.Topic = corootChartTopicFromName(firstNonBlank(title, stringFromAny(chart["title"])))
	}
	summary.ChartCount++
	summary.Titles = appendCorootUniqueString(summary.Titles, firstNonBlank(title, stringFromAny(chart["title"])), 5)
	for _, series := range objectSlice(chart["series"]) {
		summary.SeriesCount++
		summary.PointCount += len(transportAnySliceForCoroot(series["data"]))
		summary.SeriesNames = appendCorootUniqueString(summary.SeriesNames, stringFromAny(series["name"]), 5)
	}
	if threshold := objectField(chart, "threshold"); len(threshold) > 0 {
		summary.PointCount += len(transportAnySliceForCoroot(threshold["data"]))
	}
}

func corootChartTopicFromName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(normalized, "net"), strings.Contains(normalized, "network"), strings.Contains(normalized, "tcp"):
		return "net"
	case strings.Contains(normalized, "cpu"):
		return "cpu"
	case strings.Contains(normalized, "memory"), strings.Contains(normalized, "mem"), strings.Contains(normalized, "rss"):
		return "memory"
	case strings.Contains(normalized, "instances"), strings.Contains(normalized, "instance"):
		return "instances"
	default:
		return ""
	}
}

func appendCorootUniqueString(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || (limit > 0 && len(values) >= limit) {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
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

func intFromAny(value any) int {
	return int(int64FromAny(value))
}

func anyStringSlice(value any) []string {
	return stringSlice(value)
}

func truncateCorootText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
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
			mergeTopologyEdge(edgeSet, edge)
		}
		for _, downstream := range objectSlice(app["downstreams"]) {
			source := stringFromAny(downstream["id"])
			if _, ok := selected[source]; !ok {
				continue
			}
			edge := TopologyEdge{Source: source, Target: id, Direction: "downstream", Status: stringFromAny(downstream["status"]), Stats: stringSlice(downstream["stats"])}
			mergeTopologyEdge(edgeSet, edge)
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

func mergeTopologyEdge(edges map[string]TopologyEdge, edge TopologyEdge) {
	if edges == nil || edge.Source == "" || edge.Target == "" {
		return
	}
	key := edge.Source + "->" + edge.Target
	existing, ok := edges[key]
	if !ok {
		edges[key] = edge
		return
	}
	existing.Status = strongestStatus(existing.Status, edge.Status)
	if existing.Direction == "" {
		existing.Direction = edge.Direction
	}
	for _, stat := range edge.Stats {
		existing.Stats = appendCorootUniqueString(existing.Stats, stat, 12)
	}
	edges[key] = existing
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

func incidentSummaryFromObject(obj map[string]any) IncidentSummary {
	key := strings.TrimPrefix(firstNonBlank(stringFromAny(obj["key"]), stringFromAny(obj["id"])), "i-")
	applicationID := stringFromAny(obj["application_id"])
	rca := objectField(obj, "rca")
	impact, _ := floatFromAny(firstNonNil(obj["impact"], nestedAny(obj, "details", "latency_impact", "percentage"), nestedAny(obj, "details", "availability_impact", "percentage")))
	return IncidentSummary{
		ID:                      corootIncidentDisplayID(key),
		Key:                     key,
		ApplicationID:           applicationID,
		Application:             serviceName(applicationID),
		ApplicationCategory:     stringFromAny(obj["application_category"]),
		Severity:                stringFromAny(obj["severity"]),
		State:                   corootIncidentState(obj),
		Description:             stringFromAny(obj["short_description"]),
		RCAStatus:               stringFromAny(rca["status"]),
		RootCause:               firstNonBlank(stringFromAny(rca["root_cause"]), stringFromAny(rca["short_summary"])),
		ImpactedRequestsPercent: impact,
		OpenedAt:                corootTimestampString(obj["opened_at"]),
		ResolvedAt:              corootTimestampString(obj["resolved_at"]),
		DurationMs:              int64FromAny(obj["duration"]),
	}
}

func corootIncidentMatches(incident IncidentSummary, in corootInput) bool {
	if in.Service != "" && !serviceMatches(incident.ApplicationID, in.Service) {
		return false
	}
	if in.ApplicationCategory != "" && !strings.EqualFold(incident.ApplicationCategory, in.ApplicationCategory) {
		return false
	}
	if in.Severity != "" && !strings.EqualFold(incident.Severity, in.Severity) {
		return false
	}
	if in.Status != "" {
		status := strings.ToLower(strings.TrimSpace(in.Status))
		switch status {
		case "open":
			if (in.ShowResolved == nil || !*in.ShowResolved) && incident.State == "resolved" {
				return false
			}
		case "resolved":
			if incident.State != "resolved" {
				return false
			}
		default:
			if !strings.EqualFold(incident.Severity, status) && !strings.EqualFold(incident.State, status) {
				return false
			}
		}
	}
	if in.ShowResolved != nil && !*in.ShowResolved && incident.State == "resolved" {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(in.Query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		incident.ID,
		incident.Key,
		incident.ApplicationID,
		incident.Application,
		incident.ApplicationCategory,
		incident.Description,
		incident.Severity,
		incident.State,
		incident.RCAStatus,
		incident.RootCause,
	}, " "))
	return strings.Contains(haystack, query)
}

func corootIncidentDisplayID(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "i-") {
		return key
	}
	return "i-" + key
}

func corootIncidentState(obj map[string]any) string {
	if int64FromAny(obj["resolved_at"]) > 0 {
		return "resolved"
	}
	if severity := strings.ToLower(stringFromAny(obj["severity"])); severity != "" {
		return severity
	}
	return "open"
}

func corootTimestampString(value any) string {
	ts := int64FromAny(value)
	if ts <= 0 {
		return ""
	}
	if ts > 1_000_000_000_000 {
		return time.UnixMilli(ts).UTC().Format(time.RFC3339)
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func nestedAny(obj map[string]any, keys ...string) any {
	var current any = obj
	for _, key := range keys {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	return current
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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

func unavailableNativeRCAReport(project, service, incidentID string, rawRef *CorootRawRef) RCAReportResult {
	return RCAReportResult{
		SchemaVersion: corootSchemaVersion,
		Tool:          "coroot.rca_report",
		Status:        "unavailable",
		Project:       project,
		Service:       service,
		IncidentID:    incidentID,
		Summary:       "Optional native RCA from Coroot is unavailable for this application; this does not prove the service is absent.",
		DetailedAnalysis: "The application was resolved from the Coroot overview list, but Coroot's native RCA endpoint returned 404 Application not found. " +
			"Continue service RCA with coroot.collect_rca_context, metrics, topology, logs, traces, and incidents.",
		RelatedServices: uniqueStrings([]string{service}),
		RawRef:          rawRef,
	}
}

func objectArray(raw json.RawMessage, keys ...string) []map[string]any {
	var direct []map[string]any
	if err := json.Unmarshal(raw, &direct); err == nil && len(direct) > 0 {
		return direct
	}
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

func sanitizeCorootData(raw json.RawMessage) any {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{"summary": truncateCorootText(string(raw), 500)}
	}
	return redactCorootSecrets(value)
}

func rawDataMap(raw json.RawMessage) map[string]any {
	obj := firstObject(raw)
	if data := objectField(obj, "data"); len(data) > 0 {
		return data
	}
	return obj
}

func redactCorootSecrets(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if corootKeyLooksSecret(key) {
				if stringFromAny(item) == "" {
					out[key] = nil
				} else {
					out[key] = "[redacted]"
				}
				continue
			}
			out[key] = redactCorootSecrets(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactCorootSecrets(item))
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactCorootSecrets(item))
		}
		return out
	default:
		return typed
	}
}

func corootKeyLooksSecret(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "access_key", "private_key", "client_secret", "authorization", "bearer", "cookie"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
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
