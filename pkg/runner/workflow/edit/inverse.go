package edit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"runner/workflow/visual"
)

type PathSnapshot struct {
	Path   string          `json:"path"`
	Exists bool            `json:"exists"`
	Value  json.RawMessage `json:"value,omitempty"`
}
type InversePlan struct {
	Operations []WorkflowOperation `json:"operations"`
	Expected   []PathSnapshot      `json:"expected"`
}

func buildInversePlan(operations []WorkflowOperation, after visual.Graph, paths []string) (InversePlan, error) {
	plan := InversePlan{Operations: operations, Expected: make([]PathSnapshot, 0, len(paths))}
	for _, path := range paths {
		snapshot, err := snapshotPath(after, path)
		if err != nil {
			return InversePlan{}, err
		}
		plan.Expected = append(plan.Expected, snapshot)
	}
	return plan, nil
}
func ApplyInverse(current visual.Graph, plan InversePlan) (ApplyResult, error) {
	var conflicts []string
	for _, expected := range plan.Expected {
		actual, err := snapshotPath(current, expected.Path)
		if err != nil || actual.Exists != expected.Exists || !bytes.Equal(actual.Value, expected.Value) {
			conflicts = append(conflicts, expected.Path)
		}
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		return ApplyResult{}, &DomainError{Kind: "error", Code: ErrorStaleRevision, Message: "workflow changed after the transaction", ConflictingPaths: conflicts, RetryHint: "review current values before undoing"}
	}
	return ApplyOperations(current, plan.Operations)
}
func snapshotPath(graph visual.Graph, path string) (PathSnapshot, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || (parts[0] != "nodes" && parts[0] != "edges") {
		return PathSnapshot{}, fmt.Errorf("unsupported touched path %q", path)
	}
	id := unescapePointer(parts[1])
	var value any
	var found bool
	if parts[0] == "nodes" {
		value, found = findNodeValue(graph, id)
	} else {
		value, found = findEdgeValue(graph, id)
	}
	if !found {
		return PathSnapshot{Path: path}, nil
	}
	if len(parts) == 3 {
		field := unescapePointer(parts[2])
		value, found = jsonObject(value)[field]
		if !found {
			return PathSnapshot{Path: path}, nil
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return PathSnapshot{}, err
	}
	return PathSnapshot{Path: path, Exists: true, Value: raw}, nil
}
func findNodeValue(graph visual.Graph, id string) (any, bool) {
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return nil, false
}
func findEdgeValue(graph visual.Graph, id string) (any, bool) {
	for _, edge := range graph.Edges {
		if edge.ID == id {
			return edge, true
		}
	}
	return nil, false
}
func inverseNodePatch(before visual.Node, mask NodePatch) (NodePatch, bool) {
	result := NodePatch{}
	return result, copyPointerFields(&result, before, mask, nodePatchFieldMap)
}
func inverseEdgePatch(before visual.Edge, mask EdgePatch) (EdgePatch, bool) {
	result := EdgePatch{}
	return result, copyPointerFields(&result, before, mask, edgePatchFieldMap)
}
func copyPointerFields(target, source, mask any, fieldMap map[string]string) bool {
	targetValue, sourceValue, maskValue := reflect.ValueOf(target).Elem(), reflect.ValueOf(source), reflect.ValueOf(mask)
	for patchField, sourceField := range fieldMap {
		maskItem := maskValue.FieldByName(patchField)
		if maskItem.IsNil() {
			continue
		}
		targetItem, sourceItem := targetValue.FieldByName(patchField), sourceValue.FieldByName(sourceField)
		if sourceItem.Kind() == reflect.Pointer && sourceItem.IsNil() {
			return false
		}
		if sourceItem.Type().AssignableTo(targetItem.Type()) {
			targetItem.Set(sourceItem)
			continue
		}
		copyValue := reflect.New(targetItem.Type().Elem())
		copyValue.Elem().Set(sourceItem)
		targetItem.Set(copyValue)
	}
	return true
}
