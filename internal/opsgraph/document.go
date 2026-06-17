package opsgraph

import (
	"strings"
	"time"
)

const ManualGraphSchemaVersion = "aiops.opsgraph.manual/v1"

type NodeType string

const (
	NodeBusiness           NodeType = "business"
	NodeService            NodeType = "service"
	NodeEndpoint           NodeType = "endpoint"
	NodeMiddleware         NodeType = "middleware"
	NodeMiddlewareCluster  NodeType = "middleware_cluster"
	NodeMiddlewareInstance NodeType = "middleware_instance"
	NodeHost               NodeType = "host"
	NodeK8s                NodeType = "k8s"
	NodeCase               NodeType = "case"
	NodeWorkflow           NodeType = "workflow"
)

type Position struct {
	X float64 `json:"x" yaml:"x"`
	Y float64 `json:"y" yaml:"y"`
}

type Viewport struct {
	X    float64 `json:"x" yaml:"x"`
	Y    float64 `json:"y" yaml:"y"`
	Zoom float64 `json:"zoom" yaml:"zoom"`
}

type Node struct {
	ID          string            `json:"id" yaml:"id"`
	Type        NodeType          `json:"type" yaml:"type"`
	Name        string            `json:"name" yaml:"name"`
	ParentID    string            `json:"parentId,omitempty" yaml:"parentId,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Aliases     []string          `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Properties  map[string]string `json:"properties,omitempty" yaml:"properties,omitempty"`
	Position    *Position         `json:"position,omitempty" yaml:"position,omitempty"`
	Container   bool              `json:"container,omitempty" yaml:"container,omitempty"`
	Collapsed   bool              `json:"collapsed,omitempty" yaml:"collapsed,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt   time.Time         `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

type Edge struct {
	ID        string           `json:"id" yaml:"id"`
	From      string           `json:"from" yaml:"from"`
	Type      RelationshipType `json:"type" yaml:"type"`
	To        string           `json:"to" yaml:"to"`
	Note      string           `json:"note,omitempty" yaml:"note,omitempty"`
	Reason    string           `json:"reason,omitempty" yaml:"reason,omitempty"`
	CreatedAt time.Time        `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt time.Time        `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

type GraphRecord struct {
	ID          string    `json:"id" yaml:"id"`
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty"`
	Environment string    `json:"environment,omitempty" yaml:"environment,omitempty"`
	IsDefault   bool      `json:"isDefault,omitempty" yaml:"isDefault,omitempty"`
	Nodes       []Node    `json:"nodes" yaml:"nodes"`
	Edges       []Edge    `json:"edges" yaml:"edges"`
	Viewport    *Viewport `json:"viewport,omitempty" yaml:"viewport,omitempty"`
	CreatedAt   time.Time `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

type GraphDocument struct {
	SchemaVersion string        `json:"schemaVersion" yaml:"schemaVersion"`
	Graphs        []GraphRecord `json:"graphs" yaml:"graphs"`
}

type GraphSummary struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	Environment       string    `json:"environment,omitempty"`
	IsDefault         bool      `json:"isDefault"`
	NodeCount         int       `json:"nodeCount"`
	RelationshipCount int       `json:"relationshipCount"`
	IssueCount        int       `json:"issueCount"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
}

func (d GraphDocument) Normalized() GraphDocument {
	if strings.TrimSpace(d.SchemaVersion) == "" {
		d.SchemaVersion = ManualGraphSchemaVersion
	}
	for i := range d.Graphs {
		d.Graphs[i].ID = strings.TrimSpace(d.Graphs[i].ID)
		d.Graphs[i].Name = strings.TrimSpace(d.Graphs[i].Name)
	}
	return d
}

func (d GraphDocument) DefaultGraph() (GraphRecord, bool) {
	for _, graph := range d.Graphs {
		if graph.IsDefault {
			return graph, true
		}
	}
	if len(d.Graphs) > 0 {
		return d.Graphs[0], true
	}
	return GraphRecord{}, false
}

func (d GraphDocument) GraphByID(id string) (GraphRecord, bool) {
	id = strings.TrimSpace(id)
	for _, graph := range d.Graphs {
		if graph.ID == id {
			return graph, true
		}
	}
	return GraphRecord{}, false
}

func (d GraphDocument) Summaries() []GraphSummary {
	out := make([]GraphSummary, 0, len(d.Graphs))
	for _, graph := range d.Graphs {
		out = append(out, GraphSummary{
			ID:                graph.ID,
			Name:              graph.Name,
			Description:       graph.Description,
			Environment:       graph.Environment,
			IsDefault:         graph.IsDefault,
			NodeCount:         len(graph.Nodes),
			RelationshipCount: len(graph.Edges),
			IssueCount:        len(ValidateGraph(graph)),
			UpdatedAt:         graph.UpdatedAt,
		})
	}
	return out
}

func CompileGraphStore(graph GraphRecord) *Store {
	entities := make([]Entity, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		entities = append(entities, entityFromNode(node))
	}
	relationships := make([]Relationship, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		relationships = append(relationships, Relationship{
			From:   strings.TrimSpace(edge.From),
			Type:   edge.Type,
			To:     strings.TrimSpace(edge.To),
			Reason: firstNonEmpty(edge.Reason, edge.Note),
		})
	}
	return NewStore(entities, relationships)
}

func entityFromNode(node Node) Entity {
	attributes := map[string]string{}
	for key, value := range node.Labels {
		attributes[key] = value
	}
	for key, value := range node.Properties {
		attributes[key] = value
	}
	if node.ParentID != "" {
		attributes["parentId"] = node.ParentID
	}
	if node.Container {
		attributes["container"] = "true"
	}
	if node.Collapsed {
		attributes["collapsed"] = "true"
	}
	return Entity{
		ID:          strings.TrimSpace(node.ID),
		Type:        entityTypeFromNodeType(node.Type),
		Name:        strings.TrimSpace(node.Name),
		Description: strings.TrimSpace(node.Description),
		Aliases:     node.Aliases,
		Tags:        node.Tags,
		Attributes:  attributes,
	}
}

func entityTypeFromNodeType(typ NodeType) EntityType {
	switch typ {
	case NodeBusiness:
		return EntityBusinessCapability
	case NodeMiddleware:
		return EntityMiddleware
	case NodeMiddlewareCluster:
		return EntityMiddlewareCluster
	case NodeMiddlewareInstance:
		return EntityMiddlewareInstance
	case NodeK8s:
		return EntityK8s
	case NodeWorkflow:
		return EntityRunbook
	case NodeCase:
		return EntityCase
	default:
		return EntityType(typ)
	}
}
