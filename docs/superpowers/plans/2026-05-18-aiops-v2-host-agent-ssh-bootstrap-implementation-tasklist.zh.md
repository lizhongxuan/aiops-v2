# aiops-v2 Host Agent SSH Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 基于 `docs/superpowers/specs/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-design.zh.md` 落地“添加主机 -> SSH 连通 -> Runner 多步骤 `script.shell` 安装 host-agent -> 启动 -> 心跳在线 -> SSH 终端”的首版闭环。

**Architecture:** `HostService` 负责主机记录，`HostBootstrapService` 负责把安装意图提交给嵌入式 Runner，内置 `builtin.host-agent-install/v1` 工作流由 12 个 `script.shell` 节点组成。host-agent 负责注册和心跳，`TerminalService` 根据 HostRecord 和凭据引用创建 SSH-backed terminal，安装生产链路不调用 LLM。

**Tech Stack:** Go 1.24.3, aiops-v2 `internal/appui`, `internal/server`, `internal/store`, embedded `runner`, Runner visual graph, `script.shell`, React/Vite frontend, Vitest, Playwright.

---

## 0. 实施边界

- [ ] 安装工作流只使用 `script.shell`，不使用 `cmd.run`、`shell.run` 或其他执行节点。
- [ ] 安装生产链路不调用 LLM，不调用 prompt/chat/completion/agent planning/model tool 节点。
- [ ] 首版只支持 `darwin/arm64` 和 `linux/ubuntu`，其他平台返回 `unsupported_platform`。
- [ ] HostRecord、Run Record、日志、页面、Prompt Trace 不保存明文私钥、密码或 token。
- [ ] 添加主机页面不开放任意 SSH 命令输入。
- [ ] SSH 终端不能把远程主机误接成本机 shell。
- [ ] 可见 UI 变更必须更新 Playwright screenshot snapshot。

## 1. 文件结构

### 新增文件

- `internal/appui/credential_resolver.go`
  - 定义 `CredentialResolver`、`ResolvedSSHCredential`，提供本地开发可用的 `secret://` 到私钥文件解析实现。
- `internal/appui/credential_resolver_test.go`
  - 覆盖 secret ref 校验、路径穿越拒绝、临时 key 权限、cleanup。
- `internal/appui/host_install_workflow.go`
  - 构造 `builtin.host-agent-install/v1` Runner visual graph，固定 12 个 `script.shell` 节点。
- `internal/appui/host_install_workflow_test.go`
  - 验证节点数、节点 action、禁止 LLM 类节点、支持平台分支、错误码。
- `internal/appui/host_bootstrap_service.go`
  - 定义 `HostBootstrapService`、`HostBootstrapRunner`，提交安装 run 并回写 HostRecord。
- `internal/appui/host_bootstrap_service_test.go`
  - 使用 fake runner 验证 run vars、idempotency key、状态回写、失败映射。
- `internal/appui/host_agent_service.go`
  - 处理 host-agent register/heartbeat，更新 HostRecord。
- `internal/appui/host_agent_service_test.go`
  - 覆盖 token 校验、注册更新、心跳状态、错误 token 拒绝。
- `internal/server/host_agent_api.go`
  - 暴露 `/api/v1/host-agents/register` 和 `/api/v1/host-agents/heartbeat`。
- `internal/server/host_agent_api_test.go`
  - 覆盖 register/heartbeat HTTP 行为。
- `internal/runnerembed/bootstrap_client.go`
  - 把 embedded runner runtime 包装成 appui 可依赖的 `HostBootstrapRunner`。
- `cmd/host-agent/main.go`
  - host-agent v0 入口，读取配置，注册/心跳，暴露 `/health`、`/run`、`/status`、`/cancel`。
- `cmd/host-agent/main_test.go`
  - 覆盖配置读取、能力列表、心跳 payload、禁止注册 `cmd.run` 和 `shell.run`。
- `internal/hostagent/config.go`
  - host-agent 配置、token 读取、能力列表、server URL 校验。
- `internal/hostagent/config_test.go`
  - 覆盖 `host-agent.yaml` 解析、token file 权限检查、默认能力。
- `internal/terminal/ssh_command.go`
  - 构造远程 SSH terminal 命令并清理临时凭据文件。
- `internal/terminal/ssh_command_test.go`
  - 覆盖命令参数、无明文 secret、key 文件权限和 cleanup。
- `docs/host-agent-install-manual.zh.md`
  - 真实 Ubuntu/macOS 验收准备、凭据引用格式和排障命令。

### 修改文件

- `internal/store/store.go`
  - `HostRecord` 增加 `SSHCredentialRef`、`AgentURL`、`AgentTokenRef`、`InstallRunID`、`InstallWorkflowID`、`InstallStep`。
- `internal/store/gorm_store.go`
  - 确认新增字段可被 GORM 持久化。
- `internal/appui/contracts.go`
  - 扩展 `HostSummary`、`HostUpsert`、`HostMutationResponse`、`HostService`、`HTTPServices`、`Services`。
- `internal/appui/host_service.go`
  - 创建/更新主机时保存凭据引用和 agent version，`installViaSsh=true` 时调用 `HostBootstrapService`。
- `internal/appui/terminal_service.go`
  - 对远程主机创建 SSH-backed terminal session。
- `internal/server/host_api.go`
  - 增加 `POST /api/v1/hosts/{id}/install` 和 `POST /api/v1/hosts/{id}/ssh/test`。
- `internal/server/http.go`
  - 注册 host-agent API 路由。
- `cmd/ai-server/main.go`
  - 将 embedded runner runtime、credential resolver、host bootstrap service、terminal SSH command factory 注入 appui/server。
- `pkg/runner/server/app/runtime.go`
  - 暴露窄接口方法供 embedded runtime 提交 visual graph run、查询 run detail。
- `pkg/runner/engine/defaults.go`
  - 默认 registry 移除 `cmd.run`、`shell.run`。
- `pkg/runner/server/service/action_catalog.go`
  - 默认 action catalog 移除 `cmd.run`、`shell.run`，`script.shell` 默认使用 inline controlled script。
- `pkg/runner/server/service/action_catalog_test.go`
  - 更新默认 action 断言，增加旧 action 缺失断言。
- `pkg/runner/agent/main.go`
  - 若继续复用 runner-agent 代码，能力收敛为 `script.shell`、`script.python`、必要探针。
- `web/src/pages/settingsApi.ts`
  - 扩展 HostRecord 类型和 host install/retry/ssh test API client。
- `web/src/api/hosts.js`
  - 补充 retry install 和 ssh test API client。
- `web/src/lib/hostListViewModel.js`
  - 展示安装 run、失败步骤、unsupported platform、可终端状态。
- `web/src/pages/HostsPage.tsx`
  - 接入主机对话框增加安装开关、凭据引用、agent version、安装状态、重试、Runner 详情入口。
- `web/tests/hostListViewModel.spec.js`
  - 覆盖 installing、install_failed、unsupported_platform、online、terminal enabled。
- `web/tests/e2e/hosts-management-snapshot.spec.js`
  - 更新主机接入 UI snapshot。

## 2. Task 0：建立 baseline 和实施保护线

**Files:**
- Read: `docs/superpowers/specs/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-design.zh.md`
- Read: `docs/2026-05-18-aiops-v2-first-release-scope.zh.md`

- [x] **Step 0.1：记录当前提交和工作区**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
git rev-parse HEAD
git status --short
```

Expected: 输出当前 commit hash；`git status --short` 为空。若不为空，记录已有变更文件，不回滚用户变更。

Result 2026-05-18:

- commit: `c15bf1a3c1e14be25224344416f8d880150906ec`
- branch: `manual_0513`
- `git status --short`: clean

- [x] **Step 0.2：跑当前相关测试作为 baseline**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test -count=1 ./internal/appui ./internal/server ./internal/store ./internal/terminal ./internal/runnerembed
(cd pkg/runner && go test -count=1 ./engine ./server/service ./workflow/visual)
cd web && npm test -- --run hostListViewModel
```

Expected: 以上命令退出码为 0。若已有失败，先记录失败命令和失败测试名，再决定是否单独修复。

Result 2026-05-18:

- `go test -count=1 ./internal/appui ./internal/server ./internal/store ./internal/terminal ./internal/runnerembed`: PASS
- `(cd pkg/runner && go test -count=1 ./engine ./server/service ./workflow/visual)`: PASS
- `cd web && npm test -- --run hostListViewModel`: PASS, 5 tests passed

- [x] **Step 0.3：创建实施分支或确认当前分支**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
git branch --show-current
```

Expected: 输出当前分支名。若当前分支不是实施分支，使用 `git switch -c host-agent-ssh-bootstrap` 创建分支。

Result 2026-05-18: current branch is `manual_0513`; it is not `main` or `master`, so implementation continues on the current branch.

## 3. Task 1：扩展 HostRecord 和 API 契约

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/appui/contracts.go`
- Modify: `internal/appui/host_service.go`
- Test: `internal/appui/host_service_test.go`
- Test: `internal/store/store_property_test.go`

- [ ] **Step 1.1：先写 HostService 契约测试**

在 `internal/appui/host_service_test.go` 增加测试，断言创建 SSH 安装主机会保存凭据引用、agent version 和安装字段：

```go
func TestHostServiceCreateHostStoresSSHInstallContract(t *testing.T) {
	hostRepo := newHostRepoStub()
	service := NewHostService(nil, hostRepo, NewSnapshotBuilder(hostRepo), nil)

	created, err := service.CreateHost(context.Background(), HostUpsert{
		ID:               "prod-web-01",
		Name:             "prod-web-01",
		Address:          "10.0.0.11",
		SSHUser:          "ubuntu",
		SSHPort:          22,
		SSHCredentialRef: "secret://ops/prod-web-01-ssh-key",
		AgentVersion:     "v0.1.0",
		InstallViaSSH:    true,
	})
	if err != nil {
		t.Fatalf("CreateHost() error = %v", err)
	}
	if created.Host.SSHCredentialRef != "secret://ops/prod-web-01-ssh-key" {
		t.Fatalf("SSHCredentialRef = %q", created.Host.SSHCredentialRef)
	}
	if created.Host.AgentVersion != "v0.1.0" {
		t.Fatalf("AgentVersion = %q", created.Host.AgentVersion)
	}
	if created.Host.Transport != "ssh_bootstrap" || created.Host.Status != "installing" || created.Host.InstallState != "pending_install" {
		t.Fatalf("created host state = %+v", created.Host)
	}
}
```

Run:

```bash
go test -count=1 ./internal/appui -run TestHostServiceCreateHostStoresSSHInstallContract
```

Expected: FAIL because `HostUpsert` and `HostSummary` do not yet expose the new fields.

- [ ] **Step 1.2：扩展 HostRecord 和 appui contracts**

Implement:

```go
type HostRecord struct {
	SSHCredentialRef string `json:"sshCredentialRef,omitempty"`
	AgentURL         string `json:"agentUrl,omitempty"`
	AgentTokenRef    string `json:"agentTokenRef,omitempty"`
	InstallRunID     string `json:"installRunId,omitempty"`
	InstallWorkflowID string `json:"installWorkflowId,omitempty"`
	InstallStep      string `json:"installStep,omitempty"`
}
```

同名字段加入 `HostSummary`；`HostUpsert` 增加：

```go
SSHCredentialRef string `json:"sshCredentialRef"`
AgentVersion     string `json:"agentVersion"`
```

`HostMutationResponse` 增加：

```go
InstallRunID      string `json:"installRunId,omitempty"`
InstallWorkflowID string `json:"installWorkflowId,omitempty"`
```

- [ ] **Step 1.3：更新 HostService 映射和默认值**

在 `buildNewHostRecord()` 和 `mapHostRecord()` 中保存并返回新增字段。`installViaSsh=true` 且 `agentVersion` 为空时使用 `v0.1.0`。

- [ ] **Step 1.4：跑契约测试**

Run:

```bash
go test -count=1 ./internal/appui -run 'TestHostServiceCrudAndSelect|TestHostServiceCreateHostStoresSSHInstallContract'
go test -count=1 ./internal/store
```

Expected: PASS。

- [ ] **Step 1.5：提交**

Run:

```bash
git add internal/store/store.go internal/appui/contracts.go internal/appui/host_service.go internal/appui/host_service_test.go
git commit -m "feat: extend host install contract"
```

## 4. Task 2：实现凭据解析边界

**Files:**
- Create: `internal/appui/credential_resolver.go`
- Create: `internal/appui/credential_resolver_test.go`
- Modify: `internal/appui/contracts.go`

- [ ] **Step 2.1：写凭据解析测试**

测试覆盖：

```go
func TestLocalSecretCredentialResolverRejectsPathTraversal(t *testing.T) {}
func TestLocalSecretCredentialResolverWritesTempKey0600AndCleansUp(t *testing.T) {}
func TestLocalSecretCredentialResolverRedactsSecretMaterial(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/appui -run 'TestLocalSecretCredentialResolver'
```

Expected: FAIL because resolver 还不存在。

- [ ] **Step 2.2：实现 resolver 接口和本地 secret 解析**

Implement public contract:

```go
type ResolvedSSHCredential struct {
	Ref             string
	PrivateKeyPath  string
	Password        string
	Cleanup         func() error
}

type CredentialResolver interface {
	ResolveSSHCredential(ctx context.Context, ref string) (ResolvedSSHCredential, error)
}
```

默认实现 `NewLocalSecretCredentialResolver(secretDir string)`：

- `secret://ops/prod-web-01-ssh-key` 映射到 `<secretDir>/ops/prod-web-01-ssh-key`.
- 拒绝 `..`、空路径、绝对路径、反斜杠路径。
- 复制到 `0600` 临时文件，返回临时文件路径。
- `Cleanup()` 删除临时文件。

- [ ] **Step 2.3：注入 Services 配置**

在 `servicesConfig` 增加：

```go
credentialResolver CredentialResolver
```

增加 option：

```go
func WithCredentialResolver(resolver CredentialResolver) ServicesOption
```

- [ ] **Step 2.4：跑测试**

Run:

```bash
go test -count=1 ./internal/appui -run 'TestLocalSecretCredentialResolver|TestHostService'
```

Expected: PASS。

- [ ] **Step 2.5：提交**

Run:

```bash
git add internal/appui/credential_resolver.go internal/appui/credential_resolver_test.go internal/appui/contracts.go
git commit -m "feat: add ssh credential resolver"
```

## 5. Task 3：生成内置 host-agent 安装工作流

**Files:**
- Create: `internal/appui/host_install_workflow.go`
- Create: `internal/appui/host_install_workflow_test.go`

- [ ] **Step 3.1：写 graph 结构测试**

测试必须断言：

```go
func TestBuiltinHostAgentInstallWorkflowUsesOnlyScriptShell(t *testing.T) {}
func TestBuiltinHostAgentInstallWorkflowHasRequiredStepsInOrder(t *testing.T) {}
func TestBuiltinHostAgentInstallWorkflowRejectsModelActions(t *testing.T) {}
```

Required step names:

```go
[]string{
	"validate-inputs",
	"tcp-preflight",
	"ssh-preflight",
	"detect-platform",
	"resolve-artifact",
	"upload-artifact",
	"install-files",
	"install-service",
	"start-service",
	"verify-local-health",
	"verify-aiops-heartbeat",
	"finalize-host",
}
```

Run:

```bash
go test -count=1 ./internal/appui -run 'TestBuiltinHostAgentInstallWorkflow'
```

Expected: FAIL because workflow builder 还不存在。

- [ ] **Step 3.2：实现 workflow builder**

Expose:

```go
const BuiltinHostAgentInstallWorkflowID = "builtin.host-agent-install/v1"

func BuiltinHostAgentInstallGraph() visual.Graph
func ValidateHostAgentInstallGraph(graph visual.Graph) error
```

Validation rules:

- exactly 12 action nodes with required step names.
- each required node has `Step.Action == "script.shell"`.
- forbidden action prefixes: `llm.`, `prompt.`, `chat.`, `completion.`, `agent.`.
- forbidden actions: `cmd.run`, `shell.run`.
- no user-provided script text is accepted by API payload; scripts come from code templates.

- [ ] **Step 3.3：实现受控 script 模板常量**

Create one function per node:

```go
func hostAgentInstallScript(step string) string
```

Each template must start with:

```sh
set -euo pipefail
```

and emit a machine-readable step marker:

```sh
printf 'RUNNER_EXPORT_install_step=%s\n' 'detect-platform'
```

- [ ] **Step 3.4：跑 workflow 测试**

Run:

```bash
go test -count=1 ./internal/appui -run 'TestBuiltinHostAgentInstallWorkflow'
```

Expected: PASS。

- [ ] **Step 3.5：提交**

Run:

```bash
git add internal/appui/host_install_workflow.go internal/appui/host_install_workflow_test.go
git commit -m "feat: add builtin host agent install workflow"
```

## 6. Task 4：暴露 embedded Runner 的窄提交接口

**Files:**
- Modify: `pkg/runner/server/app/runtime.go`
- Create: `internal/runnerembed/bootstrap_client.go`
- Test: `internal/runnerembed/bootstrap_client_test.go`

- [ ] **Step 4.1：写 runnerembed client 测试**

测试 fake runtime：

```go
func TestBootstrapClientSubmitGraphRunReturnsRunID(t *testing.T) {}
func TestBootstrapClientGetRunReturnsCurrentStep(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/runnerembed -run TestBootstrapClient
```

Expected: FAIL because client 还不存在。

- [ ] **Step 4.2：在 runner runtime 暴露窄方法**

In `pkg/runner/server/app/runtime.go`, add fields:

```go
visualWorkflowSvc *service.VisualWorkflowService
```

and methods:

```go
func (r *Runtime) SubmitGraphRun(ctx context.Context, graph visual.Graph, vars map[string]any, triggeredBy, idempotencyKey string) (*service.RunResponse, error)
func (r *Runtime) GetRun(ctx context.Context, runID string) (*service.RunDetail, error)
```

These methods must delegate to existing services and must not expose general LLM or AI draft functionality.

- [ ] **Step 4.3：实现 appui 依赖接口适配器**

In `internal/runnerembed/bootstrap_client.go`, implement:

```go
type BootstrapClient struct {
	runtime *Runtime
}

func NewBootstrapClient(runtime *Runtime) *BootstrapClient
func (c *BootstrapClient) SubmitHostInstallGraph(ctx context.Context, graph visual.Graph, vars map[string]any, idempotencyKey string) (appui.HostInstallRun, error)
func (c *BootstrapClient) GetHostInstallRun(ctx context.Context, runID string) (appui.HostInstallRun, error)
```

- [ ] **Step 4.4：跑 runnerembed 和 runner service 测试**

Run:

```bash
go test -count=1 ./internal/runnerembed
(cd pkg/runner && go test -count=1 ./server/app ./server/service)
```

Expected: PASS。

- [ ] **Step 4.5：提交**

Run:

```bash
git add pkg/runner/server/app/runtime.go internal/runnerembed/bootstrap_client.go internal/runnerembed/bootstrap_client_test.go
git commit -m "feat: expose embedded runner host install client"
```

## 7. Task 5：实现 HostBootstrapService 并接入 HostService

**Files:**
- Create: `internal/appui/host_bootstrap_service.go`
- Create: `internal/appui/host_bootstrap_service_test.go`
- Modify: `internal/appui/contracts.go`
- Modify: `internal/appui/host_service.go`
- Modify: `cmd/ai-server/main.go`

- [ ] **Step 5.1：写 HostBootstrapService 测试**

Fake runner 断言：

```go
func TestHostBootstrapServiceSubmitsBuiltinWorkflowWithRedactedVars(t *testing.T) {}
func TestHostBootstrapServiceMapsSubmitFailureToInstallFailed(t *testing.T) {}
func TestHostBootstrapServiceUsesStableIdempotencyKey(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/appui -run TestHostBootstrapService
```

Expected: FAIL because service 还不存在。

- [ ] **Step 5.2：定义 HostBootstrapRunner 和 HostInstallRun**

Implement:

```go
type HostInstallRun struct {
	HostID        string
	RunID         string
	WorkflowID    string
	Status        string
	CurrentStep   string
	LastError     string
	Platform      string
	AgentVersion  string
}

type HostBootstrapRunner interface {
	SubmitHostInstallGraph(ctx context.Context, graph visual.Graph, vars map[string]any, idempotencyKey string) (HostInstallRun, error)
	GetHostInstallRun(ctx context.Context, runID string) (HostInstallRun, error)
}
```

- [ ] **Step 5.3：实现 BootstrapService 提交逻辑**

`Install(ctx, host, req)` must:

- validate `host.Address`, `host.SSHUser`, `host.SSHCredentialRef`.
- build vars using only secret refs, never secret material.
- call `ValidateHostAgentInstallGraph(BuiltinHostAgentInstallGraph())`.
- submit with idempotency key `host-agent-install:<hostID>:<agentVersion>`.
- update HostRecord fields: `InstallRunID`, `InstallWorkflowID`, `InstallStep`, `Status`, `InstallState`.

- [ ] **Step 5.4：接入 HostService**

Change constructor:

```go
func NewHostService(writer SessionStore, repo HostRepository, builder *SnapshotBuilder, bootstrap ...*HostBootstrapService) HostService
```

When `InstallViaSSH=true`:

- save pending host.
- call bootstrap service if configured.
- if bootstrap is missing, mark `status=install_failed`, `installState=failed`, `lastError=runner runtime is not configured`.

- [ ] **Step 5.5：在 ai-server 注入 bootstrap client**

In `cmd/ai-server/main.go`:

- build `credentialResolver := appui.NewLocalSecretCredentialResolver(os.Getenv("AIOPS_SECRET_DIR"))`.
- if `runnerRuntime != nil`, build `runnerembed.NewBootstrapClient(runnerRuntime)`.
- pass `appui.WithCredentialResolver(credentialResolver)` and `appui.WithHostBootstrapRunner(...)`.

- [ ] **Step 5.6：跑服务测试**

Run:

```bash
go test -count=1 ./internal/appui -run 'TestHostService|TestHostBootstrapService'
go test -count=1 ./cmd/ai-server
```

Expected: PASS。

- [ ] **Step 5.7：提交**

Run:

```bash
git add internal/appui/host_bootstrap_service.go internal/appui/host_bootstrap_service_test.go internal/appui/contracts.go internal/appui/host_service.go cmd/ai-server/main.go
git commit -m "feat: submit host agent install workflow"
```

## 8. Task 6：补齐 host install 和 SSH preflight API

**Files:**
- Modify: `internal/server/host_api.go`
- Test: `internal/server/host_api_test.go`
- Modify: `internal/appui/contracts.go`
- Modify: `internal/appui/host_service.go`

- [ ] **Step 6.1：写 HTTP API 测试**

Add tests:

```go
func TestHostAPIInstallRetriesHostAgentWorkflow(t *testing.T) {}
func TestHostAPISSHTestRejectsMissingCredentialRef(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/server -run 'TestHostAPIInstall|TestHostAPISSHTest'
```

Expected: FAIL because routes 还不存在。

- [ ] **Step 6.2：扩展 HostService interface**

Add:

```go
InstallHost(ctx context.Context, hostID string, payload HostInstallRequest) (HostMutationResponse, error)
TestHostSSH(ctx context.Context, hostID string, payload HostSSHTestRequest) (HostSSHTestResponse, error)
```

Data types:

```go
type HostInstallRequest struct {
	AgentVersion     string `json:"agentVersion"`
	SSHCredentialRef string `json:"sshCredentialRef"`
	Force            bool   `json:"force"`
}

type HostSSHTestRequest struct {
	SSHCredentialRef string `json:"sshCredentialRef"`
}

type HostSSHTestResponse struct {
	Status   string `json:"status"`
	Platform string `json:"platform,omitempty"`
	OS       string `json:"os,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Sudo     string `json:"sudo,omitempty"`
	Message  string `json:"message,omitempty"`
}
```

- [ ] **Step 6.3：路由实现**

In `handleHosts`, dispatch:

- `POST /api/v1/hosts/{id}/install`
- `POST /api/v1/hosts/{id}/ssh/test`

Ensure `/api/v1/hosts` POST/GET and `/api/v1/hosts/{id}` PUT/DELETE behavior remains unchanged.

- [ ] **Step 6.4：跑 HTTP 测试**

Run:

```bash
go test -count=1 ./internal/server -run 'TestHostAPI'
```

Expected: PASS。

- [ ] **Step 6.5：提交**

Run:

```bash
git add internal/server/host_api.go internal/server/host_api_test.go internal/appui/contracts.go internal/appui/host_service.go
git commit -m "feat: add host install api"
```

## 9. Task 7：实现 host-agent register 和 heartbeat

**Files:**
- Create: `internal/appui/host_agent_service.go`
- Create: `internal/appui/host_agent_service_test.go`
- Create: `internal/server/host_agent_api.go`
- Create: `internal/server/host_agent_api_test.go`
- Modify: `internal/appui/contracts.go`
- Modify: `internal/server/http.go`

- [ ] **Step 7.1：写 appui 服务测试**

Test cases:

```go
func TestHostAgentServiceRegisterMarksHostManagedOnline(t *testing.T) {}
func TestHostAgentServiceHeartbeatUpdatesLastHeartbeat(t *testing.T) {}
func TestHostAgentServiceRejectsWrongToken(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/appui -run TestHostAgentService
```

Expected: FAIL because service 还不存在。

- [ ] **Step 7.2：实现 HostAgentService**

Add interface:

```go
type HostAgentService interface {
	Register(ctx context.Context, req HostAgentRegisterRequest, token string) (HostAgentRegisterResponse, error)
	Heartbeat(ctx context.Context, req HostAgentHeartbeatRequest, token string) (HostAgentHeartbeatResponse, error)
}
```

Register updates:

- `Status=online`
- `InstallState=installed`
- `ControlMode=managed`
- `Transport=agent_http`
- `TerminalCapable=true`
- `Executable=true`
- `OS`, `Arch`, `AgentVersion`, `LastHeartbeat`

- [ ] **Step 7.3：写 HTTP handler**

Routes:

- `POST /api/v1/host-agents/register`
- `POST /api/v1/host-agents/heartbeat`

Token sources:

- `Authorization: Bearer <token>`
- `X-Host-Agent-Token: <token>`

- [ ] **Step 7.4：跑 host-agent API 测试**

Run:

```bash
go test -count=1 ./internal/appui -run TestHostAgentService
go test -count=1 ./internal/server -run TestHostAgentAPI
```

Expected: PASS。

- [ ] **Step 7.5：提交**

Run:

```bash
git add internal/appui/host_agent_service.go internal/appui/host_agent_service_test.go internal/server/host_agent_api.go internal/server/host_agent_api_test.go internal/appui/contracts.go internal/server/http.go
git commit -m "feat: add host agent heartbeat api"
```

## 10. Task 8：实现 host-agent v0 二进制和 artifact manifest

**Files:**
- Create: `internal/hostagent/config.go`
- Create: `internal/hostagent/config_test.go`
- Create: `cmd/host-agent/main.go`
- Create: `cmd/host-agent/main_test.go`
- Modify: `pkg/runner/agent/main.go`
- Create: `artifacts/host-agent/manifest.json`

- [ ] **Step 8.1：写 hostagent config 测试**

Test cases:

```go
func TestHostAgentConfigLoadsYAMLAndTokenFile(t *testing.T) {}
func TestHostAgentDefaultCapabilitiesExcludeLegacyShellActions(t *testing.T) {}
func TestHostAgentConfigRejectsMissingServerURL(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/hostagent -run TestHostAgent
```

Expected: FAIL because package 还不存在。

- [ ] **Step 8.2：实现 host-agent 配置和能力**

Default capabilities:

```go
[]string{"script.shell", "script.python", "terminal"}
```

The list must not include:

```go
[]string{"cmd.run", "shell.run"}
```

- [ ] **Step 8.3：实现 cmd/host-agent**

Behavior:

- read `--config /etc/aiops/host-agent.yaml`.
- register on startup.
- heartbeat every `heartbeat_interval`.
- expose `/health`, `/run`, `/status`, `/cancel`.
- register only `script.shell` and `script.python` execution modules.

- [ ] **Step 8.4：创建 artifact manifest**

`artifacts/host-agent/manifest.json` shape:

```json
{
  "version": "v0.1.0",
  "artifacts": [
    { "platform": "linux/ubuntu", "os": "linux", "arch": "amd64", "path": "artifacts/host-agent/v0.1.0/linux-amd64/host-agent", "sha256": "" },
    { "platform": "darwin/arm64", "os": "darwin", "arch": "arm64", "path": "artifacts/host-agent/v0.1.0/darwin-arm64/host-agent", "sha256": "" }
  ]
}
```

During implementation, fill `sha256` from the built binary.

- [ ] **Step 8.5：跑 host-agent 测试**

Run:

```bash
go test -count=1 ./internal/hostagent ./cmd/host-agent
(cd pkg/runner && go test -count=1 ./agent)
```

Expected: PASS。

- [ ] **Step 8.6：提交**

Run:

```bash
git add internal/hostagent cmd/host-agent pkg/runner/agent/main.go artifacts/host-agent/manifest.json
git commit -m "feat: add host agent binary"
```

## 11. Task 9：收敛 Runner 默认 action 和 validator

**Files:**
- Modify: `pkg/runner/engine/defaults.go`
- Modify: `pkg/runner/server/service/action_catalog.go`
- Modify: `pkg/runner/server/service/action_catalog_test.go`
- Modify: `pkg/runner/workflow/visual/validate.go`
- Test: `pkg/runner/workflow/visual/visual_test.go`

- [ ] **Step 9.1：写默认 action 测试**

Update tests so default catalog contains `script.shell`, `script.python`, `wait.event` and does not contain `cmd.run` or `shell.run`.

Run:

```bash
(cd pkg/runner && go test -count=1 ./engine ./server/service -run 'TestDefault|TestActionCatalog')
```

Expected: FAIL before implementation because legacy actions still exist.

- [ ] **Step 9.2：移除默认 registry 里的旧 action**

In `pkg/runner/engine/defaults.go`, remove:

```go
reg.Register("cmd.run", cmd.New())
reg.Register("shell.run", shell.New())
```

and remove unused imports.

- [ ] **Step 9.3：移除 action catalog 旧 action**

In `DefaultActionSpecs()`, remove `cmd.run` and `shell.run` specs. Change `script.shell` defaults:

```go
Defaults: map[string]any{"script": "set -euo pipefail\necho ok"}
```

- [ ] **Step 9.4：新增模型节点拒绝测试**

Add visual validation test that a graph with `Action: "llm.generate"` fails when validated as host-agent install workflow via `ValidateHostAgentInstallGraph`.

- [ ] **Step 9.5：跑 runner 测试**

Run:

```bash
(cd pkg/runner && go test -count=1 ./engine ./server/service ./workflow/visual)
```

Expected: PASS。

- [ ] **Step 9.6：提交**

Run:

```bash
git add pkg/runner/engine/defaults.go pkg/runner/server/service/action_catalog.go pkg/runner/server/service/action_catalog_test.go pkg/runner/workflow/visual/validate.go pkg/runner/workflow/visual/visual_test.go
git commit -m "feat: restrict runner shell actions"
```

## 12. Task 10：实现 SSH-backed terminal

**Files:**
- Create: `internal/terminal/ssh_command.go`
- Create: `internal/terminal/ssh_command_test.go`
- Modify: `internal/appui/terminal_service.go`
- Modify: `internal/appui/terminal_service_test.go`
- Modify: `cmd/ai-server/main.go`

- [ ] **Step 10.1：写 SSH command 测试**

Test cases:

```go
func TestSSHCommandFactoryBuildsSSHArgsWithoutSecretMaterial(t *testing.T) {}
func TestSSHCommandFactoryRejectsMissingCredentialRef(t *testing.T) {}
func TestSSHCommandFactoryCleansTempKeyOnExit(t *testing.T) {}
```

Run:

```bash
go test -count=1 ./internal/terminal -run TestSSHCommandFactory
```

Expected: FAIL because factory 还不存在。

- [ ] **Step 10.2：实现 SSH command factory**

Command shape:

```text
ssh -tt -o StrictHostKeyChecking=accept-new -o ServerAliveInterval=15 -p <port> -i <temp_key> <user>@<address>
```

Rules:

- never put password or private key content in args.
- temp key file permission is `0600`.
- session cleanup removes temp key.
- server-local still uses local shell.

- [ ] **Step 10.3：接入 TerminalService**

For non-`server-local` hosts:

- require `Status=online`.
- require `TerminalCapable || Executable`.
- require `SSHCredentialRef`.
- create SSH-backed session, not local shell.

- [ ] **Step 10.4：跑 terminal 测试**

Run:

```bash
go test -count=1 ./internal/terminal ./internal/appui -run 'TestSSHCommandFactory|TestTerminalService'
```

Expected: PASS。

- [ ] **Step 10.5：提交**

Run:

```bash
git add internal/terminal/ssh_command.go internal/terminal/ssh_command_test.go internal/appui/terminal_service.go internal/appui/terminal_service_test.go cmd/ai-server/main.go
git commit -m "feat: add ssh backed terminal sessions"
```

## 13. Task 11：更新主机管理前端

**Files:**
- Modify: `web/src/pages/settingsApi.ts`
- Modify: `web/src/api/hosts.js`
- Modify: `web/src/lib/hostListViewModel.js`
- Modify: `web/src/pages/HostsPage.tsx`
- Test: `web/tests/hostListViewModel.spec.js`
- Test: `web/tests/e2e/hosts-management-snapshot.spec.js`

- [ ] **Step 11.1：写 view model 测试**

Add cases:

```js
it("surfaces install run details and retry action for failed installs", () => {})
it("marks unsupported platform with a dedicated label", () => {})
it("enables terminal only when remote host is online and terminal capable", () => {})
```

Run:

```bash
cd web
npm test -- --run hostListViewModel
```

Expected: FAIL before view model update.

- [ ] **Step 11.2：扩展 API client**

Add:

```ts
export function retryHostInstall(hostId: string, payload: JsonMap) {
  return request(`/api/v1/hosts/${encodeURIComponent(hostId)}/install`, { method: "POST", body: payload });
}

export function testHostSSH(hostId: string, payload: JsonMap) {
  return request(`/api/v1/hosts/${encodeURIComponent(hostId)}/ssh/test`, { method: "POST", body: payload });
}
```

- [ ] **Step 11.3：更新接入主机对话框**

`HostDraft` fields:

```ts
installViaSsh: boolean;
sshCredentialRef: string;
agentVersion: string;
```

Create payload includes:

```ts
installViaSsh: draft.installViaSsh,
sshCredentialRef: draft.sshCredentialRef,
agentVersion: draft.agentVersion || "v0.1.0",
```

- [ ] **Step 11.4：更新列表状态和操作**

Rows must show:

- installing: current `installStep` and `installRunId`.
- failed: `lastError`, retry button.
- unsupported platform: label `不支持的平台`.
- online: terminal button enabled.
- Runner detail link: `/runner-studio/runs/<installRunId>`.

- [ ] **Step 11.5：跑前端单测**

Run:

```bash
cd web
npm test -- --run hostListViewModel settingsApi
```

Expected: PASS。

- [ ] **Step 11.6：更新 Playwright snapshot**

Run:

```bash
cd web
npm run test:e2e -- hosts-management-snapshot.spec.js --update-snapshots
```

Expected: PASS and updated snapshot files under `web/tests/__screenshots__/` or the configured snapshot directory.

- [ ] **Step 11.7：提交**

Run:

```bash
git add web/src/pages/settingsApi.ts web/src/api/hosts.js web/src/lib/hostListViewModel.js web/src/pages/HostsPage.tsx web/tests/hostListViewModel.spec.js web/tests/e2e/hosts-management-snapshot.spec.js web/tests/__screenshots__ web/tests/screenshots
git commit -m "feat: add host ssh install ui"
```

## 14. Task 12：真实环境安装验收脚本和文档

**Files:**
- Create: `docs/host-agent-install-manual.zh.md`
- Modify: `docs/superpowers/specs/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-design.zh.md`

- [ ] **Step 12.1：写验收文档**

Include exact sections:

- Ubuntu 准备：SSH key、passwordless sudo、systemd、开放 host-agent port。
- macOS arm64 准备：Remote Login、sudo、launchd、Apple Silicon 确认。
- `AIOPS_SECRET_DIR` 凭据目录格式。
- 添加主机请求示例。
- 失败场景：错误凭据、端口不通、无 sudo、不支持平台。
- 证据保留：Run ID、Run Record、截图、service status。

- [ ] **Step 12.2：补充设计文档实施链接**

Add a short implementation reference section linking:

```markdown
实施任务清单：`docs/superpowers/plans/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-implementation-tasklist.zh.md`
```

- [ ] **Step 12.3：文档自检**

Run:

```bash
AIOPS_REDFLAG_PATTERN="$(printf '%s|%s|%s|%s|%s|%s|%s' 'T''BD' '待''定' '\?''\?\?' 'place''holder' '以后''再' '随''便' '未''定义')"
rg -n "$AIOPS_REDFLAG_PATTERN" docs/host-agent-install-manual.zh.md docs/superpowers/specs/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-design.zh.md
git diff --check -- docs/host-agent-install-manual.zh.md docs/superpowers/specs/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-design.zh.md
```

Expected: first command exits 1 with no matches; second command exits 0.

- [ ] **Step 12.4：提交**

Run:

```bash
git add docs/host-agent-install-manual.zh.md docs/superpowers/specs/2026-05-18-aiops-v2-host-agent-ssh-bootstrap-design.zh.md
git commit -m "docs: add host agent install acceptance guide"
```

## 15. Task 13：端到端验证

**Files:**
- Read: `docs/host-agent-install-manual.zh.md`
- Read: `.data/runner/run-state.json`
- Read: `.data/runner/run-records.jsonl`

- [ ] **Step 13.1：跑后端测试集合**

Run:

```bash
go test -count=1 ./internal/appui ./internal/server ./internal/store ./internal/terminal ./internal/runnerembed ./internal/hostagent
(cd pkg/runner && go test -count=1 ./engine ./server/app ./server/service ./workflow/visual ./agent)
```

Expected: PASS。

- [ ] **Step 13.2：跑前端测试集合**

Run:

```bash
cd web
npm test -- --run hostListViewModel settingsApi
npm run test:e2e -- hosts-management-snapshot.spec.js
```

Expected: PASS。

- [ ] **Step 13.3：真实 Ubuntu 验收**

Run through UI or API:

```bash
curl -sS -X POST http://127.0.0.1:18080/api/v1/hosts \
  -H 'Content-Type: application/json' \
  -d '{"id":"ubuntu-smoke","name":"ubuntu-smoke","address":"<ubuntu-ip>","sshUser":"ubuntu","sshPort":22,"sshCredentialRef":"secret://lab/ubuntu-smoke","installViaSsh":true,"agentVersion":"v0.1.0"}'
```

Expected:

- response includes `installRunId`.
- Runner run contains 12 `script.shell` steps.
- Ubuntu service `aiops-host-agent.service` is active.
- HostRecord becomes `online` and `installState=installed`.
- Terminal opens remote SSH shell.

- [ ] **Step 13.4：真实 macOS arm64 验收**

Run through UI or API with macOS host:

```bash
curl -sS -X POST http://127.0.0.1:18080/api/v1/hosts \
  -H 'Content-Type: application/json' \
  -d '{"id":"macos-smoke","name":"macos-smoke","address":"<mac-ip>","sshUser":"<user>","sshPort":22,"sshCredentialRef":"secret://lab/macos-smoke","installViaSsh":true,"agentVersion":"v0.1.0"}'
```

Expected:

- response includes `installRunId`.
- launchd service `com.aiops.host-agent` is bootstrapped.
- HostRecord becomes `online` and `installState=installed`.
- Terminal opens remote SSH shell.

- [ ] **Step 13.5：失败路径验收**

Run three negative cases:

- bad credential -> `auth_failed`.
- closed SSH port -> `ssh_unreachable`.
- unsupported platform -> `unsupported_platform`.

Expected: HostRecord remains not online, `lastError` is redacted, `installStep` points to failing step, Runner Run Record exists.

- [ ] **Step 13.6：最终安全扫描**

Run:

```bash
rg -n 'BEGIN OPENSSH PRIVATE KEY|BEGIN RSA PRIVATE KEY|password=|Authorization: Bearer [A-Za-z0-9._-]+' .data docs web/src internal pkg/runner
```

Expected: no matches for real secrets. Test fixtures may contain dummy strings only when clearly named dummy/example.

- [ ] **Step 13.7：最终提交或 PR 准备**

Run:

```bash
git status --short
git log --oneline -n 12
```

Expected: status is clean after final commit; log shows small phase commits matching the tasks above.
