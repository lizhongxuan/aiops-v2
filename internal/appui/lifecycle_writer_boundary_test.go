package appui

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"aiops-v2/internal/runtimekernel"
)

type appUILifecycleWriteSite struct {
	file     string
	function string
	line     int
	kind     string
}

func (s appUILifecycleWriteSite) String() string {
	return fmt.Sprintf("%s:%d %s (%s)", s.file, s.line, s.function, s.kind)
}

// RuntimeKernel is the unique owner of TurnSnapshot lifecycle transitions.
// AppUI may translate commands and project snapshots, but it must not fabricate
// terminal runtime state to make a synchronous response visible to transport.
func TestAppUIRuntimeLifecycleHasUniqueWriter(t *testing.T) {
	sites := appUILifecycleWriteSites(t)
	if len(sites) != 0 {
		formatted := make([]string, 0, len(sites))
		for _, site := range sites {
			formatted = append(formatted, site.String())
		}
		t.Fatalf("runtime lifecycle writers outside runtimekernel:\n%s", strings.Join(formatted, "\n"))
	}
}

func TestChatServiceHasNoRuntimeLifecycleWriter(t *testing.T) {
	var chatSites []appUILifecycleWriteSite
	for _, site := range appUILifecycleWriteSites(t) {
		if site.file == "chat_service.go" {
			chatSites = append(chatSites, site)
		}
	}
	if len(chatSites) != 0 {
		t.Fatalf("chat_service runtime lifecycle writers = %#v, want none", chatSites)
	}
}

func TestGenericOpsRepairServiceHasNoRuntimeLifecycleWriter(t *testing.T) {
	var repairSites []appUILifecycleWriteSite
	for _, site := range appUILifecycleWriteSites(t) {
		if site.file == "generic_ops_repair_service.go" {
			repairSites = append(repairSites, site)
		}
	}
	if len(repairSites) != 0 {
		t.Fatalf("generic ops repair runtime lifecycle writers = %#v, want none", repairSites)
	}
}

func TestWorkflowGenerationServiceHasNoRuntimeLifecycleWriter(t *testing.T) {
	var workflowSites []appUILifecycleWriteSite
	for _, site := range appUILifecycleWriteSites(t) {
		if site.file == "workflow_generation_service.go" {
			workflowSites = append(workflowSites, site)
		}
	}
	if len(workflowSites) != 0 {
		t.Fatalf("workflow generation runtime lifecycle writers = %#v, want none", workflowSites)
	}
}

func appUILifecycleWriteSites(t *testing.T) []appUILifecycleWriteSite {
	t.Helper()
	var sites []appUILifecycleWriteSite
	walkAppUIProductionFunctions(t, func(file string, fset *token.FileSet, function *ast.FuncDecl) {
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.CompositeLit:
				selector, ok := value.Type.(*ast.SelectorExpr)
				owner, ownerOK := selectorOwner(selector)
				if ok && ownerOK && owner == "runtimekernel" && selector.Sel.Name == "TurnSnapshot" {
					sites = append(sites, appUILifecycleSite(fset, file, function, value.Pos(), "constructs runtimekernel.TurnSnapshot"))
				}
			case *ast.AssignStmt:
				for _, target := range value.Lhs {
					selector, ok := target.(*ast.SelectorExpr)
					if !ok || (selector.Sel.Name != "CurrentTurn" && selector.Sel.Name != "TurnHistory") {
						continue
					}
					sites = append(sites, appUILifecycleSite(fset, file, function, selector.Pos(), "assigns session."+selector.Sel.Name))
				}
			}
			return true
		})
	})

	sort.Slice(sites, func(i, j int) bool {
		if sites[i].file != sites[j].file {
			return sites[i].file < sites[j].file
		}
		return sites[i].line < sites[j].line
	})
	return sites
}

func TestWorkflowMigrationDoesNotPersistTerminalLifecycleFromAppUI(t *testing.T) {
	sessions := newLifecycleWriteCaptureStore()
	runtime := &chatRuntimeCapture{}
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))

	_, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-workflow-migration-lifecycle-owner",
		Content:         "@add_workflow 每天早上8点自动抓取AI行业新闻",
		ClientMessageID: "client-msg-workflow-migration-lifecycle-owner",
		ClientTurnID:    "client-turn-workflow-migration-lifecycle-owner",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if writes := sessions.terminalWritesSnapshot(); len(writes) != 0 {
		t.Fatalf("appui persisted terminal workflow-migration lifecycle = %#v; runtimekernel must be the writer", writes)
	}
	if _, called := runtime.systemSnapshot(); !called {
		t.Fatal("workflow migration did not cross the system-turn gateway")
	}
}

func TestGenericOpsRepairDoesNotPersistTerminalLifecycleFromAppUI(t *testing.T) {
	sessions := newLifecycleWriteCaptureStore()
	runtime := newBlockingChatRuntime()
	t.Cleanup(func() { close(runtime.release) })
	service := NewChatService(runtime, sessions, NewAgentEventService(nil))

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID: "sess-generic-ops-lifecycle-owner",
		Content:   "主机A和主机B的PG主从集群异常，请帮忙恢复，数据可以不要，只需要PG主从集群正常运行，pg_mon部署在主机C。",
		Metadata: map[string]string{
			"aiops.genericOpsRepairDraftOnly": "true",
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !strings.Contains(result.Output, "stateful_middleware_cluster_repair") {
		t.Fatalf("Output = %q, want production generic-ops repair branch", result.Output)
	}
	if writes := sessions.terminalWritesSnapshot(); len(writes) != 0 {
		t.Fatalf("appui persisted terminal generic-ops lifecycle = %#v; runtimekernel must be the writer", writes)
	}
}

type lifecycleWriteCaptureStore struct {
	delegate *runtimekernel.SessionManager
	mu       sync.Mutex
	writes   []string
}

func newLifecycleWriteCaptureStore() *lifecycleWriteCaptureStore {
	return &lifecycleWriteCaptureStore{delegate: runtimekernel.NewSessionManager()}
}

func (s *lifecycleWriteCaptureStore) Get(id string) *runtimekernel.SessionState {
	return s.delegate.Get(id)
}

func (s *lifecycleWriteCaptureStore) GetLatest() *runtimekernel.SessionState {
	return s.delegate.GetLatest()
}

func (s *lifecycleWriteCaptureStore) List() []*runtimekernel.SessionState {
	return s.delegate.List()
}

func (s *lifecycleWriteCaptureStore) GetOrCreate(sessionID string, sessionType runtimekernel.SessionType, mode runtimekernel.Mode) *runtimekernel.SessionState {
	return s.delegate.GetOrCreate(sessionID, sessionType, mode)
}

func (s *lifecycleWriteCaptureStore) Update(session *runtimekernel.SessionState) {
	if session != nil && session.CurrentTurn != nil && session.CurrentTurn.Lifecycle.IsTerminal() {
		s.mu.Lock()
		s.writes = append(s.writes, session.ID+":"+session.CurrentTurn.ID+":"+string(session.CurrentTurn.Lifecycle))
		s.mu.Unlock()
	}
	s.delegate.Update(session)
}

func (s *lifecycleWriteCaptureStore) terminalWritesSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.writes...)
}

func walkAppUIProductionFunctions(t *testing.T, visit func(string, *token.FileSet, *ast.FuncDecl)) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir(appui): %v", err)
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

func selectorOwner(selector *ast.SelectorExpr) (string, bool) {
	if selector == nil {
		return "", false
	}
	owner, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	return owner.Name, true
}

func appUILifecycleSite(fset *token.FileSet, file string, function *ast.FuncDecl, pos token.Pos, kind string) appUILifecycleWriteSite {
	return appUILifecycleWriteSite{
		file:     file,
		function: function.Name.Name,
		line:     fset.Position(pos).Line,
		kind:     kind,
	}
}
