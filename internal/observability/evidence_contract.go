package observability

type EvidencePack struct {
	Provider        string               `json:"provider,omitempty"`
	Project         string               `json:"project,omitempty"`
	Target          EntityRef            `json:"target,omitempty"`
	TargetStatus    []StatusEvidence     `json:"target_status,omitempty"`
	DependencyEdges []DependencyEdge     `json:"dependency_edges,omitempty"`
	Incidents       []IncidentEvidence   `json:"incidents,omitempty"`
	Metrics         []MetricEvidence     `json:"metrics,omitempty"`
	Logs            []LogEvidence        `json:"logs,omitempty"`
	Traces          []TraceEvidence      `json:"traces,omitempty"`
	Deployments     []DeploymentEvidence `json:"deployments,omitempty"`
	Hypotheses      []Hypothesis         `json:"hypotheses,omitempty"`
	MissingEvidence []string             `json:"missing_evidence,omitempty"`
}

type EntityRef struct {
	Kind    string `json:"kind,omitempty"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Cluster string `json:"cluster,omitempty"`
	RawRef  string `json:"raw_ref,omitempty"`
}

type StatusEvidence struct {
	Entity     string `json:"entity,omitempty"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type DependencyEdge struct {
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Direction  string `json:"direction,omitempty"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type IncidentEvidence struct {
	ID         string `json:"id,omitempty"`
	Entity     string `json:"entity,omitempty"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type MetricEvidence struct {
	Entity     string `json:"entity,omitempty"`
	Name       string `json:"name,omitempty"`
	Status     string `json:"status,omitempty"`
	Value      string `json:"value,omitempty"`
	Unit       string `json:"unit,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type LogEvidence struct {
	Entity     string `json:"entity,omitempty"`
	Name       string `json:"name,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Count      int    `json:"count,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type TraceEvidence struct {
	Entity     string `json:"entity,omitempty"`
	Name       string `json:"name,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type DeploymentEvidence struct {
	Entity     string `json:"entity,omitempty"`
	Name       string `json:"name,omitempty"`
	Version    string `json:"version,omitempty"`
	Status     string `json:"status,omitempty"`
	Summary    string `json:"summary,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Confidence string `json:"confidence,omitempty"`
	RawRef     string `json:"raw_ref,omitempty"`
}

type Hypothesis struct {
	Entity     string   `json:"entity,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Severity   string   `json:"severity,omitempty"`
	Confidence string   `json:"confidence,omitempty"`
	Evidence   []string `json:"evidence,omitempty"`
	RawRef     string   `json:"raw_ref,omitempty"`
}
