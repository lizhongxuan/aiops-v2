package appui

import "context"

type CapabilityListRequest struct {
	Query    string `json:"query,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Category string `json:"category,omitempty"`
}

type CapabilityListResponse struct {
	Items []CapabilityRecord `json:"items"`
}

type CapabilityRecord struct {
	ID             string           `json:"id"`
	Kind           string           `json:"kind"`
	Category       string           `json:"category"`
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Source         string           `json:"source,omitempty"`
	SourceScope    string           `json:"sourceScope,omitempty"`
	Enabled        bool             `json:"enabled,omitempty"`
	DefaultEnabled bool             `json:"defaultEnabled,omitempty"`
	Status         string           `json:"status,omitempty"`
	Risk           string           `json:"risk,omitempty"`
	Tags           []string         `json:"tags,omitempty"`
	Facets         CapabilityFacets `json:"facets,omitempty"`
}

type CapabilityFacets struct {
	Skill      *CapabilitySkillFacet      `json:"skill,omitempty"`
	Connection *CapabilityConnectionFacet `json:"connection,omitempty"`
	Plugin     *CapabilityPluginFacet     `json:"plugin,omitempty"`
}

type CapabilitySkillFacet struct {
	ActivationMode string   `json:"activationMode,omitempty"`
	InvocationMode string   `json:"invocationMode,omitempty"`
	AllowedTools   []string `json:"allowedTools,omitempty"`
	DeniedTools    []string `json:"deniedTools,omitempty"`
	UserInvocable  bool     `json:"userInvocable,omitempty"`
	ModelInvocable bool     `json:"modelInvocable,omitempty"`
}

type CapabilityConnectionFacet struct {
	Type                         string `json:"type,omitempty"`
	Permission                   string `json:"permission,omitempty"`
	ApprovalStatus               string `json:"approvalStatus,omitempty"`
	RuntimeStatus                string `json:"runtimeStatus,omitempty"`
	RequiresExplicitUserApproval bool   `json:"requiresExplicitUserApproval,omitempty"`
}

type CapabilityPluginFacet struct {
	Name           string   `json:"name,omitempty"`
	SkillCount     int      `json:"skillCount,omitempty"`
	MCPServerCount int      `json:"mcpServerCount,omitempty"`
	CommandCount   int      `json:"commandCount,omitempty"`
	AgentCount     int      `json:"agentCount,omitempty"`
	ManifestPath   string   `json:"manifestPath,omitempty"`
	Root           string   `json:"root,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}

type CapabilityService interface {
	ListRecords(ctx context.Context, req CapabilityListRequest) (CapabilityListResponse, error)
	Search(ctx context.Context, req CapabilityListRequest) (CapabilityListResponse, error)
	Resolve(ctx context.Context, id string) (CapabilityRecord, error)
	Pin(ctx context.Context, id string) (CapabilityRecord, error)
	Unpin(ctx context.Context, id string) (CapabilityRecord, error)
}
