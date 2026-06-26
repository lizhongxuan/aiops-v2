package envcontext

import "time"

type FactKind string

const (
	FactKindHostIdentity       FactKind = "host_identity"
	FactKindServiceIdentity    FactKind = "service_identity"
	FactKindMiddlewareIdentity FactKind = "middleware_identity"
	FactKindVersion            FactKind = "version"
	FactKindPort               FactKind = "port"
	FactKindRole               FactKind = "role"
	FactKindTopology           FactKind = "topology"
	FactKindObservedSymptom    FactKind = "observed_symptom"
	FactKindExternalKnowledge  FactKind = "external_knowledge"
)

type FactSource string

const (
	FactSourceUser         FactSource = "user_explicit"
	FactSourceHostObserved FactSource = "host_observed"
	FactSourceCoroot       FactSource = "coroot"
	FactSourceInventory    FactSource = "inventory"
	FactSourceOpsGraph     FactSource = "opsgraph"
	FactSourceToolOutput   FactSource = "tool_output"
	FactSourceWebLearn     FactSource = "weblearn"
)

type FactConfidence string

const (
	FactConfidenceConfirmed FactConfidence = "confirmed"
	FactConfidenceObserved  FactConfidence = "observed"
	FactConfidenceInferred  FactConfidence = "inferred"
	FactConfidenceMissing   FactConfidence = "missing"
)

type TargetKind string

const (
	TargetKindHost       TargetKind = "host"
	TargetKindService    TargetKind = "service"
	TargetKindMiddleware TargetKind = "middleware"
)

type EnvironmentFact struct {
	ID          string         `json:"id,omitempty"`
	Kind        FactKind       `json:"kind,omitempty"`
	Subject     string         `json:"subject,omitempty"`
	Value       string         `json:"value,omitempty"`
	Source      FactSource     `json:"source,omitempty"`
	SourceRef   string         `json:"sourceRef,omitempty"`
	Confidence  FactConfidence `json:"confidence,omitempty"`
	CollectedAt time.Time      `json:"collectedAt,omitempty"`
}

type EnvironmentFactConflict struct {
	Subject string            `json:"subject,omitempty"`
	Kind    FactKind          `json:"kind,omitempty"`
	Facts   []EnvironmentFact `json:"facts,omitempty"`
	Reason  string            `json:"reason,omitempty"`
}

type TargetRef struct {
	ID          string         `json:"id,omitempty"`
	Kind        TargetKind     `json:"kind,omitempty"`
	DisplayName string         `json:"displayName,omitempty"`
	Address     string         `json:"address,omitempty"`
	Source      FactSource     `json:"source,omitempty"`
	Confidence  FactConfidence `json:"confidence,omitempty"`
}
