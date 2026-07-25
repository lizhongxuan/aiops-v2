package edit

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"runner/workflow/visual"
)

func cloneTestGraph(t *testing.T, graph visual.Graph) visual.Graph {
	t.Helper()
	raw, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	var cloned visual.Graph
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func graphsEqual(t *testing.T, left, right visual.Graph) bool {
	t.Helper()
	sort.Slice(left.Nodes, func(i, j int) bool { return left.Nodes[i].ID < left.Nodes[j].ID })
	sort.Slice(right.Nodes, func(i, j int) bool { return right.Nodes[i].ID < right.Nodes[j].ID })
	sort.Slice(left.Edges, func(i, j int) bool { return left.Edges[i].ID < left.Edges[j].ID })
	sort.Slice(right.Edges, func(i, j int) bool { return right.Edges[i].ID < right.Edges[j].ID })
	var leftJSON, rightJSON any
	leftRaw, _ := json.Marshal(left)
	rightRaw, _ := json.Marshal(right)
	if err := json.Unmarshal(leftRaw, &leftJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rightRaw, &rightJSON); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(leftJSON, rightJSON)
}
