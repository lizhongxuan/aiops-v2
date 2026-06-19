package opsmanual

const (
	ResourceRoleDataNode = "data_node"
	ResourceRoleMonitor  = "monitor"
	ResourceRoleProxy    = "proxy"
	ResourceRoleObserver = "observer"
	ResourceRoleExecutor = "executor"

	RelationshipMonitors     = "monitors"
	RelationshipReplicatesTo = "replicates_to"

	ExecutionSurfaceHostAgent = "host_agent"
	ExecutionSurfaceRunner    = "runner"
	ExecutionSurfaceMCP       = "mcp"
	ExecutionSurfaceAPI       = "api"
	ExecutionSurfaceSQL       = "sql"
	ExecutionSurfaceUnknown   = "unknown"

	ObservationPointMonitorComponent = "monitor_component"
	ObservationAccessHostAgent       = "host_agent"
	ObservationAccessHTTP            = "http"
	ObservationAccessUnknown         = "unknown"
)

type OperationResourceRole struct {
	ID           string `json:"id,omitempty"`
	Kind         string `json:"kind,omitempty"`
	ResourceRef  string `json:"resource_ref,omitempty"`
	UserLabel    string `json:"user_label,omitempty"`
	RuntimeName  string `json:"runtime_name,omitempty"`
	InferredFrom string `json:"inferred_from,omitempty"`
}

type OperationResourceRelationship struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	Type string `json:"type,omitempty"`
}

type OperationExecutionSurface struct {
	Kind      string   `json:"kind,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

type OperationObservationPoint struct {
	Kind        string `json:"kind,omitempty"`
	ResourceRef string `json:"resource_ref,omitempty"`
	Role        string `json:"role,omitempty"`
	Access      string `json:"access,omitempty"`
}

type OperationRiskPreference struct {
	DataLossAcceptable    bool `json:"data_loss_acceptable,omitempty"`
	StillRequiresApproval bool `json:"still_requires_approval,omitempty"`
}
