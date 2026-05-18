package visual

import (
	"testing"

	"runner/workflow"
)

func TestResolveVariablesExposesOnlyTopologicalUpstreamOutputs(t *testing.T) {
	graph := variableScopeGraph()

	preScopes := VariablesForNode(graph, "pre")
	if hasVariable(preScopes, "node_output", "restore_lsn") {
		t.Fatalf("pre node should not see downstream restore_lsn: %+v", preScopes)
	}
	restoreScopes := VariablesForNode(graph, "verify")
	if !hasVariable(restoreScopes, "node_output", "disk_free") {
		t.Fatalf("restore node should see upstream disk_free: %+v", restoreScopes)
	}
	if !hasVariable(restoreScopes, "workflow_input", "backup_id") {
		t.Fatalf("restore node should see workflow input backup_id: %+v", restoreScopes)
	}
}

func TestResolveVariablesAfterJoinRequiresExplicitJoinOutput(t *testing.T) {
	graph := variableScopeGraph()

	verifyScopes := VariablesForNode(graph, "verify")
	if hasVariable(verifyScopes, "node_output", "branch_a_result") {
		t.Fatalf("verify should not see branch A output after join without explicit join output: %+v", verifyScopes)
	}
	if hasVariable(verifyScopes, "node_output", "branch_b_result") {
		t.Fatalf("verify should not see branch B output after join without explicit join output: %+v", verifyScopes)
	}
	if !hasVariable(verifyScopes, "node_output", "joined_status") {
		t.Fatalf("verify should see explicit join output: %+v", verifyScopes)
	}
	if !hasVariable(verifyScopes, "node_output", "disk_free") {
		t.Fatalf("verify should still see common pre-branch output: %+v", verifyScopes)
	}
}

func TestResolveVariablesImplicitMergeIntersectsBranchOutputs(t *testing.T) {
	graph := implicitMergeVariableGraph()

	verifyScopes := VariablesForNode(graph, "verify")
	if hasVariable(verifyScopes, "node_output", "branch_a_result") {
		t.Fatalf("implicit merge should not see branch A output: %+v", verifyScopes)
	}
	if hasVariable(verifyScopes, "node_output", "branch_b_result") {
		t.Fatalf("implicit merge should not see branch B output: %+v", verifyScopes)
	}
	if !hasVariable(verifyScopes, "node_output", "disk_free") {
		t.Fatalf("implicit merge should still see common pre-branch output: %+v", verifyScopes)
	}
}

func TestResolveVariablesKeepsLoopInternalOutputsScoped(t *testing.T) {
	graph := variableScopeGraph()

	insideScopes := VariablesForNode(graph, "loop-inner")
	if !hasVariable(insideScopes, "workflow_input", "backup_id") {
		t.Fatalf("loop inner should see workflow input: %+v", insideScopes)
	}
	afterScopes := VariablesForNode(graph, "after-loop")
	if hasVariable(afterScopes, "node_output", "item_tmp") {
		t.Fatalf("after-loop should not see loop internal item_tmp: %+v", afterScopes)
	}
	if !hasVariable(afterScopes, "node_output", "loop_result") {
		t.Fatalf("after-loop should see explicit loop output: %+v", afterScopes)
	}
}

func TestValidateGraphRejectsDownstreamVariableReference(t *testing.T) {
	graph := downstreamVariableReferenceGraph()
	graph.Nodes[1].Inputs = []InputParamSpec{{
		Key:  "lsn",
		Type: "string",
		ValueSource: ValueSource{
			Type:     "variable",
			Variable: &VariableRef{Scope: "node_output", NodeID: "verify", Name: "restore_lsn"},
		},
	}}

	err := ValidateGraph(graph)
	if !hasValidationIssue(err, "input_variable_scope_invalid", "pre") {
		t.Fatalf("expected downstream variable scope issue, got %v", err)
	}
}

func TestValidateGraphRejectsBranchVariableAfterJoin(t *testing.T) {
	graph := joinVariableReferenceGraph()
	graph.Nodes[5].Inputs = []InputParamSpec{{
		Key:  "branch_result",
		Type: "string",
		ValueSource: ValueSource{
			Type:     "variable",
			Variable: &VariableRef{Scope: "node_output", NodeID: "branch-a", Name: "branch_a_result"},
		},
	}}

	err := ValidateGraph(graph)
	if !hasValidationIssue(err, "input_variable_scope_invalid", "verify") {
		t.Fatalf("expected branch variable scope issue after join, got %v", err)
	}
}

func variableScopeGraph() Graph {
	return Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "variable-scope-demo",
			Vars:    map[string]any{"backup_id": "b-001", "retry_limit": 2},
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"pg-primary": {Address: "127.0.0.1"}},
				Vars:  map[string]any{"region": "sh"},
			},
		},
		Nodes: []Node{
			{
				ID:   "start",
				Type: NodeTypeStart,
				Outputs: []OutputParamSpec{{
					Key:  "backup_id",
					Type: "string",
				}},
			},
			{
				ID:       "pre",
				Type:     NodeTypeAction,
				StepName: "pre",
				Step:     &workflow.Step{Name: "pre", Action: "script.shell", Args: map[string]any{"script": "echo pre"}},
				Outputs: []OutputParamSpec{{
					Key:  "disk_free",
					Type: "boolean",
				}},
			},
			{
				ID:       "branch-a",
				Type:     NodeTypeAction,
				StepName: "branch-a",
				Step:     &workflow.Step{Name: "branch-a", Action: "script.shell", Args: map[string]any{"script": "echo a"}},
				Outputs: []OutputParamSpec{{
					Key:  "branch_a_result",
					Type: "string",
				}},
			},
			{
				ID:       "branch-b",
				Type:     NodeTypeAction,
				StepName: "branch-b",
				Step:     &workflow.Step{Name: "branch-b", Action: "script.shell", Args: map[string]any{"script": "echo b"}},
				Outputs: []OutputParamSpec{{
					Key:  "branch_b_result",
					Type: "string",
				}},
			},
			{
				ID:   "join",
				Type: NodeTypeJoin,
				Join: &JoinSpec{Strategy: "all_success"},
				Outputs: []OutputParamSpec{{
					Key:  "joined_status",
					Type: "string",
				}},
			},
			{
				ID:       "verify",
				Type:     NodeTypeAction,
				StepName: "verify",
				Step:     &workflow.Step{Name: "verify", Action: "script.shell", Args: map[string]any{"script": "echo verify"}},
				Outputs: []OutputParamSpec{{
					Key:  "restore_lsn",
					Type: "string",
				}},
			},
			{
				ID:      "loop",
				Type:    NodeType("loop"),
				Outputs: []OutputParamSpec{{Key: "loop_result", Type: "array"}},
			},
			{
				ID:       "loop-inner",
				Type:     NodeTypeAction,
				ParentID: "loop",
				StepName: "loop-inner",
				Step:     &workflow.Step{Name: "loop-inner", Action: "script.shell", Args: map[string]any{"script": "echo item"}},
				Outputs: []OutputParamSpec{{
					Key:  "item_tmp",
					Type: "string",
				}},
			},
			{
				ID:       "after-loop",
				Type:     NodeTypeAction,
				StepName: "after-loop",
				Step:     &workflow.Step{Name: "after-loop", Action: "script.shell", Args: map[string]any{"script": "echo after"}},
			},
		},
		Edges: []Edge{
			{ID: "start-pre", Source: "start", Target: "pre"},
			{ID: "pre-a", Source: "pre", Target: "branch-a"},
			{ID: "pre-b", Source: "pre", Target: "branch-b"},
			{ID: "a-join", Source: "branch-a", Target: "join"},
			{ID: "b-join", Source: "branch-b", Target: "join"},
			{ID: "join-verify", Source: "join", Target: "verify"},
			{ID: "verify-loop", Source: "verify", Target: "loop"},
			{ID: "loop-inner", Source: "loop", Target: "loop-inner"},
			{ID: "inner-after", Source: "loop-inner", Target: "after-loop"},
			{ID: "loop-after", Source: "loop", Target: "after-loop"},
		},
	}
}

func downstreamVariableReferenceGraph() Graph {
	return Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "downstream-variable-demo",
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Outputs: []OutputParamSpec{{Key: "backup_id", Type: "string"}}},
			{
				ID:       "pre",
				Type:     NodeTypeAction,
				StepName: "pre",
				Step:     &workflow.Step{Name: "pre", Action: "script.shell", Args: map[string]any{"script": "echo pre"}},
				Outputs:  []OutputParamSpec{{Key: "disk_free", Type: "boolean"}},
			},
			{
				ID:       "verify",
				Type:     NodeTypeAction,
				StepName: "verify",
				Step:     &workflow.Step{Name: "verify", Action: "script.shell", Args: map[string]any{"script": "echo verify"}},
				Outputs:  []OutputParamSpec{{Key: "restore_lsn", Type: "string"}},
			},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "start-pre", Source: "start", Target: "pre"},
			{ID: "pre-verify", Source: "pre", Target: "verify"},
			{ID: "verify-end", Source: "verify", Target: "end"},
		},
	}
}

func joinVariableReferenceGraph() Graph {
	return Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "join-variable-demo",
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{
				ID:       "pre",
				Type:     NodeTypeAction,
				StepName: "pre",
				Step:     &workflow.Step{Name: "pre", Action: "script.shell", Args: map[string]any{"script": "echo pre"}},
				Outputs:  []OutputParamSpec{{Key: "disk_free", Type: "boolean"}},
			},
			{
				ID:       "branch-a",
				Type:     NodeTypeAction,
				StepName: "branch-a",
				Step:     &workflow.Step{Name: "branch-a", Action: "script.shell", Args: map[string]any{"script": "echo a"}},
				Outputs:  []OutputParamSpec{{Key: "branch_a_result", Type: "string"}},
			},
			{
				ID:       "branch-b",
				Type:     NodeTypeAction,
				StepName: "branch-b",
				Step:     &workflow.Step{Name: "branch-b", Action: "script.shell", Args: map[string]any{"script": "echo b"}},
				Outputs:  []OutputParamSpec{{Key: "branch_b_result", Type: "string"}},
			},
			{
				ID:      "join",
				Type:    NodeTypeJoin,
				Join:    &JoinSpec{Strategy: "all_success"},
				Outputs: []OutputParamSpec{{Key: "joined_status", Type: "string"}},
			},
			{
				ID:       "verify",
				Type:     NodeTypeAction,
				StepName: "verify",
				Step:     &workflow.Step{Name: "verify", Action: "script.shell", Args: map[string]any{"script": "echo verify"}},
			},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "start-pre", Source: "start", Target: "pre"},
			{ID: "pre-a", Source: "pre", Target: "branch-a"},
			{ID: "pre-b", Source: "pre", Target: "branch-b"},
			{ID: "a-join", Source: "branch-a", Target: "join"},
			{ID: "b-join", Source: "branch-b", Target: "join"},
			{ID: "join-verify", Source: "join", Target: "verify"},
			{ID: "verify-end", Source: "verify", Target: "end"},
		},
	}
}

func implicitMergeVariableGraph() Graph {
	return Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "implicit-merge-variable-demo",
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{
				ID:       "pre",
				Type:     NodeTypeAction,
				StepName: "pre",
				Step:     &workflow.Step{Name: "pre", Action: "script.shell", Args: map[string]any{"script": "echo pre"}},
				Outputs:  []OutputParamSpec{{Key: "disk_free", Type: "boolean"}},
			},
			{
				ID:       "branch-a",
				Type:     NodeTypeAction,
				StepName: "branch-a",
				Step:     &workflow.Step{Name: "branch-a", Action: "script.shell", Args: map[string]any{"script": "echo a"}},
				Outputs:  []OutputParamSpec{{Key: "branch_a_result", Type: "string"}},
			},
			{
				ID:       "branch-b",
				Type:     NodeTypeAction,
				StepName: "branch-b",
				Step:     &workflow.Step{Name: "branch-b", Action: "script.shell", Args: map[string]any{"script": "echo b"}},
				Outputs:  []OutputParamSpec{{Key: "branch_b_result", Type: "string"}},
			},
			{
				ID:       "verify",
				Type:     NodeTypeAction,
				StepName: "verify",
				Step:     &workflow.Step{Name: "verify", Action: "script.shell", Args: map[string]any{"script": "echo verify"}},
			},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "start-pre", Source: "start", Target: "pre"},
			{ID: "pre-a", Source: "pre", Target: "branch-a"},
			{ID: "pre-b", Source: "pre", Target: "branch-b"},
			{ID: "a-verify", Source: "branch-a", Target: "verify"},
			{ID: "b-verify", Source: "branch-b", Target: "verify"},
			{ID: "verify-end", Source: "verify", Target: "end"},
		},
	}
}

func hasVariable(scopes []VariableScope, scope, name string) bool {
	for _, item := range scopes {
		if item.Scope != scope {
			continue
		}
		for _, variable := range item.Variables {
			if variable.Name == name {
				return true
			}
		}
	}
	return false
}

func hasValidationIssue(err error, code, nodeID string) bool {
	validationErr, ok := err.(*ValidationError)
	if !ok {
		return false
	}
	for _, issue := range validationErr.Issues {
		if issue.Code == code && issue.NodeID == nodeID {
			return true
		}
	}
	return false
}
