package opsgraph

type EntityType string

const (
	EntityERPModule          EntityType = "erp_module"
	EntityBusinessCapability EntityType = "business_capability"
	EntityService            EntityType = "service"
	EntityDB                 EntityType = "db"
	EntityMQ                 EntityType = "mq"
	EntityRedis              EntityType = "redis"
	EntityHost               EntityType = "host"
	EntityPod                EntityType = "pod"
	EntityTenant             EntityType = "tenant"
	EntityRunbook            EntityType = "runbook"
)

type RelationshipType string

const (
	RelOwns      RelationshipType = "owns"
	RelDependsOn RelationshipType = "depends_on"
	RelRunsOn    RelationshipType = "runs_on"
	RelAffects   RelationshipType = "affects"
	RelHandledBy RelationshipType = "handled_by"
)

type Entity struct {
	ID          string            `json:"id" yaml:"id"`
	Type        EntityType        `json:"type" yaml:"type"`
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Aliases     []string          `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty" yaml:"attributes,omitempty"`
}

type Relationship struct {
	From   string           `json:"from" yaml:"from"`
	Type   RelationshipType `json:"type" yaml:"type"`
	To     string           `json:"to" yaml:"to"`
	Reason string           `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type LookupRequest struct {
	Query string       `json:"query"`
	Types []EntityType `json:"types,omitempty"`
	Limit int          `json:"limit,omitempty"`
}

type Neighborhood struct {
	Root          Entity         `json:"root"`
	Depth         int            `json:"depth"`
	Entities      []Entity       `json:"entities"`
	Relationships []Relationship `json:"relationships"`
}

type BusinessImpact struct {
	Entity       Entity   `json:"entity"`
	Modules      []Entity `json:"modules"`
	Capabilities []Entity `json:"capabilities"`
	Tenants      []Entity `json:"tenants"`
	Services     []Entity `json:"services"`
	Summary      string   `json:"summary"`
}

type RunbookMatch struct {
	Runbook Entity `json:"runbook"`
	Reason  string `json:"reason"`
}
