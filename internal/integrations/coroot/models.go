package coroot

const corootSchemaVersion = "aiops.coroot/v1"

type CorootRawRef struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}

type CorootRawRefSummary struct {
	Purpose string        `json:"purpose"`
	RawRef  *CorootRawRef `json:"rawRef,omitempty"`
}

type CorootErrorPayload struct {
	Kind       string `json:"kind"`
	StatusCode int    `json:"statusCode,omitempty"`
	URI        string `json:"uri,omitempty"`
	Message    string `json:"message,omitempty"`
}

type corootErrorResult struct {
	SchemaVersion string             `json:"schemaVersion"`
	Tool          string             `json:"tool"`
	Status        string             `json:"status"`
	Error         CorootErrorPayload `json:"error"`
	RawRef        *CorootRawRef      `json:"rawRef,omitempty"`
}

type ServiceSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Cluster  string `json:"cluster,omitempty"`
	Category string `json:"category,omitempty"`
	Status   string `json:"status,omitempty"`
}

type ListServicesResult struct {
	SchemaVersion string           `json:"schemaVersion"`
	Tool          string           `json:"tool"`
	Status        string           `json:"status"`
	Project       string           `json:"project"`
	Services      []ServiceSummary `json:"services"`
	RawRef        *CorootRawRef    `json:"rawRef,omitempty"`
}

type MetricSummary struct {
	Name       string         `json:"name"`
	Status     string         `json:"status,omitempty"`
	Value      string         `json:"value,omitempty"`
	Unit       string         `json:"unit,omitempty"`
	ChartTitle string         `json:"chartTitle,omitempty"`
	Values     [][]float64    `json:"values,omitempty"`
	Series     []MetricSeries `json:"series,omitempty"`
}

type MetricSeries struct {
	Name   string      `json:"name,omitempty"`
	Value  string      `json:"value,omitempty"`
	Values [][]float64 `json:"values,omitempty"`
}

type ServiceMetricsResult struct {
	SchemaVersion string              `json:"schemaVersion"`
	Tool          string              `json:"tool"`
	Status        string              `json:"status"`
	Project       string              `json:"project"`
	Service       string              `json:"service"`
	Metrics       []MetricSummary     `json:"metrics"`
	ChartReports  []CorootChartReport `json:"chartReports,omitempty"`
	ChartSummary  CorootChartSummary  `json:"chartSummary,omitempty"`
	RawRef        *CorootRawRef       `json:"rawRef,omitempty"`
}

type ServiceMetricsModelResult struct {
	SchemaVersion   string                     `json:"schemaVersion"`
	Tool            string                     `json:"tool"`
	Status          string                     `json:"status"`
	Project         string                     `json:"project"`
	Service         string                     `json:"service"`
	MetricSummaries []CorootMetricChartSummary `json:"metricSummaries,omitempty"`
	ChartSummary    CorootChartSummary         `json:"chartSummary,omitempty"`
	RawRef          *CorootRawRef              `json:"rawRef,omitempty"`
}

type RCAContextTarget struct {
	Service     string `json:"service,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Cluster     string `json:"cluster,omitempty"`
	Category    string `json:"category,omitempty"`
	Status      string `json:"status,omitempty"`
	TimeRange   string `json:"timeRange,omitempty"`
	IncidentID  string `json:"incidentId,omitempty"`
}

type RCAContextSummary struct {
	Health          string   `json:"health,omitempty"`
	TopSignals      []string `json:"topSignals,omitempty"`
	PrimarySuspects []string `json:"primarySuspects,omitempty"`
	MissingEvidence []string `json:"missingEvidence,omitempty"`
}

type RCAContextTimeWindow struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type RCAEvidenceGraph struct {
	Nodes     []RCAEvidenceNode `json:"nodes,omitempty"`
	Edges     []RCAEvidenceEdge `json:"edges,omitempty"`
	Paths     []RCAEvidencePath `json:"paths,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

type RCAEvidenceNode struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Cluster  string `json:"cluster,omitempty"`
	Category string `json:"category,omitempty"`
	Status   string `json:"status,omitempty"`
	Distance int    `json:"distance,omitempty"`
}

type RCAEvidenceEdge struct {
	Source              string   `json:"source"`
	Target              string   `json:"target"`
	SourceName          string   `json:"sourceName,omitempty"`
	TargetName          string   `json:"targetName,omitempty"`
	Direction           string   `json:"direction,omitempty"`
	Status              string   `json:"status,omitempty"`
	Connectivity        string   `json:"connectivity,omitempty"`
	ConnectivityMessage string   `json:"connectivityMessage,omitempty"`
	Stats               []string `json:"stats,omitempty"`
	Depth               int      `json:"depth,omitempty"`
}

type RCAEvidencePath struct {
	From         string   `json:"from,omitempty"`
	To           string   `json:"to,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	Services     []string `json:"services,omitempty"`
	ServiceNames []string `json:"serviceNames,omitempty"`
}

type RCAEdgeEvidence struct {
	Source              string   `json:"source"`
	Target              string   `json:"target"`
	SourceName          string   `json:"sourceName,omitempty"`
	TargetName          string   `json:"targetName,omitempty"`
	Status              string   `json:"status,omitempty"`
	Connectivity        string   `json:"connectivity,omitempty"`
	ConnectivityMessage string   `json:"connectivityMessage,omitempty"`
	Stats               []string `json:"stats,omitempty"`
	Signals             []string `json:"signals,omitempty"`
	Score               int      `json:"score,omitempty"`
}

type RCAHypothesis struct {
	Rank            int      `json:"rank"`
	Title           string   `json:"title"`
	SuspectService  string   `json:"suspectService,omitempty"`
	SuspectEdge     string   `json:"suspectEdge,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	Score           int      `json:"score,omitempty"`
	Evidence        []string `json:"evidence,omitempty"`
	CounterEvidence []string `json:"counterEvidence,omitempty"`
	PropagationPath []string `json:"propagationPath,omitempty"`
	NextDrilldowns  []string `json:"nextDrilldowns,omitempty"`
}

type CorootLogSummary struct {
	TotalCount     int              `json:"totalCount"`
	MatchedCount   int              `json:"matchedCount"`
	ErrorLikeCount int              `json:"errorLikeCount"`
	Entries        []CorootLogEntry `json:"entries,omitempty"`
	Applications   []string         `json:"applications,omitempty"`
	Severities     map[string]int   `json:"severities,omitempty"`
}

type CorootLogEntry struct {
	Application string `json:"application,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Message     string `json:"message,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

type CorootTraceSummary struct {
	Status         string            `json:"status,omitempty"`
	Message        string            `json:"message,omitempty"`
	SpanCount      int               `json:"spanCount"`
	ErrorSpanCount int               `json:"errorSpanCount"`
	Limit          int               `json:"limit,omitempty"`
	Sources        []string          `json:"sources,omitempty"`
	LinkedServices []string          `json:"linkedServices,omitempty"`
	SlowestSpans   []CorootTraceSpan `json:"slowestSpans,omitempty"`
}

type CorootTraceSpan struct {
	Service    string  `json:"service,omitempty"`
	Client     string  `json:"client,omitempty"`
	Name       string  `json:"name,omitempty"`
	Status     string  `json:"status,omitempty"`
	TraceID    string  `json:"traceId,omitempty"`
	DurationMS float64 `json:"durationMs,omitempty"`
}

type CorootProfilingSummary struct {
	Status         string   `json:"status,omitempty"`
	Message        string   `json:"message,omitempty"`
	ProfileCount   int      `json:"profileCount"`
	InstanceCount  int      `json:"instanceCount"`
	LinkedServices []string `json:"linkedServices,omitempty"`
	Profiles       []string `json:"profiles,omitempty"`
	Instances      []string `json:"instances,omitempty"`
}

type CorootDeploymentEvent struct {
	ApplicationID string   `json:"applicationId,omitempty"`
	Application   string   `json:"application,omitempty"`
	Category      string   `json:"category,omitempty"`
	Version       string   `json:"version,omitempty"`
	Deployed      string   `json:"deployed,omitempty"`
	Status        string   `json:"status,omitempty"`
	Age           string   `json:"age,omitempty"`
	Summary       []string `json:"summary,omitempty"`
}

type RCAContextResult struct {
	SchemaVersion    string                      `json:"schemaVersion"`
	Tool             string                      `json:"tool"`
	Status           string                      `json:"status"`
	Project          string                      `json:"project"`
	Target           RCAContextTarget            `json:"target"`
	Summary          RCAContextSummary           `json:"summary"`
	TimeWindow       RCAContextTimeWindow        `json:"timeWindow,omitempty"`
	SLOs             []SLOStatus                 `json:"slos,omitempty"`
	MetricSummaries  []CorootMetricChartSummary  `json:"metricSummaries,omitempty"`
	ReportSummaries  []CorootReportChartSummary  `json:"reportSummaries,omitempty"`
	Dependencies     TopologyDependencyGroups    `json:"dependencies,omitempty"`
	RelatedServices  []TopologyDependencySummary `json:"relatedServices,omitempty"`
	AbnormalServices []TopologyDependencySummary `json:"abnormalServices,omitempty"`
	EvidenceGraph    *RCAEvidenceGraph           `json:"evidenceGraph,omitempty"`
	EdgeEvidence     []RCAEdgeEvidence           `json:"edgeEvidence,omitempty"`
	Hypotheses       []RCAHypothesis             `json:"hypotheses,omitempty"`
	LogSummary       *CorootLogSummary           `json:"logSummary,omitempty"`
	TraceSummary     *CorootTraceSummary         `json:"traceSummary,omitempty"`
	ProfilingSummary *CorootProfilingSummary     `json:"profilingSummary,omitempty"`
	DeploymentEvents []CorootDeploymentEvent     `json:"deploymentEvents,omitempty"`
	RecentIncidents  []IncidentSummary           `json:"recentIncidents,omitempty"`
	ReferenceRCA     *RCAReportResult            `json:"referenceRca,omitempty"`
	Limitations      []string                    `json:"limitations,omitempty"`
	RawRefs          []CorootRawRefSummary       `json:"rawRefs,omitempty"`
}

type CorootChartSummary struct {
	Service         string                     `json:"service,omitempty"`
	MetricSummaries []CorootMetricChartSummary `json:"metricSummaries,omitempty"`
	Reports         []CorootReportChartSummary `json:"reports,omitempty"`
}

type CorootMetricChartSummary struct {
	Name        string   `json:"name"`
	Topic       string   `json:"topic,omitempty"`
	Status      string   `json:"status,omitempty"`
	Value       string   `json:"value,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	ChartTitle  string   `json:"chartTitle,omitempty"`
	SeriesCount int      `json:"seriesCount,omitempty"`
	PointCount  int      `json:"pointCount,omitempty"`
	SeriesNames []string `json:"seriesNames,omitempty"`
}

type CorootReportChartSummary struct {
	Name        string   `json:"name"`
	Topic       string   `json:"topic,omitempty"`
	Status      string   `json:"status,omitempty"`
	ChartCount  int      `json:"chartCount,omitempty"`
	SeriesCount int      `json:"seriesCount,omitempty"`
	PointCount  int      `json:"pointCount,omitempty"`
	Titles      []string `json:"titles,omitempty"`
	SeriesNames []string `json:"seriesNames,omitempty"`
}

type CorootChartReport struct {
	Name    string           `json:"name"`
	Status  string           `json:"status,omitempty"`
	Widgets []map[string]any `json:"widgets"`
}

type SLOStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status,omitempty"`
	Value    string `json:"value,omitempty"`
	Violated bool   `json:"violated"`
}

type SLOStatusResult struct {
	SchemaVersion string        `json:"schemaVersion"`
	Tool          string        `json:"tool"`
	Status        string        `json:"status"`
	Project       string        `json:"project"`
	Service       string        `json:"service,omitempty"`
	SLOName       string        `json:"sloName,omitempty"`
	SLOs          []SLOStatus   `json:"slos"`
	RawRef        *CorootRawRef `json:"rawRef,omitempty"`
}

type TopologyNode struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Cluster  string `json:"cluster,omitempty"`
	Category string `json:"category,omitempty"`
	Status   string `json:"status,omitempty"`
}

type TopologyEdge struct {
	Source    string   `json:"source"`
	Target    string   `json:"target"`
	Direction string   `json:"direction"`
	Status    string   `json:"status,omitempty"`
	Stats     []string `json:"stats,omitempty"`
}

type ServiceTopologyResult struct {
	SchemaVersion string         `json:"schemaVersion"`
	Tool          string         `json:"tool"`
	Status        string         `json:"status"`
	Project       string         `json:"project"`
	Service       string         `json:"service"`
	Depth         int            `json:"depth"`
	Nodes         []TopologyNode `json:"nodes"`
	Edges         []TopologyEdge `json:"edges"`
	RawRef        *CorootRawRef  `json:"rawRef,omitempty"`
}

type TopologyDependencyGroups struct {
	Upstream   []TopologyDependencySummary `json:"upstream,omitempty"`
	Downstream []TopologyDependencySummary `json:"downstream,omitempty"`
}

type TopologyDependencySummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name,omitempty"`
	Cluster   string   `json:"cluster,omitempty"`
	Category  string   `json:"category,omitempty"`
	Status    string   `json:"status,omitempty"`
	Direction string   `json:"direction,omitempty"`
	Stats     []string `json:"stats,omitempty"`
}

type ServiceTopologyModelResult struct {
	SchemaVersion    string                      `json:"schemaVersion"`
	Tool             string                      `json:"tool"`
	Status           string                      `json:"status"`
	Project          string                      `json:"project"`
	Service          string                      `json:"service"`
	ServiceName      string                      `json:"serviceName,omitempty"`
	Depth            int                         `json:"depth"`
	NodeCount        int                         `json:"nodeCount"`
	EdgeCount        int                         `json:"edgeCount"`
	Dependencies     TopologyDependencyGroups    `json:"dependencies"`
	RelatedServices  []TopologyDependencySummary `json:"relatedServices,omitempty"`
	AbnormalServices []TopologyDependencySummary `json:"abnormalServices,omitempty"`
	Truncated        bool                        `json:"truncated,omitempty"`
	RawRef           *CorootRawRef               `json:"rawRef,omitempty"`
}

type TimelineEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp,omitempty"`
	Message   string `json:"message"`
	Severity  string `json:"severity,omitempty"`
	Service   string `json:"service,omitempty"`
}

type IncidentTimelineResult struct {
	SchemaVersion string          `json:"schemaVersion"`
	Tool          string          `json:"tool"`
	Status        string          `json:"status"`
	Project       string          `json:"project"`
	IncidentID    string          `json:"incidentId"`
	Service       string          `json:"service,omitempty"`
	Events        []TimelineEvent `json:"events"`
	RawRef        *CorootRawRef   `json:"rawRef,omitempty"`
}

type IncidentSummary struct {
	ID                      string  `json:"id"`
	Key                     string  `json:"key,omitempty"`
	ApplicationID           string  `json:"applicationId,omitempty"`
	Application             string  `json:"application,omitempty"`
	ApplicationCategory     string  `json:"applicationCategory,omitempty"`
	Severity                string  `json:"severity,omitempty"`
	State                   string  `json:"state,omitempty"`
	Description             string  `json:"description,omitempty"`
	RCAStatus               string  `json:"rcaStatus,omitempty"`
	RootCause               string  `json:"rootCause,omitempty"`
	ImpactedRequestsPercent float64 `json:"impactedRequestsPercent"`
	OpenedAt                string  `json:"openedAt,omitempty"`
	ResolvedAt              string  `json:"resolvedAt,omitempty"`
	DurationMs              int64   `json:"durationMs,omitempty"`
}

type IncidentsResult struct {
	SchemaVersion string            `json:"schemaVersion"`
	Tool          string            `json:"tool"`
	Status        string            `json:"status"`
	Project       string            `json:"project"`
	Incidents     []IncidentSummary `json:"incidents"`
	RawRef        *CorootRawRef     `json:"rawRef,omitempty"`
}

type RCAReportResult struct {
	SchemaVersion    string        `json:"schemaVersion"`
	Tool             string        `json:"tool"`
	Status           string        `json:"status"`
	Project          string        `json:"project"`
	Service          string        `json:"service,omitempty"`
	IncidentID       string        `json:"incidentId,omitempty"`
	Summary          string        `json:"summary,omitempty"`
	RootCause        string        `json:"rootCause,omitempty"`
	Remediations     string        `json:"remediations,omitempty"`
	DetailedAnalysis string        `json:"detailedAnalysis,omitempty"`
	RelatedServices  []string      `json:"relatedServices"`
	RawRef           *CorootRawRef `json:"rawRef,omitempty"`
}

type AlertRuleSummary struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description,omitempty"`
}

type AlertRulesResult struct {
	SchemaVersion string             `json:"schemaVersion"`
	Tool          string             `json:"tool"`
	Status        string             `json:"status"`
	Project       string             `json:"project"`
	Rules         []AlertRuleSummary `json:"rules"`
	RawRef        *CorootRawRef      `json:"rawRef,omitempty"`
}

type HealthCheckResult struct {
	SchemaVersion string        `json:"schemaVersion"`
	Tool          string        `json:"tool"`
	Status        string        `json:"status"`
	Healthy       bool          `json:"healthy"`
	Message       string        `json:"message,omitempty"`
	RawRef        *CorootRawRef `json:"rawRef,omitempty"`
}

type ProjectSummary struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Role string `json:"role,omitempty"`
}

type ProjectsResult struct {
	SchemaVersion string           `json:"schemaVersion"`
	Tool          string           `json:"tool"`
	Status        string           `json:"status"`
	Projects      []ProjectSummary `json:"projects"`
	RawRef        *CorootRawRef    `json:"rawRef,omitempty"`
}

type GenericCorootDataResult struct {
	SchemaVersion string        `json:"schemaVersion"`
	Tool          string        `json:"tool"`
	Status        string        `json:"status"`
	Project       string        `json:"project,omitempty"`
	Data          any           `json:"data,omitempty"`
	RawRef        *CorootRawRef `json:"rawRef,omitempty"`
}

type ApplicationLogsResult struct {
	SchemaVersion string           `json:"schemaVersion"`
	Tool          string           `json:"tool"`
	Status        string           `json:"status"`
	Project       string           `json:"project"`
	Service       string           `json:"service,omitempty"`
	Summary       CorootLogSummary `json:"summary"`
	RawRef        *CorootRawRef    `json:"rawRef,omitempty"`
}

type ApplicationTracesResult struct {
	SchemaVersion string             `json:"schemaVersion"`
	Tool          string             `json:"tool"`
	Status        string             `json:"status"`
	Project       string             `json:"project"`
	Service       string             `json:"service,omitempty"`
	TraceID       string             `json:"traceId,omitempty"`
	Summary       CorootTraceSummary `json:"summary"`
	RawRef        *CorootRawRef      `json:"rawRef,omitempty"`
}

type ApplicationProfilingResult struct {
	SchemaVersion string                 `json:"schemaVersion"`
	Tool          string                 `json:"tool"`
	Status        string                 `json:"status"`
	Project       string                 `json:"project"`
	Service       string                 `json:"service,omitempty"`
	Summary       CorootProfilingSummary `json:"summary"`
	RawRef        *CorootRawRef          `json:"rawRef,omitempty"`
}

type NodeSummary struct {
	ID              string   `json:"id,omitempty"`
	Name            string   `json:"name,omitempty"`
	Status          string   `json:"status,omitempty"`
	Cluster         string   `json:"cluster,omitempty"`
	Region          string   `json:"region,omitempty"`
	InstanceType    string   `json:"instanceType,omitempty"`
	Applications    []string `json:"applications,omitempty"`
	Summary         []string `json:"summary,omitempty"`
	ResourceSignals []string `json:"resourceSignals,omitempty"`
}

type NodeResult struct {
	SchemaVersion string        `json:"schemaVersion"`
	Tool          string        `json:"tool"`
	Status        string        `json:"status"`
	Project       string        `json:"project"`
	NodeID        string        `json:"nodeId"`
	Node          NodeSummary   `json:"node"`
	RawRef        *CorootRawRef `json:"rawRef,omitempty"`
}

type DashboardSummary struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	PanelCount  int      `json:"panelCount,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type DashboardsResult struct {
	SchemaVersion string             `json:"schemaVersion"`
	Tool          string             `json:"tool"`
	Status        string             `json:"status"`
	Project       string             `json:"project"`
	Dashboards    []DashboardSummary `json:"dashboards"`
	RawRef        *CorootRawRef      `json:"rawRef,omitempty"`
}

type DashboardResult struct {
	SchemaVersion string           `json:"schemaVersion"`
	Tool          string           `json:"tool"`
	Status        string           `json:"status"`
	Project       string           `json:"project"`
	DashboardID   string           `json:"dashboardId"`
	Dashboard     DashboardSummary `json:"dashboard"`
	RawRef        *CorootRawRef    `json:"rawRef,omitempty"`
}

type PanelDataResult struct {
	SchemaVersion string             `json:"schemaVersion"`
	Tool          string             `json:"tool"`
	Status        string             `json:"status"`
	Project       string             `json:"project"`
	DashboardID   string             `json:"dashboardId"`
	PanelID       string             `json:"panelId"`
	ChartSummary  CorootChartSummary `json:"chartSummary,omitempty"`
	RawRef        *CorootRawRef      `json:"rawRef,omitempty"`
}
