# Runner 可视化编排生产级设计方案

## 1. 目标

Runner 可视化编排不是临时 YAML 编辑器，而是生产级工作流编排能力。最终形态应支持：

- 可视化 DAG 编排：顺序、条件、并行、join、失败分支、人工审批、子流程。
- YAML 兼容：现有 `steps[]` YAML 可以继续执行，并能自动转换为 graph。
- 单一语义：graph 是可视化和长期执行语义；旧 YAML 是兼容投影，不维护第二套 DSL。
- 运行态闭环：画布上实时展示 node/edge/host/log/vars/approval 状态。
- 生产治理：权限、审计、版本、发布门禁、可观测性、测试和迁移文档完整。

## 2. 当前基础

现有 runner 已具备以下能力：

- `workflow.Workflow`：旧 YAML 模型，核心是 `Steps []Step`。
- `engine.Engine` + `executor.Executor`：当前按 steps 顺序执行。
- `state.RunStateStore`：run 生命周期、step/host 状态持久化。
- `server/service.RunService`：队列、执行、取消、SSE 事件、历史记录。
- `WorkflowService`：workflow YAML 持久化。
- `Preprocessor`：script_ref、agent 地址、action 白名单处理。
- `agent/script/skill/mcp/env` 等管理 API。

生产级方案的重点不是替换这些能力，而是新增 graph 层，并让旧顺序执行成为 graph 的兼容子集。

## 3. 核心架构

```text
Visual UI
  |
  | Graph JSON / YAML
  v
VisualWorkflow API
  |
  +-- workflow/visual: parse / compile / validate / overlay
  +-- ActionCatalog: node palette + form schema
  +-- WorkflowService: graph YAML persistence
  +-- RunService: submit / cancel / history / SSE
  |
  v
Engine
  |
  +-- legacy executor: sequential steps compatibility
  +-- graph executor: DAG / branch / parallel / approval / subflow
```

## 4. 数据模型

### 4.1 YAML 字段

使用两个顶层扩展字段：

- `x_runner_ui`：布局、viewport、折叠状态、画布偏好，只影响 UI。
- `x_runner_graph`：生产级 DAG 执行语义，包含 nodes/edges/ports/join/approval/subflow 等。

旧 YAML 示例仍然有效：

```yaml
version: v0.1
name: demo
inventory:
  hosts:
    local:
      address: local
steps:
  - name: hello
    targets: [local]
    action: cmd.run
    args:
      cmd: echo hello
```

带 graph 的生产级 YAML 示例：

```yaml
version: v0.1
name: pg-restore
plan:
  strategy: graph

x_runner_ui:
  version: v1
  layout:
    direction: LR
    viewport: { x: 0, y: 0, zoom: 1 }

x_runner_graph:
  version: v1
  nodes:
    - id: start
      type: start
    - id: pre-check
      type: action
      step:
        name: pre-check
        action: shell.run
        targets: [pg-primary]
        args:
          script: df -h /data
        expect_vars: [disk_free]
    - id: approve
      type: manual_approval
      approval:
        subjects: [dba-oncall]
        timeout: 30m
        on_timeout: reject
    - id: restore
      type: action
      step:
        name: restore
        action: script.shell
        targets: [pg-primary, pg-standby]
        args:
          script_ref: restore_pg.sh
    - id: verify
      type: action
      step:
        name: verify
        action: cmd.run
        targets: [pg-primary]
        args:
          cmd: ./verify_restore.sh
    - id: end
      type: end
  edges:
    - id: start-pre-check
      source: start
      target: pre-check
      kind: next
    - id: pre-check-approve
      source: pre-check
      target: approve
      kind: success
    - id: approve-restore
      source: approve
      target: restore
      kind: approval_approved
    - id: restore-verify
      source: restore
      target: verify
      kind: success
    - id: verify-end
      source: verify
      target: end
      kind: success

steps:
  - name: pre-check
    targets: [pg-primary]
    action: shell.run
    args:
      script: df -h /data
    expect_vars: [disk_free]
  - name: restore
    targets: [pg-primary, pg-standby]
    action: script.shell
    args:
      script_ref: restore_pg.sh
  - name: verify
    targets: [pg-primary]
    action: cmd.run
    args:
      cmd: ./verify_restore.sh
```

`steps[]` 在 graph workflow 中作为兼容投影存在；不可线性化的 DAG 不得伪装成顺序语义执行。

### 4.2 Graph 类型

核心类型：

- `Graph`: version、workflow metadata、nodes、edges、layout、extensions。
- `Node`: id、type、label、step、condition、approval、subflow、join、ui。
- `Edge`: id、source、target、kind、condition、ui。
- `Issue`: severity、code、node_id、edge_id、field、message、suggestion。
- `Overlay`: run/node/edge/host 状态。

节点类型：

- `start`
- `action`
- `condition`
- `parallel`
- `join`
- `handler`
- `manual_approval`
- `subflow`
- `end`
- `group`

边类型：

- `next`
- `success`
- `failure`
- `condition`
- `always`
- `approval_approved`
- `approval_rejected`

## 5. 执行语义

### 5.1 兼容顺序执行

旧 workflow 无 graph 字段时：

1. `steps[]` 自动转换为 `start -> action... -> end` graph。
2. 可以继续使用旧 executor 执行。
3. run state 继续返回 `Steps []StepState`。

### 5.2 DAG 执行

graph workflow 使用 graph executor：

- 拓扑调度节点。
- action 节点复用现有 module/dispatcher/targets/retries/timeout/loop。
- condition edge 根据表达式选择路径。
- parallel 节点派发多个分支。
- join 节点支持 `all_success`、`any_success`、`always`、`failure_threshold`。
- failure edge 捕获失败分支。
- always edge 可用于清理和通知。
- manual_approval 节点进入 waiting 状态，支持 approve/reject/timeout。
- subflow 节点调用已保存 workflow，并继承 run trace。
- cancel 必须传播到所有 running 节点和远端 agent task。

## 6. 后端 API

新增 API：

```http
GET  /api/v1/workflows/{name}/graph
PUT  /api/v1/workflows/{name}/graph
POST /api/v1/workflows/graph/compile
POST /api/v1/workflows/graph/validate
POST /api/v1/workflows/graph/dry-run
POST /api/v1/workflows/graph/runs
GET  /api/v1/actions/catalog
GET  /api/v1/runs/{id}/graph
POST /api/v1/runs/{id}/nodes/{node_id}/approve
POST /api/v1/runs/{id}/nodes/{node_id}/reject
```

要求：

- 所有写操作走现有 auth middleware。
- graph save/run/approve/reject/cancel 必须 audit。
- 错误响应沿用现有 `writeServiceError` 规则。
- base path、CORS、SSE 与现有 router 保持一致。

## 7. Action Catalog

节点库和属性表单必须由后端 catalog 驱动：

```go
type ActionSpec struct {
    Action      string          `json:"action"`
    Title       string          `json:"title"`
    Category    string          `json:"category"`
    Description string          `json:"description"`
    Risk        string          `json:"risk"`
    ArgsSchema  json.RawMessage `json:"args_schema"`
    Outputs     []OutputSpec    `json:"outputs,omitempty"`
    Examples    []ActionExample `json:"examples,omitempty"`
}
```

首批内置：

- `cmd.run`
- `shell.run`
- `script.shell`
- `script.python`
- `wait.event`，只有实现后才能标记 production ready。

后续自定义 module 必须能注册自己的 ActionSpec。

## 8. 前端设计

页面结构与用户草图一致：

- 顶栏：workflow 名、版本、状态、YAML、校验、试运行、发布。
- 左侧：节点库，按 catalog 分组，支持搜索和风险提示。
- 中间：Vue Flow 画布，支持拖拽、连线、自动布局、缩放、小地图。
- 右侧：当前节点配置，包含配置、高级、YAML diff、运行态。
- 底部：运行抽屉，展示状态、step-host、stdout/stderr、导出变量、SSE 事件。

生产级交互：

- 支持撤销/重做、复制/粘贴、框选、删除、自动命名。
- 支持 YAML 模式双向同步。
- 支持 graph diff，区分执行语义变更和布局变更。
- 支持版本冲突提示和保存备注。
- 支持审批节点操作。

### 8.1 新增工作流

新增工作流必须是 graph-first 链路，不要求用户先手写 YAML：

- 顶栏提供 workflow 选择器，展示 `name/version/status`。
- 顶栏提供 `New` 和 `Clone` 入口。
- `New` 弹窗支持模板创建：`cmd-run-basic`、`shell-run-basic`、`manual-approval-basic`。
- `New` 弹窗支持 YAML 粘贴创建：先调用 `POST /api/v1/workflows/graph/parse` 转为 graph，再按用户输入 name 覆盖 metadata。
- `Clone` 复制当前 graph 的执行语义和布局，但必须清理 run state、node/edge state、`ui.resource_version`，并写入新的 workflow metadata。
- 创建成功后进入同一个 draft 编辑态，后续保存、发布、dry-run、运行仍使用现有 graph 机制。
- dirty 状态下切换 workflow 必须确认，避免静默丢失变更。

生产级创建 API：

```http
POST /api/v1/workflows/graph
```

请求：

```json
{
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
}
```

响应：

```json
{
  "name": "visual-create-smoke",
  "status": "draft",
  "workflow": { "version": "v0.1", "name": "visual-create-smoke" },
  "graph": {},
  "yaml": "version: v0.1\nname: visual-create-smoke\n",
  "warnings": []
}
```

旧 `POST /api/v1/workflows` 继续作为 YAML 兼容 API 保留；可视化 UI 不通过它创建新 workflow。

## 9. 运行态可视化

运行态叠加到 graph：

- run status：queued/running/success/failed/canceled/interrupted/waiting_approval。
- node status：pending/running/success/failed/skipped/canceled/waiting_approval.
- edge status：selected/skipped/blocked。
- host status：每个 action node 下展示 host 级结果。
- log：stdout/stderr 增量显示，支持导出。
- vars：展示导出变量，敏感变量脱敏。
- failure focus：失败节点自动聚焦。
- history replay：支持历史 run 回放。

## 10. 权限、安全与审计

- 保存、发布、运行、取消、审批必须审计。
- 高风险 action 需要权限或确认。
- action 白名单和 agent capability 必须在 dry-run 阶段检查。
- 敏感变量展示脱敏。
- script/env 安全扫描预留接口。
- SSE 和 graph API 复用现有认证。

## 11. 测试与发布门禁

必须覆盖：

- Go 单测：visual parse/compile/validate/overlay、graph executor、RunState 兼容、service、api。
- 前端单测：graph store、event reducer、schema form、YAML sync。
- Playwright：创建 DAG、保存、刷新、试运行、SSE、失败定位、审批。
- 视觉 QA：1440x900、1280x800、1024x768、390x844。
- 压测：100 节点 graph、1000 SSE 事件、并发运行和取消。
- 回归：旧 YAML workflow 执行不变。

发布前必须通过：

- `go test ./...`
- 前端 test/build
- runner server 本地启动
- auth enabled API 测试
- CORS/base path SSE 测试
- 旧 YAML 和新 DAG 工作流执行测试

## 12. 推荐落地顺序

1. `workflow/visual` graph model、parse、compile、validate、overlay。
2. ActionCatalog。
3. VisualWorkflowService 和 API handler。
4. workflow graph 扩展与 graph executor。
5. RunState graph overlay 和 SSE graph events。
6. runner UI 前端源码目录和 API client。
7. 画布、属性面板、YAML/diff、运行抽屉。
8. 权限审计、发布门禁、E2E、视觉 QA、文档。

## 参考

- Dify workflow 文档：<https://docs.dify.ai/en/guides/workflow/node/start>
- Dify Output node 文档：<https://docs.dify.ai/en/use-dify/nodes/output>
- Dify Variable Aggregator 文档：<https://docs.dify.ai/en/use-dify/nodes/variable-aggregator>
- Vue Flow 官方文档：<https://vueflow.dev/>
- Vue Flow Drag & Drop 示例：<https://vue-flow-docs.netlify.app/examples/dnd>
