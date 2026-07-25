package edit

import (
	"reflect"
	"testing"
)

func TestDiffGraphsReturnsStableTouchedPaths(t *testing.T) {
	before := editTestGraph()
	after := cloneTestGraph(t, before)
	after.Nodes[1].Label = "Changed"
	after.Edges[1].Condition = "ok"
	diff := DiffGraphs(before, after)
	want := []string{"/edges/collect-end/condition", "/nodes/collect/label"}
	if !reflect.DeepEqual(diff.TouchedPaths, want) {
		t.Fatalf("touched paths = %#v, want %#v", diff.TouchedPaths, want)
	}
	if !reflect.DeepEqual(diff.Delta.UpdatedNodeIDs, []string{"collect"}) || !reflect.DeepEqual(diff.Delta.UpdatedEdgeIDs, []string{"collect-end"}) {
		t.Fatalf("delta = %#v", diff.Delta)
	}
}
