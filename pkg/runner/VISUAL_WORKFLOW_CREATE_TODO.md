# Runner 可视化新增工作流生产级实施清单

本文档是 `VISUAL_WORKFLOW_UI_TODO.md` 的补充清单，专门覆盖“如何在可视化编排 UI 中新增工作流”。目标是补齐生产级工作流创建链路：用户在 Runner 工作台中可以从模板、克隆、YAML 导入创建 workflow，创建后进入同一个 graph 编辑、保存、发布、dry-run、运行生命周期。

执行要求：

- 每完成一个任务，必须把对应复选框从 `[ ]` 更新为 `[x]`。
- 不新增第二套工作流机制；Graph 是 UI 唯一编辑模型，YAML 只是兼容执行格式。
- 不走“前端先编译 YAML 再绕到普通 YAML 创建”的长期方案；生产级入口必须是 graph-native create。
- 旧 `POST /api/v1/workflows` YAML 创建能力继续保留，供 CLI/API 兼容使用。
- 所有创建、导入、克隆、保存、发布都必须进入同一套 audit、version、status、validation、graph compile 流程。

## 0. 当前状态与缺口确认

- [x] 确认当前后端已有 YAML 创建接口：`POST /api/v1/workflows`，实现位于 `pkg/runner/server/api/workflow_handler.go`。
- [x] 确认当前后端 graph API 已有读取、更新、解析、编译、校验、dry-run、运行能力，缺少 graph create。
- [x] 确认当前前端 `RunnerWorkbench.vue` 只有加载、保存、发布、导入、导出、历史、dry-run、运行入口，缺少 `新建工作流`。
- [x] 确认当前 `graphStore.ts` 的 `load()` 默认加载 `service-restart-candidate`，缺少 `createWorkflowFromGraph` / `cloneWorkflow` / `switchWorkflow`。
- [x] 确认 `workflowOptions` 已可承载工作流选择器和重复 name 校验。
- [x] 确认 `importWorkflowBundle()` 是 bundle 导入能力，不等价于生产级新建工作流。

验收：

- [x] 团队明确“新增工作流”不是画布里拖一个节点，而是创建 workflow 资源、加载 graph、进入 draft 生命周期。
- [x] 不再要求用户先手写 YAML 才能进入可视化编辑。

## 1. 产品交互设计

### 1.1 顶部工具栏入口

- [x] 在 `pkg/runner/server/ui/frontend/src/components/RunnerWorkbench.vue` 顶部工具栏左侧增加工作流选择器。
- [x] 工作流选择器展示 `name`、`version`、`status`，优先按 `updated_at` 或 `name` 稳定排序。
- [x] 在工作流选择器旁增加 `新建` 按钮。
- [x] 在更多操作区增加 `克隆当前工作流`。
- [x] 保留现有 `Import`，但文案区分为 `导入 Bundle` 或 `导入 YAML`，避免和新建混淆。
- [x] 当前 graph 有未保存变更时，切换工作流前必须弹出确认。

验收：

- [x] 用户可以从工作台直接创建新 workflow，不需要离开可视化页面。
- [x] 用户可以在多个 workflow 间切换，且不会静默丢失未保存变更。

### 1.2 新建工作流弹窗

- [x] 新增 `pkg/runner/server/ui/frontend/src/components/NewWorkflowModal.vue`。
- [x] 弹窗字段包含 `name`、`description`、`version`、`labels`、`template`、`save_note`。
- [x] `name` 必填，默认不自动生成随机名称；可提供基于模板的建议名称。
- [x] `version` 默认 `v0.1`。
- [x] `save_note` 默认 `initial visual workflow draft`。
- [x] 支持模板：
  - [x] `cmd-run-basic`：Start -> cmd.run -> End。
  - [x] `shell-run-basic`：Start -> shell.run -> End。
  - [x] `manual-approval-basic`：Start -> manual_approval -> cmd.run -> End。
  - [x] `from-yaml`：粘贴 YAML，先 parse 成 graph，再创建。
  - [x] `clone-current`：复制当前 workflow graph，替换 metadata、清理运行态和 resource version。
- [x] 弹窗内展示 name 重复校验结果。
- [x] 弹窗内展示 graph validation 错误摘要。
- [x] 创建按钮在 name 为空、name 重复、graph 校验失败时禁用。

验收：

- [x] 默认模板创建出的 graph 可以直接通过 validate 和 dry-run。
- [x] 克隆不会携带 run state、node state、edge state、resource version。

### 1.3 创建后的页面状态

- [x] 创建成功后自动切换到新 workflow。
- [x] 新 workflow 状态为 `draft`。
- [x] `dirty=false`，`baselineGraph` 等于服务端返回 graph。
- [x] 默认选中第一个 action 节点；没有 action 时选中第一个非 start 节点。
- [x] 清空旧 workflow 的 validation、dryRun、run state、historyPast、historyFuture、clipboardNode。
- [x] 刷新工作流列表，选择器中立即出现新 workflow。
- [x] `yamlPreview` 使用服务端返回 YAML。

验收：

- [x] 新建成功后，用户可以立刻继续拖拽节点、保存 draft、发布、dry-run、运行。

## 2. 后端 API 与服务层

### 2.1 API 契约

- [x] 在 `pkg/runner/server/api/router.go` 注册：

```http
POST /api/v1/workflows/graph
```

- [x] 请求体定义：

```json
{
  "graph": {
    "version": "v1",
    "workflow": {
      "version": "v0.1",
      "name": "example-workflow",
      "description": "Created from visual workflow UI"
    },
    "nodes": [],
    "edges": []
  },
  "labels": {
    "source": "visual-ui"
  },
  "save_note": "initial visual workflow draft"
}
```

- [x] 响应体定义：

```json
{
  "name": "example-workflow",
  "status": "draft",
  "workflow": {
    "version": "v0.1",
    "name": "example-workflow"
  },
  "graph": {},
  "yaml": "version: v0.1\nname: example-workflow\n",
  "warnings": []
}
```

- [x] 错误格式沿用现有 `writeServiceError` / `writeJSONError`。
- [x] name 重复返回 conflict。
- [x] graph 无效返回 invalid，并保留可定位到 node/edge/field 的错误信息。

验收：

- [x] 前端只调用 `POST /api/v1/workflows/graph` 创建可视化工作流。
- [x] CLI 或外部系统仍可调用旧 `POST /api/v1/workflows` 创建 YAML 工作流。

### 2.2 VisualWorkflowService 创建方法

- [x] 在 `pkg/runner/server/service/visual_workflow_service.go` 增加 `VisualWorkflowCreateOptions`：

```go
type VisualWorkflowCreateOptions struct {
	Labels      map[string]string `json:"labels,omitempty"`
	SaveNote    string            `json:"save_note,omitempty"`
	SaveNoteSet bool              `json:"-"`
}
```

- [x] 增加 `CreatedVisualWorkflow`：

```go
type CreatedVisualWorkflow struct {
	Name     string                `json:"name"`
	Status   string                `json:"status"`
	Workflow workflow.Workflow     `json:"workflow"`
	Graph    visual.Graph          `json:"graph"`
	YAML     string                `json:"yaml"`
	Warnings []VisualWorkflowIssue `json:"warnings,omitempty"`
}
```

- [x] 增加 `CreateGraph(ctx context.Context, graph visual.Graph, opts VisualWorkflowCreateOptions)`。
- [x] `CreateGraph` 必须调用 `normalizeGraph`。
- [x] `CreateGraph` 必须拒绝空 workflow name。
- [x] `CreateGraph` 必须清理 `graph.UI.resource_version`。
- [x] `CreateGraph` 必须调用现有 `Compile()`，保证 graph validate、workflow load、workflow validate 全部经过同一套逻辑。
- [x] `CreateGraph` 必须调用现有 `WorkflowService.Create`，不要直接写 repository。
- [x] `CreateGraph` 必须把 labels、description、save note 传给 workflow service。
- [x] `CreateGraph` 创建成功后重新 `GetGraph`，返回带服务端 resource version 的 graph。

验收：

- [x] 创建、保存、发布共享同一套 graph compile 和 workflow version 机制。
- [x] 创建后的 graph 再保存不会触发错误的资源版本冲突。

### 2.3 VisualWorkflowHandler 创建方法

- [x] 在 `pkg/runner/server/api/router.go` 的 `VisualWorkflowHandler` interface 增加 `CreateGraph`。
- [x] 在 `pkg/runner/server/api/visual_workflow_handler.go` 实现 `CreateGraph`。
- [x] 增加 request decoder，支持 `{ graph, labels, save_note }` 包装结构。
- [x] decoder 必须拒绝空 graph。
- [x] 创建成功写 audit：

```go
auditLog(r, "workflow.graph.create", created.Name, map[string]any{
	"name": created.Name,
	"nodes": len(req.Graph.Nodes),
	"edges": len(req.Graph.Edges),
	"save_note": req.SaveNote,
})
```

- [x] 创建成功返回 HTTP `201 Created`。
- [x] 创建失败时不写入半成品 workflow。

验收：

- [x] API handler 单测覆盖成功、重复 name、非法 graph、空 graph、audit。

## 3. Graph 模板与前端类型

### 3.1 前端模板工具

- [x] 新增 `pkg/runner/server/ui/frontend/src/utils/workflowTemplates.ts`。
- [x] 导出模板类型：

```ts
export type WorkflowTemplateKind = "cmd-run-basic" | "shell-run-basic" | "manual-approval-basic";
```

- [x] 导出创建函数：

```ts
export function createWorkflowGraphFromTemplate(input: {
  kind: WorkflowTemplateKind;
  name: string;
  version: string;
  description?: string;
  labels?: Record<string, string>;
}): WorkflowGraph
```

- [x] `cmd-run-basic` 模板生成：
  - [x] `start` 节点。
  - [x] `run-command` action 节点，action 为 `cmd.run`，args 为 `{ "cmd": "echo hello" }`。
  - [x] `end` 节点。
  - [x] `start-run-command` 和 `run-command-end` 两条 `next` 边。
- [x] `shell-run-basic` 模板生成 `shell.run` action，args 为 `{ "script": "echo hello" }`。
- [x] `manual-approval-basic` 模板生成审批节点，subjects 默认 `["ops"]`，timeout 默认 `30m`，on_timeout 默认 `reject`。
- [x] 模板不得写入 `state`、`ui.resource_version`、运行态 overlay。

验收：

- [x] 模板 graph 可被当前 `runnerApi.validateGraph()` 接受。
- [x] 模板节点布局在 LR 方向下不重叠。

### 3.2 前端类型与 API client

- [x] 在 `pkg/runner/server/ui/frontend/src/types/workflow.ts` 增加：

```ts
export interface CreateGraphWorkflowRequest {
  graph: WorkflowGraph;
  labels?: Record<string, string>;
  save_note?: string;
}

export interface CreatedGraphWorkflowResult extends CompiledWorkflowResult {
  name: string;
  status: "draft" | "published" | string;
  graph: WorkflowGraph;
}
```

- [x] 在 `pkg/runner/server/ui/frontend/src/api/client.ts` 增加：

```ts
async createGraphWorkflow(request: CreateGraphWorkflowRequest): Promise<CreatedGraphWorkflowResult> {
  return requestJSON<CreatedGraphWorkflowResult>("/workflows/graph", {
    method: "POST",
    body: JSON.stringify(request),
  });
}
```

- [x] `mockApi` 增加 create mock，返回传入 graph 的深拷贝。

验收：

- [x] 前端所有新增 workflow 创建都通过 `runnerApi.createGraphWorkflow()`。

## 4. Graph Store 状态与动作

### 4.1 Store 状态

- [x] 在 `GraphStoreState` 增加：

```ts
creatingWorkflow: boolean;
switchingWorkflow: boolean;
```

- [x] `initialGraphStoreState()` 初始化上述字段为 `false`。
- [x] `__resetGraphStoreForTests()` 支持测试覆盖上述字段。

验收：

- [x] 新建、切换的 loading 状态不会复用保存或导入状态，UI 文案准确。

### 4.2 创建工作流动作

- [x] 在 `graphStore.ts` 增加：

```ts
async function createWorkflowFromGraph(graph: WorkflowGraph, options: { labels?: Record<string, string>; saveNote?: string } = {}) {
  state.creatingWorkflow = true;
  state.error = null;
  try {
    const result = state.offline
      ? await mockApi.createGraphWorkflow({ graph, labels: options.labels, save_note: options.saveNote })
      : await runnerApi.createGraphWorkflow({ graph, labels: options.labels, save_note: options.saveNote });
    state.graph = result.graph;
    state.baselineGraph = cloneGraph(result.graph);
    state.workflowStatus = result.status || "draft";
    state.workflowVersions = [];
    state.selectedNodeId = selectInitialEditableNode(result.graph);
    state.saveNote = "";
    state.validation = null;
    state.dryRun = null;
    state.yamlPreview = result.yaml || "";
    state.riskAcknowledged = false;
    state.warningAcknowledged = false;
    state.semanticChangeAcknowledged = false;
    state.dirty = false;
    state.run = createInitialRunState();
    clearEditSession();
    state.workflowOptions = await runnerApi.listWorkflows();
    state.offline = false;
  } catch (error) {
    state.error = errorMessage(error);
  } finally {
    state.creatingWorkflow = false;
  }
}
```

- [x] 增加 `selectInitialEditableNode(graph)`，优先 action，其次 manual_approval/subflow/condition，最后非 start 节点。
- [x] 创建成功后必须关闭旧 SSE 订阅，避免旧 run 事件污染新 workflow。
- [x] 创建失败不得覆盖当前正在编辑的 graph。

验收：

- [x] 创建失败时用户仍停留在原 workflow，原 graph 不丢失。
- [x] 创建成功后状态等同于刚从服务端加载该 workflow。

### 4.3 切换与克隆

- [x] 增加 `switchWorkflow(name: string, options?: { force?: boolean })`。
- [x] `switchWorkflow` 当 `dirty=true` 且 `force` 未设置时返回明确错误或需要确认的状态。
- [x] `switchWorkflow` 成功时复用 `load(name)` 的状态重置逻辑。
- [x] 增加 `cloneCurrentWorkflow(input)`，内部深拷贝当前 graph，替换 workflow metadata，清理 UI resource version 和 run state，然后调用 `createWorkflowFromGraph()`。

验收：

- [x] 克隆出来的新 workflow 与源 workflow 执行语义一致，但 name、description、version、labels 独立。

## 5. RunnerWorkbench UI 集成

### 5.1 新建弹窗接入

- [x] 在 `RunnerWorkbench.vue` 引入 `NewWorkflowModal.vue`。
- [x] 增加本地状态 `showNewWorkflowModal`。
- [x] 点击 `新建` 打开弹窗。
- [x] 弹窗提交时调用 `store.createWorkflowFromGraph()`。
- [x] 提交中按钮展示 loading，并禁止重复提交。
- [x] 错误展示复用现有错误区域或弹窗内错误提示。

验收：

- [x] 用户可以通过 `新建` 完成模板创建。
- [x] 创建过程中不会误触发保存当前 workflow。

### 5.2 工作流选择器接入

- [x] 使用 `state.workflowOptions` 渲染选择器。
- [x] 当前值绑定 `state.graph.workflow.name`。
- [x] 切换时调用 `store.switchWorkflow(name)`。
- [x] dirty 状态下弹出确认；确认后调用 `store.switchWorkflow(name, { force: true })`。
- [x] 切换 loading 时禁用保存、发布、dry-run、run 按钮。

验收：

- [x] 工作流切换后画布、属性面板、YAML 预览、运行抽屉状态全部对应新 workflow。

### 5.3 YAML 创建入口

- [x] 新建弹窗中 `from-yaml` 模式提供 YAML 输入框。
- [x] 点击解析时调用现有 `runnerApi.parseGraphYAML(yaml)`。
- [x] 解析成功后要求用户填写或确认新 name。
- [x] 如果 YAML 中已有 name 且与输入 name 不同，以输入 name 为准并更新 graph.workflow.name。
- [x] 解析失败展示 parse error type。
- [x] 提交时仍调用 `createWorkflowFromGraph()`，不调用旧 YAML create。

验收：

- [x] YAML 导入创建最终仍落在 graph-native create 链路。

## 6. 后端测试

### 6.1 Service 单测

- [x] 在 `pkg/runner/server/service/visual_workflow_service_test.go` 增加 `TestVisualWorkflowServiceCreateGraphPersistsDraft`。
- [x] 断言创建后 `workflowSvc.Get()` 能读到 RawYAML。
- [x] 断言返回 graph 带 `ui.resource_version`。
- [x] 断言返回 status 为 `draft`。
- [x] 增加 `TestVisualWorkflowServiceCreateGraphRejectsInvalidGraph`。
- [x] 增加 `TestVisualWorkflowServiceCreateGraphDetectsDuplicateName`。
- [x] 增加 `TestVisualWorkflowServiceCreateGraphPersistsLabelsAndSaveNote`。

验收命令：

```bash
go test ./pkg/runner/server/service -run 'TestVisualWorkflowServiceCreateGraph' -count=1
```

预期：

```text
ok  	runner/server/service
```

### 6.2 API Handler 单测

- [x] 在 `pkg/runner/server/api/visual_workflow_handler_test.go` 增加 `TestVisualWorkflowRouteCreateGraphWorkflow`。
- [x] 断言 `POST /api/v1/workflows/graph` 返回 `201`。
- [x] 断言响应包含 `name`、`status`、`graph`、`yaml`。
- [x] 增加空 body、空 graph、重复 name、非法 graph 用例。
- [x] 在 `visual_workflow_audit_test.go` 增加 create audit 覆盖。

验收命令：

```bash
go test ./pkg/runner/server/api -run 'TestVisualWorkflowRouteCreateGraphWorkflow|TestVisualWorkflowAuditCoversGraphLifecycle' -count=1
```

预期：

```text
ok  	runner/server/api
```

## 7. 前端测试

### 7.1 模板测试

- [x] 新增 `pkg/runner/server/ui/frontend/src/__tests__/workflowTemplates.test.ts`。
- [x] 覆盖三个模板的节点、边、workflow metadata。
- [x] 断言模板不包含 `state` 和 `resource_version`。
- [x] 断言默认模板 action args 正确。

验收命令：

```bash
npm test -- workflowTemplates.test.ts
```

执行目录：

```bash
cd pkg/runner/server/ui/frontend
```

### 7.2 Store 测试

- [x] 在 `graphStore.test.ts` 增加 create workflow 用例。
- [x] mock `POST /api/v1/workflows/graph` 和 `GET /api/v1/workflows?limit=200`。
- [x] 断言请求 body 包含 graph、labels、save_note。
- [x] 断言成功后 `baselineGraph`、`workflowOptions`、`dirty`、`workflowStatus`、`yamlPreview` 正确。
- [x] 增加创建失败不覆盖当前 graph 的用例。
- [x] 增加 dirty 切换 workflow 需要确认的用例。

验收命令：

```bash
npm test -- graphStore.test.ts
```

### 7.3 Workbench 组件测试

- [x] 在 `RunnerWorkbench.test.ts` 增加顶部工具栏包含工作流选择器和 `新建`。
- [x] 增加点击 `新建` 展示 `NewWorkflowModal`。
- [x] 增加创建提交会调用 store 的用例。
- [x] 增加 dirty 切换工作流展示确认的用例。

验收命令：

```bash
npm test -- RunnerWorkbench.test.ts
```

### 7.4 前端完整验证

- [x] 执行前端单测：

```bash
cd pkg/runner/server/ui/frontend
npm test
```

- [x] 执行前端 build：

```bash
cd pkg/runner/server/ui/frontend
npm run build
```

验收：

- [x] Vitest 全部通过。
- [x] `vue-tsc --noEmit` 通过。
- [x] Vite build 成功生成 runner UI dist。

## 8. 浏览器与端到端验证

- [x] 启动 runner server 或前端 dev server。
- [x] 使用浏览器打开 Runner 可视化工作台。
- [x] 点击 `新建`。
- [x] 使用 `cmd-run-basic` 创建 `visual-create-smoke`。
- [x] 创建后确认画布显示 Start -> cmd.run -> End。
- [x] 点击 `Validate`，确认通过。
- [x] 点击 `Dry run`，确认返回目标、路径和 YAML。
- [x] 修改 cmd.run 参数。
- [x] 勾选语义变更确认。
- [x] 点击 `Save draft`。
- [x] 点击 `Publish`。
- [x] 切换到另一个 workflow，再切回 `visual-create-smoke`。
- [x] 确认 graph、状态、YAML 预览一致。

验收：

- [x] 新建、编辑、保存、发布、切换形成闭环。
- [x] 浏览器控制台无未处理异常。
- [x] Network 中没有调用旧 YAML create 来创建可视化 workflow。

## 9. 文档与操作说明

- [x] 更新 `pkg/runner/VISUAL_WORKFLOW_UI_DESIGN.md`，补充“新增工作流”章节。
- [x] 更新 `pkg/runner/VISUAL_WORKFLOW_UI_TODO.md`，增加指向本文档的补充项。
- [x] 增加 API 示例：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/workflows/graph \
  -H 'Content-Type: application/json' \
  -d '{
    "graph": {
      "version": "v1",
      "workflow": {
        "version": "v0.1",
        "name": "visual-create-smoke",
        "description": "Created from visual workflow UI"
      },
      "layout": { "direction": "LR" },
      "nodes": [
        { "id": "start", "type": "start", "label": "Start", "position": { "x": 80, "y": 120 } },
        { "id": "run-command", "type": "action", "position": { "x": 320, "y": 120 }, "step": { "id": "run-command", "name": "run-command", "action": "cmd.run", "args": { "cmd": "echo hello" } } },
        { "id": "end", "type": "end", "label": "End", "position": { "x": 600, "y": 120 } }
      ],
      "edges": [
        { "id": "start-run-command", "source": "start", "target": "run-command", "kind": "next" },
        { "id": "run-command-end", "source": "run-command", "target": "end", "kind": "next" }
      ]
    },
    "labels": { "source": "visual-ui" },
    "save_note": "initial visual workflow draft"
  }'
```

验收：

- [x] 文档说明清楚 UI 新建、API 新建、YAML 兼容创建三者关系。

## 10. 发布门禁

- [x] 后端定向测试通过：

```bash
go test ./pkg/runner/server/service ./pkg/runner/server/api -count=1
```

- [x] runner 后端全量关键测试通过：

```bash
go test ./pkg/runner/... -count=1
```

- [x] 前端测试通过：

```bash
cd pkg/runner/server/ui/frontend
npm test
```

- [x] 前端构建通过：

```bash
cd pkg/runner/server/ui/frontend
npm run build
```

- [x] 浏览器 smoke test 通过。
- [x] `VISUAL_WORKFLOW_CREATE_TODO.md` 中已完成任务状态全部更新。
- [x] 没有新增 parallel YAML-only create UI。
- [x] 没有破坏旧 `POST /api/v1/workflows`。

最终验收：

- [x] 用户可以在可视化工作台中新建一个生产级 workflow。
- [x] 新 workflow 创建后立即进入同一套 graph 编辑、保存、发布、dry-run、运行机制。
- [x] 后端 audit、version、validation、compile、resource version 全部生效。
- [x] 创建链路只有一套：Graph-first create，YAML-compatible execution。
