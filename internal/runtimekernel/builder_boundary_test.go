package runtimekernel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type productionBuilderSite struct {
	file     string
	function string
	line     int
}

func TestAssemblyBoundarySingleBuilder(t *testing.T) {
	var definitions, wrapperCalls, directCalls []productionBuilderSite
	walkProductionFunctions(t, func(file string, fset *token.FileSet, function *ast.FuncDecl) {
		if function.Name.Name == "buildRuntimeTurnAssembly" {
			definitions = append(definitions, productionSite(fset, file, function.Name.Pos(), function.Name.Name))
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				if target.Name == "buildRuntimeTurnAssembly" {
					wrapperCalls = append(wrapperCalls, productionSite(fset, file, target.Pos(), function.Name.Name))
				}
			case *ast.SelectorExpr:
				owner, ok := target.X.(*ast.Ident)
				if ok && owner.Name == "agentassembly" && target.Sel.Name == "BuildTurnAssembly" {
					directCalls = append(directCalls, productionSite(fset, file, target.Pos(), function.Name.Name))
				}
			}
			return true
		})
	})

	requireProductionBuilderSite(t, "buildRuntimeTurnAssembly definition", definitions, "turn_admission.go", "buildRuntimeTurnAssembly")
	requireProductionBuilderSite(t, "agentassembly.BuildTurnAssembly call", directCalls, "turn_admission.go", "buildRuntimeTurnAssembly")
	if len(wrapperCalls) != 1 {
		t.Fatalf("buildRuntimeTurnAssembly production calls = %#v, want exactly one", wrapperCalls)
	}
}

func TestRuntimeStepContextSingleProductionBuilder(t *testing.T) {
	var definitions, composites, freezeCalls []productionBuilderSite
	walkProductionFunctions(t, func(file string, fset *token.FileSet, function *ast.FuncDecl) {
		if function.Name.Name == "buildRuntimeStepContext" {
			definitions = append(definitions, productionSite(fset, file, function.Name.Pos(), function.Name.Name))
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CompositeLit:
				typeName, ok := value.Type.(*ast.Ident)
				if ok && typeName.Name == "RuntimeStepContext" && len(value.Elts) > 0 {
					composites = append(composites, productionSite(fset, file, value.Pos(), function.Name.Name))
				}
			case *ast.CallExpr:
				target, ok := value.Fun.(*ast.Ident)
				if ok && target.Name == "FreezeRuntimeStepContext" {
					freezeCalls = append(freezeCalls, productionSite(fset, file, target.Pos(), function.Name.Name))
				}
			}
			return true
		})
	})

	requireProductionBuilderSite(t, "buildRuntimeStepContext definition", definitions, "step_builder.go", "buildRuntimeStepContext")
	requireProductionBuilderSite(t, "non-empty RuntimeStepContext literal", composites, "step_builder.go", "buildRuntimeStepContext")
	requireProductionBuilderSite(t, "FreezeRuntimeStepContext call", freezeCalls, "step_builder.go", "buildRuntimeStepContext")
}

func TestRuntimeTurnLoopHasNoCopiedTestEntrypoint(t *testing.T) {
	var runTurnDefinitions, copiedEntrypoints, legacyExecutors []productionBuilderSite
	walkProductionFunctions(t, func(file string, fset *token.FileSet, function *ast.FuncDecl) {
		if function.Recv != nil && function.Name.Name == "RunTurn" {
			runTurnDefinitions = append(runTurnDefinitions, productionSite(fset, file, function.Name.Pos(), function.Name.Name))
		}
		if function.Recv != nil && strings.HasPrefix(function.Name.Name, "RunTurn") && function.Name.Name != "RunTurn" {
			copiedEntrypoints = append(copiedEntrypoints, productionSite(fset, file, function.Name.Pos(), function.Name.Name))
		}
		if function.Name.Name == "executeAgent" {
			legacyExecutors = append(legacyExecutors, productionSite(fset, file, function.Name.Pos(), function.Name.Name))
		}
	})
	requireProductionBuilderSite(t, "RuntimeKernel.RunTurn definition", runTurnDefinitions, "runtime_kernel.go", "RunTurn")
	if len(copiedEntrypoints) != 0 {
		t.Fatalf("copied RunTurn production entrypoints = %#v, want none", copiedEntrypoints)
	}
	if len(legacyExecutors) != 0 {
		t.Fatalf("legacy direct agent executors = %#v, want none", legacyExecutors)
	}
}

func walkProductionFunctions(t *testing.T, visit func(string, *token.FileSet, *ast.FuncDecl)) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(runtimekernel): %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fset, name, nil, 0)
		if parseErr != nil {
			t.Fatalf("ParseFile(%s): %v", name, parseErr)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil {
				visit(name, fset, function)
			}
		}
	}
}

func productionSite(fset *token.FileSet, file string, pos token.Pos, function string) productionBuilderSite {
	return productionBuilderSite{file: filepath.Base(file), function: function, line: fset.Position(pos).Line}
}

func requireProductionBuilderSite(t *testing.T, label string, sites []productionBuilderSite, file, function string) {
	t.Helper()
	if len(sites) != 1 {
		t.Fatalf("%s sites = %#v, want exactly one", label, sites)
	}
	if sites[0].file != file || sites[0].function != function {
		t.Fatalf("%s owner = %s:%d %s, want %s %s", label, sites[0].file, sites[0].line, sites[0].function, file, function)
	}
}
