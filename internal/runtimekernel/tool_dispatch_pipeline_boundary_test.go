package runtimekernel

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

var _ func(
	*ToolDispatcher,
	context.Context,
	string,
	string,
	ToolCall,
	SessionType,
	Mode,
	string,
	bool,
	*VerifiedActionToken,
) DispatchResult = (*ToolDispatcher).dispatch

func TestToolDispatchPipelinePackageBoundary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob runtimekernel production files: %v", err)
	}

	fset := token.NewFileSet()
	methods := map[string][]string{}
	var pipeline *ast.File
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		if filepath.Base(path) == "tool_dispatch_pipeline.go" {
			pipeline = parsed
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || receiverTypeName(fn.Recv.List[0].Type) != "ToolDispatcher" {
				continue
			}
			methods[fn.Name.Name] = append(methods[fn.Name.Name], filepath.Base(path))
		}
	}

	assertMethodOwner(t, methods, "dispatch", "tool_dispatch_pipeline.go")
	assertMethodOwner(t, methods, "Dispatch", "dispatch.go")
	assertMethodOwner(t, methods, "DispatchApproved", "dispatch.go")
	assertMethodOwner(t, methods, "DispatchWithParentSpan", "dispatch.go")
	if pipeline == nil {
		return
	}

	dispatch := methodDecl(pipeline, "ToolDispatcher", "dispatch")
	if dispatch == nil || dispatch.Body == nil {
		t.Fatal("tool_dispatch_pipeline.go does not contain (*ToolDispatcher).dispatch")
	}

	forbidden := map[string]struct{}{
		"BuildStepToolRouter":                 {},
		"StepToolRouterInput":                 {},
		"hydrateStepToolRouterForDispatch":    {},
		"RuntimeToolRouterSnapshotFromPolicy": {},
		"modelVisibleToolsForStep":            {},
		"toolNames":                           {},
		"assembledToolLookup":                 {},
		"NewToolDispatcher":                   {},
		"WithStepToolRouter":                  {},
		"promptcompiler":                      {},
	}
	ast.Inspect(pipeline, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok {
			if _, blocked := forbidden[ident.Name]; blocked {
				t.Errorf("tool_dispatch_pipeline.go must consume the frozen StepToolRouter; forbidden rebuild identifier %q", ident.Name)
			}
		}
		return true
	})

	ast.Inspect(dispatch.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			selector, ok := lhs.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "stepToolRouter" {
				continue
			}
			if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "d" {
				t.Error("dispatch pipeline must not assign d.stepToolRouter")
			}
		}
		return true
	})

	wantOrder := []string{
		"rejectModelToolOutsideStep",
		"LookupTool",
		"validateMutationPermissionBinding",
		"revalidateDispatch",
		"ValidateArguments",
		"checkExecutionScopeGuard",
		"checkRoleBindingGuard",
		"checkPlanApprovalPrecedence",
		"CheckPermissions",
		"CheckToolCall",
		"Decide",
		"evaluateMutationSafetyGuard",
		"acquireToolResourceLocks",
		"EventToolStarted",
		"executeToolWithReadOnlyRetry",
		"EventToolCompleted",
	}
	positions := firstPipelinePositions(dispatch.Body, wantOrder)
	previous := token.NoPos
	for _, name := range wantOrder {
		position := positions[name]
		if position == token.NoPos {
			t.Errorf("dispatch pipeline is missing locked control call %q", name)
			continue
		}
		if previous != token.NoPos && position <= previous {
			t.Errorf("dispatch pipeline control order changed at %q", name)
		}
		previous = position
	}
}

func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverTypeName(typed.X)
	default:
		return ""
	}
}

func assertMethodOwner(t *testing.T, methods map[string][]string, method, want string) {
	t.Helper()
	owners := methods[method]
	if len(owners) != 1 || owners[0] != want {
		t.Errorf("(*ToolDispatcher).%s owner = %v, want exactly [%s]", method, owners, want)
	}
}

func methodDecl(file *ast.File, receiver, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		if receiverTypeName(fn.Recv.List[0].Type) == receiver {
			return fn
		}
	}
	return nil
}

func firstPipelinePositions(body *ast.BlockStmt, names []string) map[string]token.Pos {
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	positions := make(map[string]token.Pos, len(names))
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			name := calledPipelineName(typed.Fun)
			if _, ok := wanted[name]; ok && positions[name] == token.NoPos {
				positions[name] = typed.Pos()
			}
		case *ast.Ident:
			if (typed.Name == "EventToolStarted" || typed.Name == "EventToolCompleted") && positions[typed.Name] == token.NoPos {
				positions[typed.Name] = typed.Pos()
			}
		}
		return true
	})
	return positions
}

func calledPipelineName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	default:
		return ""
	}
}
