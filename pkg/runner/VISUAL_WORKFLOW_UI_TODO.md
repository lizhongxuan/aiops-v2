# Runner 可视化编排生产级实施清单

本文档是 `VISUAL_WORKFLOW_UI_DESIGN.md` 的生产级实施清单。目标不是做临时原型，而是一次性交付可长期演进的可视化编排能力：图模型、DAG 语义、YAML 兼容、服务端契约、运行态可观测、权限审计、测试门禁和发布验证必须形成同一套机制。

执行要求：

- 每完成一个任务，必须把对应复选框从 `[ ]` 更新为 `[x]`。
- 不并行维护多套执行机制；旧 YAML 顺序执行要作为 graph 的兼容形态保留。
- 代码变更必须保持现有 runner API 和 YAML workflow 可用。
- 多 agent 并行时必须有明确文件边界，最终由主线统一集成和验证。

## 0. 范围与工程边界

- [x] 将实施范围从原 MVP 口径升级为生产级一次性交付。
- [x] 同步 `VISUAL_WORKFLOW_UI_DESIGN.md` 为生产级 graph/DAG 口径，移除 MVP/sequential-only 结论。
- [x] 确认当前 runner 后端已有 workflow/run/SSE/agent/script/env/mcp 管理能力。
- [x] 确认当前 runner 嵌入 UI 只有 `pkg/runner/server/ui/dist` 产物，缺少可维护源码目录。
- [x] 完成并行任务拆分：`workflow/visual/**`、`server/service/**`、`server/api/**`、前端源码接入互不重叠。
- [x] 建立生产级 UI 源码归属：优先新增 `pkg/runner/server/ui/frontend/`，或明确改造主 `web/` 并同步 runner dist。
- [x] 明确 graph 是唯一可视化领域模型，YAML 是执行兼容格式，不新增第二套 DSL。
- [x] 明确 graph DAG 是长期执行语义，legacy `steps[]` 是 DAG 的线性投影。
- [x] 明确所有 UI 元数据只写入 `x_runner_ui`，执行语义写入 `x_runner_graph` 或正式 graph 字段。

验收：

- [x] 任何新旧 workflow 都能被统一解析为 graph。
- [x] 不存在“画布语义”和“执行语义”互相分叉的实现。

## 1. 生产级 Graph 领域模型

### 1.1 Graph 包与类型

- [x] 新增 `pkg/runner/workflow/visual/`。
- [x] 定义 `Graph`、`Node`、`Edge`、`Port`、`Layout`、`Viewport`、`Issue`、`Overlay`。
- [x] 定义 node type：`start`、`action`、`condition`、`parallel`、`join`、`handler`、`manual_approval`、`subflow`、`end`、`group`。
- [x] 定义 edge kind：`next`、`success`、`failure`、`condition`、`always`、`approval_approved`、`approval_rejected`。
- [x] Node 必须支持稳定 `id`，Step 必须支持稳定 `step_id` 映射。
- [x] Graph 必须支持 `version`，当前为 `v1`。
- [x] Graph 必须携带 workflow metadata：`name/version/description/env_packages/validation_env/inventory/vars/plan/tests`。
- [x] Graph 必须支持扩展字段，但扩展字段不能影响执行校验。

验收：

- [x] graph model JSON/YAML round-trip 不丢字段。
- [x] graph model 可表达线性 workflow、条件分支、并行 join、错误分支、人工审批和子流程占位。

### 1.2 Graph 校验

- [x] 校验必须存在唯一 start。
- [x] 校验必须至少存在一个可执行 action/subflow/manual_approval/end 路径。
- [x] 校验 node id 唯一。
- [x] 校验 step name 非空且在执行节点中唯一。
- [x] 校验 action 节点必须有 `workflow.Step` 或可编译 action spec。
- [x] 校验 edge source/target/port 均存在。
- [x] 校验 DAG 无环，除非未来显式引入 bounded loop 节点。
- [x] 校验所有执行节点从 start 可达。
- [x] 校验所有非 terminal 执行节点至少有一条出边。
- [x] 校验 join 的入边数量和 join strategy 合法。
- [x] 校验 condition edge 必须有 condition 表达式或引用 condition node。
- [x] 校验 manual approval 节点必须有超时、审批主体和 reject 策略。
- [x] 校验 subflow 节点引用的 workflow 必须存在或可延迟解析。
- [x] 校验结果必须包含 `severity/code/node_id/edge_id/field/message/suggestion`。

验收：

- [x] 单测覆盖空图、重复节点、无 start、多 start、环路、孤儿节点、非法 join、非法审批、非法子流程。
- [x] 校验问题可直接定位到画布节点、边和属性字段。

### 1.3 YAML/Graph 双向解析

- [x] 支持无 `x_runner_ui` 的旧 YAML 自动解析为线性 graph。
- [x] 支持有 `x_runner_ui` 的 YAML 保留布局。
- [x] 支持有 `x_runner_graph` 或未来正式 graph 字段的 YAML 还原 DAG。
- [x] 使用 `yaml.v3` AST 保留未知顶层字段。
- [x] 自动生成稳定 node id 和 edge id。
- [x] 自动布局旧 YAML：LR 方向，长链路换行，handler/notify 放辅助泳道。
- [x] 解析 handlers、notify、when、loop、expect_vars、must_vars、continue_on_error。
- [x] 解析失败必须区分 YAML 语法错误、workflow 校验错误和 graph 校验错误。

验收：

- [x] 现有 `examples/*.yaml` 均可 parse 为 graph。
- [x] graph -> YAML -> graph round-trip 保留执行语义和布局。

### 1.4 Graph 编译

- [x] 编译 legacy sequential YAML，保证现有 engine 可执行。
- [x] 编译生产级 graph YAML，保留 DAG 执行语义。
- [x] 编译时写入 `x_runner_ui` 保存布局。
- [x] 编译时写入 `x_runner_graph` 保存 DAG 语义，避免丢失分支/并行。
- [x] 对可线性化 graph 生成 `steps[]` 兼容投影。
- [x] 对不可线性化 graph 不得伪装成 sequential，必须走 graph executor 或报错。
- [x] 编译后必须调用 workflow/graph 双重校验。

验收：

- [x] 线性 graph 可被旧 engine 执行。
- [x] DAG graph 不会被错误降级为顺序 YAML。

## 2. Graph 执行语义

### 2.1 Workflow 模型兼容扩展

- [x] 设计 `workflow.GraphSpec` 或兼容 `x_runner_graph` 的正式 Go 类型。
- [x] `workflow.Load` 保持旧 YAML 兼容。
- [x] `workflow.Validate` 增加 graph-aware 校验入口，不破坏旧校验。
- [x] 为 step 引入稳定 ID 的兼容方案，避免只靠 name 映射运行态。

验收：

- [x] 旧 YAML 的单测全部通过。
- [x] 新 graph YAML 能被 Load 并校验。

### 2.2 DAG Executor

- [x] 新增 graph executor，不替换旧 executor 的公共行为。
- [x] 支持拓扑调度。
- [x] 支持 condition edge。
- [x] 支持 parallel fork。
- [x] 支持 join 策略：all_success、any_success、always、failure_threshold。
- [x] 支持 failure edge。
- [x] 支持 always edge。
- [x] 支持 continue_on_error 与 failure edge 的优先级规则。
- [x] 支持 timeout/retries/loop/targets 与现有 step 行为一致。
- [x] 支持 manual approval 节点状态暂停和恢复。
- [x] 支持 subflow 节点调用已保存 workflow。
- [x] 支持取消时传播到所有 running 节点。

验收：

- [x] 单测覆盖顺序、分支、并行、join、失败分支、取消、超时、审批暂停/恢复。
- [x] 旧 `executor.Run` 行为不回归。

### 2.3 RunState 升级

- [x] 扩展 `state.RunState` schema，新增兼容的 `GraphRunState`、`NodeState`、`EdgeState` 字段。
- [x] 为 graph run state 增加深拷贝逻辑和单元测试。
- [x] 扩展 run state 支持 node/edge 状态，不破坏现有 `Steps []StepState`。
- [x] 增加 `GraphState`：node status、edge status、attempt、host results、timestamps。
- [x] 支持从 GraphState 合成旧 StepState。
- [x] 事件模型增加 node_started/node_finished/edge_selected/approval_waiting/approval_resolved。
- [x] SSE 兼容旧事件并发布新 graph 事件。

验收：

- [x] 旧 run detail API 可继续返回 steps。
- [x] 新 run graph API 可返回完整 graph overlay。

## 3. Action Catalog 与表单 Schema

- [x] 新增生产级 ActionCatalog。
- [x] 每个 action spec 包含 category/title/description/risk/schema/defaults/outputs/examples。
- [x] `cmd.run` schema 完整覆盖 cmd/dir/env/output 限制。
- [x] `shell.run` schema 完整覆盖 script/dir/env/export_vars/output 限制。
- [x] `script.shell` schema 支持 script_ref、args、env、输出变量。
- [x] `script.python` schema 支持 script_ref、args、env、输出变量。
- [x] `wait.event` 明确实现状态；未实现前不得作为生产可执行节点。
- [x] 支持自定义 module 注册 ActionSpec。
- [x] catalog API 支持版本和 capability 检测。

验收：

- [x] 前端节点库完全由 catalog 渲染，不硬编码 action 参数。
- [x] action schema 能生成属性表单和校验规则。

## 4. 服务层与 HTTP API

### 4.1 VisualWorkflowService

- [x] 新增 `VisualWorkflowService`。
- [x] 实现 `GetGraph`。
- [x] 实现 `SaveGraph`。
- [x] 实现 `CompileGraph`。
- [x] 实现 `ValidateGraph`。
- [x] 实现 `DryRunGraph`。
- [x] 实现 `SubmitGraphRun`。
- [x] 实现 `GetRunGraph`。
- [x] 实现 `ApproveNode` / `RejectNode`。
- [x] 实现 graph 版本冲突检测。

验收：

- [x] service 单测覆盖成功、校验失败、并发保存冲突、审批状态转换。

### 4.2 API Handler

- [x] 注册 `GET /api/v1/workflows/{name}/graph`。
- [x] 注册 `PUT /api/v1/workflows/{name}/graph`。
- [x] 注册 `POST /api/v1/workflows/graph/compile`。
- [x] 注册 `POST /api/v1/workflows/graph/validate`。
- [x] 注册 `POST /api/v1/workflows/graph/dry-run`。
- [x] 注册 `POST /api/v1/workflows/graph/runs`。
- [x] 注册 `GET /api/v1/actions/catalog`。
- [x] 注册 `GET /api/v1/runs/{id}/graph`。
- [x] 注册 `POST /api/v1/runs/{id}/nodes/{node_id}/approve`。
- [x] 注册 `POST /api/v1/runs/{id}/nodes/{node_id}/reject`。
- [x] 所有变更 API 必须接入 audit log。
- [x] API 错误格式必须与现有 handler 一致。

验收：

- [x] auth、CORS、base path、SSE 均兼容。
- [x] handler 单测覆盖路由、错误、审计。

## 5. 前端源码与工程化

- [x] 建立 runner UI 可维护源码目录。
- [x] 使用 React + Vite。
- [x] 引入轻量 React/SVG 画布。
- [x] 引入 Monaco YAML 编辑。
- [x] 建立 runner workflow API client。
- [x] 建立 graph store/composable。
- [x] 建立 run event reducer。
- [x] 建立端到端 mock fixture。
- [x] 构建产物进入 `pkg/runner/server/ui/dist` 或配置化 dist。

验收：

- [x] 前端 build/test 可重复运行。
- [x] dist 可被 runner server 正常服务。

## 6. 生产级编排页面

### 6.1 页面布局

- [x] 顶栏：workflow 名称、版本、draft/saved/running、YAML、校验、dry-run、试运行、发布。
- [x] 左侧节点库：按 catalog 渲染，支持搜索、分类、风险标识。
- [x] 中间画布：拖拽、连线、框选、缩放、自动布局、小地图。
- [x] 右侧属性面板：配置、高级、YAML diff、运行态。
- [x] 底部运行抽屉：状态、step-host、stdout/stderr、导出变量、SSE 事件。
- [x] 响应式布局支持桌面、平板和移动。

验收：

- [x] UI 对齐用户提供的草图信息结构。
- [x] 不出现文本重叠和布局跳动。

### 6.2 画布能力

- [x] Start/End 节点。
- [x] Action 节点。
- [x] Condition 节点。
- [x] Parallel/Join 节点。
- [x] Handler/Notify 节点。
- [x] Manual Approval 节点。
- [x] Subflow 节点。
- [x] Group 节点。
- [x] 节点复制、粘贴、删除、撤销、重做。
- [x] 自动布局支持 DAG。
- [x] 不合法连线实时阻止或标红。

验收：

- [x] 用户能构建非平凡 DAG 并通过服务端校验。

### 6.3 属性面板

- [x] 根据 action schema 渲染表单。
- [x] 支持 targets 选择、agent capability 校验。
- [x] 支持 vars/inventory 编辑。
- [x] 支持 condition 表达式编辑。
- [x] 支持 join 策略编辑。
- [x] 支持 manual approval 主体、超时、reject 策略编辑。
- [x] 支持 subflow 选择和入参映射。
- [x] 支持 raw YAML/JSON 高级编辑。
- [x] 支持字段级校验和修复建议。

验收：

- [x] 表单变更能实时更新 graph 和编译预览。

## 7. YAML、Diff 与版本治理

- [x] YAML 模式可查看完整执行 YAML。
- [x] YAML 模式可编辑并同步回 graph。
- [x] Diff 区分执行语义变更、布局变更、元数据变更。
- [x] 保存时做乐观锁/版本冲突检测。
- [x] 支持 draft/published 状态。
- [x] 支持保存备注。
- [x] 支持历史版本查看和回滚。
- [x] 支持导入导出 workflow bundle。

验收：

- [x] 用户能审查并确认执行语义变更。
- [x] 并发编辑不会静默覆盖。

## 8. Dry-run、仿真与发布门禁

- [x] Graph dry-run 展示节点覆盖范围、目标 host、action、变量引用、脚本引用。
- [x] 支持 agent capability 预检查。
- [x] 支持脚本引用存在性和语言匹配检查。
- [x] 支持变量未定义检查。
- [x] 支持危险 action 风险提示。
- [x] 支持 DAG 路径仿真。
- [x] 发布前必须通过校验、dry-run 和权限检查。

验收：

- [x] 有 error 时不能发布或试运行。
- [x] warning 可显式确认并记录审计。

## 9. 运行态与可观测性

- [x] SSE 实时更新 graph node/edge 状态。
- [x] 支持运行历史回放。
- [x] 支持失败节点自动聚焦。
- [x] 支持 step-host 结果表。
- [x] 支持 stdout/stderr 增量显示。
- [x] 支持导出变量面板。
- [x] 支持 runner_debug 展示。
- [x] 支持取消运行。
- [x] 支持审批节点操作。
- [x] 支持日志导出。
- [x] 支持 Prometheus 指标：graph run/node duration/failure count/approval wait。

验收：

- [x] 长任务运行中刷新页面可恢复状态。
- [x] 失败定位不依赖人工翻日志。

## 10. 权限、安全与审计

- [x] Graph 保存、发布、运行、取消、审批必须审计。
- [x] 高风险 action 需要权限或二次确认。
- [x] 支持 action 白名单。
- [x] 支持 agent capability 约束。
- [x] 支持敏感变量脱敏展示。
- [x] 支持脚本内容和 env 的安全扫描占位。
- [x] SSE 和 graph API 遵守现有 auth middleware。

验收：

- [x] 审计日志能回答谁在何时改了什么、运行了什么、批准了什么。

## 11. 测试矩阵

### 11.1 Go 测试

- [x] `workflow/visual` model/parse/compile/validate/overlay 单测。
- [x] graph executor 单测。
- [x] RunState graph 兼容单测。
- [x] service 单测。
- [x] API handler 单测。
- [x] 旧 workflow/executor/run_service 回归测试。

### 11.2 前端测试

- [x] graph store 单测。
- [x] run event reducer 单测。
- [x] property panel schema 渲染测试。
- [x] YAML sync 测试。
- [x] canvas interaction 测试。

### 11.3 E2E 与视觉 QA

- [x] Playwright 覆盖创建 DAG、保存、刷新、试运行、SSE、失败定位、审批。
- [x] 1440x900、1280x800、1024x768、390x844 截图 QA。
- [x] 检查节点文本不溢出。
- [x] 检查运行抽屉不遮挡主操作。

### 11.4 压测与可靠性

- [x] 100 节点 graph 编排性能测试。
- [x] 1000 事件 SSE 回放测试。
- [x] 并发运行和取消测试。
- [x] server 重启后 running graph 对账测试。

## 12. 文档与迁移

- [x] 更新 `pkg/runner/README.md`。
- [x] 编写 Graph YAML 规范。
- [x] 编写 API 文档。
- [x] 编写用户操作 runbook。
- [x] 编写开发者 ActionSpec 扩展指南。
- [x] 编写旧 workflow 自动迁移说明。
- [x] 编写生产发布检查表。
- [ ] 按 `pkg/runner/VISUAL_WORKFLOW_CREATE_TODO.md` 补齐可视化新增工作流生产级链路。

## 13. 发布门禁

- [x] `go test ./...` 在 `pkg/runner` 通过。
- [x] 前端 test/build 通过。
- [x] runner server 本地启动通过。
- [x] 嵌入 UI 或 dist_dir 服务通过。
- [x] Auth enabled 下新 API 通过。
- [x] CORS/base path 下 SSE 通过。
- [x] 旧 YAML workflow 执行不回归。
- [x] 新 DAG workflow 执行通过。
- [x] Playwright 关键路径通过。
- [x] 视觉 QA 通过。

## 14. 当前并行实施分工

- [x] 主线：负责 TODO 状态、最终集成、路由接线、验证与总结。
- [x] Worker A：负责 `pkg/runner/workflow/visual/**`。
- [x] Worker B：负责 `pkg/runner/server/service/action_catalog.go`、`visual_workflow_service.go` 及对应测试。
- [x] Worker C：只读梳理 `pkg/runner/server/api/**` 接入点和测试清单。

## 15. 完成定义

生产级交付只有在以下条件全部满足时才算完成：

- [x] 旧 sequential YAML 可自动转换为 graph，执行行为不变。
- [x] 新 DAG graph 可保存、校验、dry-run、执行、观察和回放。
- [x] 可视化 UI 支持生产级编辑、审查、发布、运行和故障定位。
- [x] Graph/YAML/RunState/SSE/API 只有一套一致语义。
- [x] 权限、审计、安全、测试、文档和发布门禁完整。
