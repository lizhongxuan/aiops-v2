package coroot

const corootSchemaVersion = "aiops.coroot/v1"

type CorootRawRef struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
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
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
	Value  string `json:"value,omitempty"`
}

type ServiceMetricsResult struct {
	SchemaVersion string          `json:"schemaVersion"`
	Tool          string          `json:"tool"`
	Status        string          `json:"status"`
	Project       string          `json:"project"`
	Service       string          `json:"service"`
	Metrics       []MetricSummary `json:"metrics"`
	RawRef        *CorootRawRef   `json:"rawRef,omitempty"`
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
