package workfloweditor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"runner/workflow/visual"
)

var allowedPatchOps = map[PatchOperationType]bool{
	PatchAddNode:    true,
	PatchUpdateNode: true,
	PatchDeleteNode: true,
	PatchAddEdge:    true,
	PatchDeleteEdge: true,
}

func RevisionDigest(graph visual.Graph) string {
	normalized, _ := json.Marshal(graph)
	sum := sha256.Sum256(normalized)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func CloneGraph(graph visual.Graph) visual.Graph {
	raw, _ := json.Marshal(graph)
	var out visual.Graph
	_ = json.Unmarshal(raw, &out)
	return out
}

func ValidateWorkflowPatch(req ValidatePatchRequest) WorkflowPatchValidation {
	var errors []string
	patch := req.Patch
	if strings.TrimSpace(patch.ID) == "" {
		errors = append(errors, "patch_id is required")
	}
	if strings.TrimSpace(firstNonEmpty(patch.BaseRevision, req.BaseRevision)) == "" {
		errors = append(errors, "base_revision is required")
	}
	if len(patch.Operations) == 0 {
		errors = append(errors, "at least one operation is required")
	}
	for i, op := range patch.Operations {
		if op.Op == PatchReplaceFullGraph {
			errors = append(errors, fmt.Sprintf("operation %d replace_full_graph is retired; use canonical typed operations", i))
			continue
		}
		if !allowedPatchOps[op.Op] {
			errors = append(errors, fmt.Sprintf("operation %d has unsupported op %q", i, op.Op))
		}
		switch op.Op {
		case PatchAddNode:
			if op.Node == nil || strings.TrimSpace(op.Node.ID) == "" {
				errors = append(errors, fmt.Sprintf("operation %d add_node requires node.id", i))
			}
		case PatchUpdateNode, PatchDeleteNode:
			if strings.TrimSpace(op.NodeID) == "" {
				errors = append(errors, fmt.Sprintf("operation %d %s requires node_id", i, op.Op))
			}
			if op.Op == PatchUpdateNode {
				for field := range op.Fields {
					if field != "label" && field != "type" {
						errors = append(errors, fmt.Sprintf("operation %d update_node field %q is not part of the compatibility contract", i, field))
					}
				}
			}
		case PatchAddEdge:
			if op.Edge == nil || strings.TrimSpace(op.Edge.ID) == "" || strings.TrimSpace(op.Edge.Source) == "" || strings.TrimSpace(op.Edge.Target) == "" {
				errors = append(errors, fmt.Sprintf("operation %d add_edge requires edge id/source/target", i))
			}
		case PatchDeleteEdge:
			if strings.TrimSpace(op.EdgeID) == "" {
				errors = append(errors, fmt.Sprintf("operation %d delete_edge requires edge_id", i))
			}
		}
	}
	return WorkflowPatchValidation{Valid: len(errors) == 0, Errors: errors}
}

func DetectWorkflowPatchEffect(graph visual.Graph, patch WorkflowPatch, appliedPatchIDs map[string]bool) PatchEffectResult {
	if appliedPatchIDs[strings.TrimSpace(patch.ID)] {
		return PatchEffectResult{Status: EffectDuplicate, Summary: "patch was already applied"}
	}
	next, effect, err := ApplyPatchToGraph(graph, patch)
	if err != nil {
		return PatchEffectResult{Status: EffectNoEffect, Summary: err.Error()}
	}
	if effect.Status == EffectMetadataOnly {
		return effect
	}
	if reflect.DeepEqual(graph, next) {
		effect.Status = EffectNoEffect
		effect.Summary = firstNonEmpty(effect.Summary, "patch does not change workflow graph")
		return effect
	}
	if effect.Status == "" {
		effect.Status = EffectChanged
	}
	effect.Summary = firstNonEmpty(effect.Summary, "patch changes workflow graph")
	return effect
}

func ApplyPatchToGraph(graph visual.Graph, patch WorkflowPatch) (visual.Graph, PatchEffectResult, error) {
	next := CloneGraph(graph)
	effect := PatchEffectResult{Status: EffectChanged}
	changed := false
	metadataOnly := len(patch.Operations) > 0
	for _, op := range patch.Operations {
		switch op.Op {
		case PatchAddNode:
			metadataOnly = false
			if op.Node == nil {
				return graph, effect, fmt.Errorf("add_node requires node")
			}
			if nodeIndex(next.Nodes, op.Node.ID) >= 0 {
				return graph, effect, fmt.Errorf("node %q already exists", op.Node.ID)
			}
			next.Nodes = append(next.Nodes, *op.Node)
			effect.AffectedNodes = appendUnique(effect.AffectedNodes, op.Node.ID)
			changed = true
		case PatchUpdateNode:
			metadataOnly = false
			idx := nodeIndex(next.Nodes, op.NodeID)
			if idx < 0 {
				return graph, effect, fmt.Errorf("node %q not found", op.NodeID)
			}
			updated, updateErr := applyNodeFields(&next.Nodes[idx], op.Fields)
			if updateErr != nil {
				return graph, effect, updateErr
			}
			if updated {
				changed = true
			}
			effect.AffectedNodes = appendUnique(effect.AffectedNodes, op.NodeID)
		case PatchDeleteNode:
			metadataOnly = false
			idx := nodeIndex(next.Nodes, op.NodeID)
			if idx < 0 {
				return graph, effect, fmt.Errorf("node %q not found", op.NodeID)
			}
			effect.AffectedNodes = appendUnique(effect.AffectedNodes, op.NodeID)
			effect.AffectedVariables = appendUnique(effect.AffectedVariables, nodeOutputKeys(next.Nodes[idx])...)
			next.Nodes = append(next.Nodes[:idx], next.Nodes[idx+1:]...)
			var edges []visual.Edge
			for _, edge := range next.Edges {
				if edge.Source == op.NodeID || edge.Target == op.NodeID {
					effect.AffectedEdges = appendUnique(effect.AffectedEdges, edge.ID)
					changed = true
					continue
				}
				edges = append(edges, edge)
			}
			next.Edges = edges
			changed = true
		case PatchAddEdge:
			metadataOnly = false
			if op.Edge == nil {
				return graph, effect, fmt.Errorf("add_edge requires edge")
			}
			if edgeIndex(next.Edges, op.Edge.ID) >= 0 {
				return graph, effect, fmt.Errorf("edge %q already exists", op.Edge.ID)
			}
			next.Edges = append(next.Edges, *op.Edge)
			effect.AffectedEdges = appendUnique(effect.AffectedEdges, op.Edge.ID)
			changed = true
		case PatchDeleteEdge:
			metadataOnly = false
			idx := edgeIndex(next.Edges, op.EdgeID)
			if idx < 0 {
				return graph, effect, fmt.Errorf("edge %q not found", op.EdgeID)
			}
			next.Edges = append(next.Edges[:idx], next.Edges[idx+1:]...)
			effect.AffectedEdges = appendUnique(effect.AffectedEdges, op.EdgeID)
			changed = true
		default:
			return graph, effect, fmt.Errorf("unsupported operation %q", op.Op)
		}
	}
	if !changed {
		effect.Status = EffectNoEffect
		effect.Summary = "patch does not change workflow graph"
		return next, effect, nil
	}
	if metadataOnly {
		effect.Status = EffectMetadataOnly
		effect.Summary = "patch only changes workflow metadata"
	} else {
		effect.Status = EffectChanged
		effect.Summary = "patch changes workflow graph"
	}
	effect.AffectedNodes = sortedUnique(effect.AffectedNodes)
	effect.AffectedEdges = sortedUnique(effect.AffectedEdges)
	effect.AffectedVariables = sortedUnique(effect.AffectedVariables)
	return next, effect, nil
}

func applyNodeFields(node *visual.Node, fields map[string]any) (bool, error) {
	changed := false
	for key, value := range fields {
		switch key {
		case "label":
			label, ok := value.(string)
			if !ok {
				return false, fmt.Errorf("update_node label must be a string")
			}
			if node.Label != label {
				node.Label = label
				changed = true
			}
		case "type":
			raw, ok := value.(string)
			if !ok {
				return false, fmt.Errorf("update_node type must be a string")
			}
			if string(node.Type) != raw {
				node.Type = visual.NodeType(raw)
				changed = true
			}
		default:
			return false, fmt.Errorf("update_node field %q is not supported", key)
		}
	}
	return changed, nil
}

func nodeIndex(nodes []visual.Node, id string) int {
	for i, node := range nodes {
		if node.ID == id {
			return i
		}
	}
	return -1
}

func edgeIndex(edges []visual.Edge, id string) int {
	for i, edge := range edges {
		if edge.ID == id {
			return i
		}
	}
	return -1
}

func nodeOutputKeys(node visual.Node) []string {
	keys := make([]string, 0, len(node.Outputs))
	for _, output := range node.Outputs {
		if strings.TrimSpace(output.Key) != "" {
			keys = append(keys, output.Key)
		}
	}
	return keys
}

func appendUnique(values []string, extras ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, extra := range extras {
		extra = strings.TrimSpace(extra)
		if extra == "" || seen[extra] {
			continue
		}
		values = append(values, extra)
		seen[extra] = true
	}
	return values
}

func sortedUnique(values []string) []string {
	values = appendUnique(nil, values...)
	sort.Strings(values)
	return values
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
