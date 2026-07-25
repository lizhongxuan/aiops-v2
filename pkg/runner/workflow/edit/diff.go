package edit

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"runner/workflow/visual"
)

type GraphDiff struct {
	Delta        WorkflowGraphDelta `json:"delta"`
	TouchedPaths []string           `json:"touchedPaths"`
}

func DiffGraphs(before, after visual.Graph) GraphDiff {
	beforeNodes, afterNodes := entityMap(before.Nodes, func(node visual.Node) string { return node.ID }), entityMap(after.Nodes, func(node visual.Node) string { return node.ID })
	beforeEdges, afterEdges := entityMap(before.Edges, func(edge visual.Edge) string { return edge.ID }), entityMap(after.Edges, func(edge visual.Edge) string { return edge.ID })
	var result GraphDiff
	diffEntities("nodes", beforeNodes, afterNodes, &result.Delta.AddedNodeIDs, &result.Delta.UpdatedNodeIDs, &result.Delta.DeletedNodeIDs, &result.TouchedPaths)
	diffEntities("edges", beforeEdges, afterEdges, &result.Delta.AddedEdgeIDs, &result.Delta.UpdatedEdgeIDs, &result.Delta.DeletedEdgeIDs, &result.TouchedPaths)
	sort.Strings(result.Delta.AddedNodeIDs)
	sort.Strings(result.Delta.UpdatedNodeIDs)
	sort.Strings(result.Delta.DeletedNodeIDs)
	sort.Strings(result.Delta.AddedEdgeIDs)
	sort.Strings(result.Delta.UpdatedEdgeIDs)
	sort.Strings(result.Delta.DeletedEdgeIDs)
	sort.Strings(result.TouchedPaths)
	return result
}
func diffEntities[T any](kind string, before, after map[string]T, added, updated, deleted *[]string, paths *[]string) {
	ids := map[string]struct{}{}
	for id := range before {
		ids[id] = struct{}{}
	}
	for id := range after {
		ids[id] = struct{}{}
	}
	for id := range ids {
		oldValue, hadOld := before[id]
		newValue, hasNew := after[id]
		basePath := "/" + kind + "/" + escapePointer(id)
		switch {
		case !hadOld && hasNew:
			*added = append(*added, id)
			*paths = append(*paths, basePath)
		case hadOld && !hasNew:
			*deleted = append(*deleted, id)
			*paths = append(*paths, basePath)
		case !reflect.DeepEqual(oldValue, newValue):
			*updated = append(*updated, id)
			*paths = append(*paths, changedFieldPaths(basePath, oldValue, newValue)...)
		}
	}
}
func changedFieldPaths(base string, before, after any) []string {
	oldFields, newFields := jsonObject(before), jsonObject(after)
	keys := map[string]struct{}{}
	for key := range oldFields {
		keys[key] = struct{}{}
	}
	for key := range newFields {
		keys[key] = struct{}{}
	}
	var paths []string
	for key := range keys {
		if key == "state" {
			continue
		}
		if !reflect.DeepEqual(oldFields[key], newFields[key]) {
			paths = append(paths, base+"/"+escapePointer(key))
		}
	}
	return paths
}
func jsonObject(value any) map[string]any {
	raw, _ := json.Marshal(value)
	object := map[string]any{}
	_ = json.Unmarshal(raw, &object)
	return object
}
func entityMap[T any](values []T, id func(T) string) map[string]T {
	result := make(map[string]T, len(values))
	for _, value := range values {
		result[id(value)] = value
	}
	return result
}
func escapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func unescapePointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~")
}
