# Runner Studio 实施工作清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to execute one task at a time. Every completed checkbox must be updated in this file before moving to the next task.

**Goal:** 将 `/runner` 落地为主应用原生一体化的 Runner Studio，提供简洁的 Dify 风格工作流编排体验、结构化参数输入输出、AI 辅助、发布治理和运行闭环。

**Architecture:** 后端 graph/schema/变量作用域/API 契约先行，主应用前端只调用同源 `/api/runner-studio/*`。前端组件基于服务端契约渲染，不维护私有编排 DSL；旧 `pkg/runner/server/ui/frontend` 只作为迁移参考，不再作为主入口。

**Tech Stack:** Go runner graph/service/API、React、现有 `web/`、shadcn/ui、React/SVG workflow canvas、Monaco YAML、现有 LLM 配置。

---

## 执行规则

- [x] 每完成一个任务，必须将对应复选框改为 `[x]`。
- [x] 每个任务先写测试，再实现，再运行指定验证命令。
- [x] 多 agent 并行时只允许按任务边界拆分，不能两个 agent 同时改同一个文件。
- [x] 前端不得直接调用外部 `:8090` Runner UI。
- [x] 前端不得维护私有 graph/input/output schema。
- [x] 文档、测试、页面状态必须一致，不允许标记完成但真实浏览器不可用。

## 0. Review 修正闭环

**Files:**
- Modify: `pkg/runner/RUNNER_STUDIO_DESIGN.md`
- Modify: `pkg/runner/RUNNER_STUDIO_TODO.md`

- [x] 确认结构化 I/O 后端契约排在前端 I/O 组件之前。
- [x] 确认 `loop` 首版有完整后端、执行、run state、YAML 映射任务。
- [x] 确认实施清单不再是大模块清单，每个任务都有文件边界、测试和验证命令。
- [x] 确认主应用 API 只使用 `/api/runner-studio/*`。
- [x] 确认发布状态机绑定 `graph_hash`。
- [x] 确认旧 Runner UI 生命周期有明确处理任务。

验证：

```bash
rg -n "T[B]D|待[定]|或迁移[后]|Modify [o]r migrate|Replace [o]r rename" pkg/runner/RUNNER_STUDIO_DESIGN.md pkg/runner/RUNNER_STUDIO_TODO.md
```

期望：除说明历史问题的正文外，不出现模糊占位和双入口实施路径。

## 1. 主应用 API 边界

**Files:**
- Modify: `cmd/ai-server/main.go`
- Modify: `cmd/ai-server/main_test.go`
- Create: `internal/server/runner_studio_api.go`
- Create: `internal/server/runner_studio_api_test.go`
- Modify: `internal/server/http.go`
- Create: `web/src/api/runnerStudioClient.js`
- Create: `web/src/api/runnerStudioClient.test.js`
- Modify: `web/src/api/runnerUi.js`
- Create: `web/src/pages/RunnerStudioPage.tsx`
- Create: `web/src/pages/RunnerStudioPage.test.js`
- Modify: `web/src/router.js`

- [x] 写失败测试：`runner_studio_api_test.go` 覆盖 `/api/runner-studio/actions/catalog`、`/api/runner-studio/workflows`、`/api/runner-studio/workflows/{name}/graph` 的路由注册。
- [x] 实现 `internal/server/runner_studio_api.go`，统一挂载 `/api/runner-studio/*`。
- [x] 在 `internal/server/http.go` 注册 Runner Studio API。
- [x] 创建 `runnerStudioClient.js`，只允许调用 `/api/runner-studio/*`。
- [x] 将 `runnerUi.js` 收敛为 legacy 兼容文件，不再作为 `/runner` 主页面依赖。
- [x] 在 `cmd/ai-server` 接入 `AIOPS_RUNNER_STUDIO_UPSTREAM_URL` 等环境变量，生产启动时可配置同源聚合上游。
- [x] 将 `/runner` 路由切到主应用内 `RunnerStudioPage`，避免继续展示外部 Runner UI 跳板。

验证：

```bash
go test ./internal/server -run 'TestRunnerStudio' -count=1
go test ./cmd/ai-server -run 'TestRunnerStudioUpstreamFromEnv' -count=1
cd web && npm test -- runnerStudioClient.test.js
```

期望：主应用同源 API 可测试，前端 client 不包含 `:8090` 或外部 Runner UI 地址。

## 2. Graph 结构化 I/O 契约

**Files:**
- Modify: `pkg/runner/workflow/model.go`
- Modify: `pkg/runner/workflow/visual/model.go`
- Modify: `pkg/runner/workflow/visual/validate.go`
- Modify: `pkg/runner/workflow/visual/compile.go`
- Modify: `pkg/runner/workflow/visual/parse.go`
- Modify: `pkg/runner/workflow/visual/visual_test.go`
- Create: `pkg/runner/workflow/visual/io_test.go`

- [x] 写失败测试：graph node 可 round-trip `inputs`、`outputs`、`value_source`、`extract_source`。
- [x] 在 `model.go` 增加 `InputParamSpec`、`OutputParamSpec`、`ValueSource`、`ExtractSource`、`VariableRef`。
- [x] 在 `validate.go` 校验输入 key、输出 key、类型、值来源、提取来源和重复名。
- [x] 在 `compile.go` 保证 Graph -> YAML 不丢结构化 I/O。
- [x] 在 `parse.go` 保证 YAML -> Graph 能还原结构化 I/O。

验证：

```bash
cd pkg/runner && go test ./workflow/visual -run 'Test.*IO|Test.*RoundTrip' -count=1
```

期望：结构化 I/O 是后端 graph 契约的一部分，不是前端私有状态。

## 3. 变量作用域解析

**Files:**
- Create: `pkg/runner/workflow/visual/variables.go`
- Create: `pkg/runner/workflow/visual/variables_test.go`
- Modify: `pkg/runner/workflow/visual/validate.go`
- Modify: `pkg/runner/server/api/visual_workflow_handler.go`
- Modify: `pkg/runner/server/api/visual_workflow_handler_test.go`
- Modify: `internal/server/runner_studio_api.go`
- Modify: `internal/server/runner_studio_api_test.go`
- Modify: `web/src/api/runnerStudioClient.js`
- Modify: `web/src/api/runnerStudioClient.test.js`

- [x] 写失败测试：下游节点只能引用拓扑上游输出。
- [x] 写失败测试：分支变量经过 `join` 后只有显式 join output 可见。
- [x] 写失败测试：loop scope 内变量默认不泄漏到 loop 外。
- [x] 实现变量作用域解析器，返回 system、workflow_input、workflow_var、inventory、node_output、approval、subflow 变量。
- [x] 增加服务端 API：`POST /api/v1/workflows/graph/variables/resolve`。
- [x] 在主应用 API 中映射为 `POST /api/runner-studio/workflows/graph/variables/resolve`。

验证：

```bash
cd pkg/runner && go test ./workflow/visual -run 'Test.*Variable' -count=1
cd pkg/runner && go test ./server/api -run 'TestVisualWorkflow.*Variable' -count=1
```

期望：变量 picker 只消费服务端作用域结果。

## 4. Action Catalog 输入输出 Schema

**Files:**
- Modify: `pkg/runner/server/service/action_catalog.go`
- Modify: `pkg/runner/server/service/action_catalog_test.go`
- Modify: `pkg/runner/server/api/visual_workflow_handler.go`
- Modify: `pkg/runner/server/api/visual_workflow_handler_test.go`

- [x] 写失败测试：`cmd.run`、`shell.run`、`script.shell` 返回结构化 input schema 和 output schema。
- [x] 扩展 `ActionSpec`，明确 `inputs_schema`、`outputs_schema`、`input_examples`、`output_examples`。
- [x] 保留现有 `args_schema` 兼容字段，避免旧 UI 和已有 API 断裂。
- [x] 在 catalog API capabilities 中声明 `structured_io_schema=true`。

验证：

```bash
cd pkg/runner && go test ./server/service -run 'TestActionCatalog' -count=1
cd pkg/runner && go test ./server/api -run 'TestVisualWorkflowActionCatalog' -count=1
```

期望：前端节点配置表单完全由 catalog schema 驱动。

## 5. Loop Graph 与执行语义

**Files:**
- Modify: `pkg/runner/workflow/visual/model.go`
- Modify: `pkg/runner/workflow/visual/validate.go`
- Modify: `pkg/runner/workflow/visual/compile.go`
- Modify: `pkg/runner/workflow/visual/parse.go`
- Modify: `pkg/runner/executor/graph_executor.go`
- Modify: `pkg/runner/executor/graph_executor_test.go`
- Modify: `pkg/runner/state/model.go`
- Modify: `pkg/runner/state/runstate.go`
- Modify: `pkg/runner/state/runstate_graph_test.go`
- Modify: `pkg/runner/engine/run_tracker.go`
- Modify: `pkg/runner/engine/recorder.go`
- Modify: `pkg/runner/engine/engine_runstate_test.go`

- [x] 写失败测试：graph 支持 `loop` 节点 round-trip。
- [x] 写失败测试：loop 必须配置 `mode` 和 `max_iterations`。
- [x] 写失败测试：loop 执行记录每次 iteration 的 node state。
- [x] 增加 `NodeTypeLoop` 和 `LoopSpec`。
- [x] 实现 `for_each` 与 `while_condition` 的受控执行语义。
- [x] run state 增加 iteration 维度，取消 run 时停止当前和后续 iteration。

验证：

```bash
cd pkg/runner && go test ./workflow/visual -run 'Test.*Loop' -count=1
cd pkg/runner && go test ./executor -run 'TestGraphExecutor.*Loop|TestGraphExecutor.*Cancel' -count=1
```

期望：`loop` 作为首版节点时不是 UI 占位，而是完整可执行语义。

## 6. 发布状态机与 Graph Hash

**Files:**
- Modify: `pkg/runner/server/service/types.go`
- Modify: `pkg/runner/server/service/workflow_service.go`
- Modify: `pkg/runner/server/service/workflow_service_test.go`
- Modify: `pkg/runner/server/service/visual_workflow_service.go`
- Modify: `pkg/runner/server/service/visual_workflow_service_test.go`
- Modify: `pkg/runner/server/api/workflow_handler.go`
- Modify: `pkg/runner/server/api/workflow_handler_test.go`
- Modify: `pkg/runner/server/api/visual_workflow_audit_test.go`

- [x] 写失败测试：校验通过后持久化 `validated_graph_hash`、`validated_at`、`validated_by`。
- [x] 写失败测试：执行语义变化会让 validated 失效。
- [x] 写失败测试：布局变化不让 validated 失效。
- [x] 写失败测试：发布只能发布当前 `validated_graph_hash`。
- [x] 写失败测试：AI generated draft 不能绕过发布审阅。
- [x] 实现 graph hash 计算，区分执行语义 hash 和 UI layout hash。
- [x] 实现 `POST /api/v1/workflows/{name}/publish`，写入审计。

验证：

```bash
cd pkg/runner && go test ./server/service -run 'TestVisualWorkflow.*Publish|TestVisualWorkflow.*GraphHash' -count=1
cd pkg/runner && go test ./server/api -run 'TestVisualWorkflowAudit.*Publish' -count=1
```

期望：`draft -> validated -> published` 是可执行服务端状态机。

## 7. 旧 Runner UI 生命周期

**Files:**
- Modify: `pkg/runner/server/ui/frontend/README.md`
- Modify: `pkg/runner/server/ui/frontend/src/api/client.ts`
- Modify: `web/src/api/runnerUi.js`
- Modify: `web/src/api/runnerStudioClient.test.js`
- Modify: `web/src/pages/RunnerStudioPage.test.js`
- Modify: `web/src/pages/RunnerUiEntryPage.tsx`
- Modify: `web/src/pages/RunnerUiEntryPage.test.js`
- Modify: `web/src/router.js`

- [x] 标记 `pkg/runner/server/ui/frontend` 为 legacy/debug UI，不再作为主应用产品入口。
- [x] 梳理可迁移代码清单：workflow templates、graph utils、mock fixtures、canvas interaction。
- [x] `/runner` 路由切到 `RunnerStudioPage` 后，旧 `RunnerUiEntryPage` 测试改为 legacy 删除或不再路由引用。
- [x] 确认主导航不再出现外部 Runner UI 跳板文案。

验证：

```bash
cd web && npm test -- RunnerUiEntryPage.test.js App.route-session.test.js RunnerStudioPage.test.js runnerStudioClient.test.js
! rg -n ":8090|打开 Runner UI|启动 Runner UI" web/src
```

期望：主应用没有外部 Runner UI 主流程入口；legacy 只保留调试说明。

## 8. Runner Studio Shell

**Files:**
- Modify: `web/src/pages/RunnerStudioPage.tsx`
- Modify: `web/src/pages/RunnerStudioPage.test.js`
- Modify: `web/src/router.js`
- Modify: `web/src/App.tsx`
- Create: `web/src/components/runner/RunnerStudioShell.tsx`
- Create: `web/src/components/runner/RunnerStudioShell.test.js`
- Create: `web/src/components/runner/runnerStudio.css`

- [x] 写失败测试：访问 `/runner` 渲染 Runner Studio shell，而不是入口跳板。
- [x] 建立顶栏、左侧栏、画布区、底部抽屉挂载点。
- [x] 顶栏只展示工作流名称、状态、保存、校验、Dry Run、运行、发布、AI 生成。
- [x] 后端不可用时显示同页错误态，不跳转外部 URL。
- [x] 响应式布局覆盖桌面和平板宽度。

验证：

```bash
cd web && npm test -- RunnerStudioPage.test.js RunnerStudioShell.test.js App.route-session.test.js
```

期望：`/runner` 已是主应用内原生页面。

## 9. 工作流选择与管理

**Files:**
- Create: `web/src/components/runner/WorkflowQuickList.tsx`
- Create: `web/src/components/runner/WorkflowQuickList.test.js`
- Create: `web/src/components/runner/WorkflowManagerModal.tsx`
- Create: `web/src/components/runner/WorkflowManagerModal.test.js`
- Create: `web/src/components/runner/workflowManagerState.js`
- Create: `web/src/components/runner/workflowManagerState.test.js`
- Modify: `web/src/pages/RunnerStudioPage.tsx`
- Modify: `web/src/pages/RunnerStudioPage.test.js`
- Modify: `web/src/components/runner/RunnerStudioShell.tsx`
- Modify: `web/src/components/runner/RunnerStudioShell.test.js`
- Modify: `web/src/components/runner/runnerStudio.css`

- [x] 写失败测试：左侧只显示最近使用和收藏工作流。
- [x] 写失败测试：完整列表、搜索、筛选、归档、克隆、历史版本只在管理弹窗中出现。
- [x] 实现新建入口：空白、模板、YAML 导入、克隆、AI 生成。
- [x] 切换工作流时处理 dirty confirm。
- [x] 最近使用排序和收藏状态写入本地 UI state，不影响 graph 语义。

验证：

```bash
cd web && npm test -- WorkflowQuickList.test.js WorkflowManagerModal.test.js workflowManagerState.test.js
```

期望：主界面不被完整工作流列表淹没。

## 10. 画布与节点动作

**Files:**
- Create: `web/src/components/runner/RunnerCanvas.tsx`
- Create: `web/src/components/runner/RunnerCanvas.test.js`
- Create: `web/src/components/runner/CanvasToolbar.tsx`
- Create: `web/src/components/runner/CanvasToolbar.test.js`
- Create: `web/src/components/runner/NodeActionMenu.tsx`
- Create: `web/src/components/runner/NodeActionMenu.test.js`
- Create: `web/src/components/runner/canvasGraphAdapter.js`
- Create: `web/src/components/runner/canvasGraphAdapter.test.js`
- Modify: `web/src/components/runner/RunnerStudioShell.tsx`
- Modify: `web/src/components/runner/RunnerStudioShell.test.js`
- Modify: `web/src/components/runner/runnerStudio.css`

- [x] 写失败测试：catalog action 可拖入画布生成 graph node。
- [x] 写失败测试：节点连线生成 graph edge。
- [x] 写失败测试：单击节点只更新 summary selection。
- [x] 写失败测试：双击节点触发 config modal open event。
- [x] 写失败测试：右键菜单包含复制、删除、禁用、单节点试跑、最近运行、AI 修复。
- [x] 实现 canvas graph adapter，隔离画布节点格式和后端 graph 格式。

验证：

```bash
cd web && npm test -- RunnerCanvas.test.js CanvasToolbar.test.js NodeActionMenu.test.js canvasGraphAdapter.test.js
```

期望：画布交互可测试，UI graph 格式不会污染后端 graph 契约。

## 11. 节点配置弹窗

**Files:**
- Create: `web/src/components/runner/NodeConfigModal.tsx`
- Create: `web/src/components/runner/NodeConfigModal.test.js`
- Create: `web/src/components/runner/node-config/BasicTab.tsx`
- Create: `web/src/components/runner/node-config/BasicTab.test.js`
- Create: `web/src/components/runner/node-config/InputTab.tsx`
- Create: `web/src/components/runner/node-config/OutputTab.tsx`
- Create: `web/src/components/runner/node-config/AdvancedTab.tsx`
- Create: `web/src/components/runner/node-config/RunAiTab.tsx`
- Modify: `web/src/components/runner/RunnerStudioShell.tsx`
- Modify: `web/src/components/runner/runnerStudio.css`

- [x] 写失败测试：双击 action 节点打开五页签弹窗。
- [x] 基础页签编辑 `name`、`action`、`targets`、`retries`、`timeout`。
- [x] 输入页签挂载结构化输入编辑器。
- [x] 输出页签挂载结构化输出编辑器。
- [x] 高级页签默认折叠 `when`、`continue_on_error`、`env`、`workdir`、`failure edge`、`join/loop/subflow`。
- [x] 运行与 AI 页签展示最近试跑、校验问题、AI diff。

验证：

```bash
cd web && npm test -- NodeConfigModal.test.js BasicTab.test.js
```

期望：节点配置不依赖常驻右栏。

## 12. 结构化输入参数编辑器

**Files:**
- Create: `web/src/components/runner/io/ioTypes.js`
- Create: `web/src/components/runner/io/ioTypes.test.js`
- Create: `web/src/components/runner/io/InputParamList.tsx`
- Create: `web/src/components/runner/io/InputParamList.test.js`
- Create: `web/src/components/runner/io/InputParamRow.tsx`
- Create: `web/src/components/runner/io/ValueSourceSwitch.tsx`
- Create: `web/src/components/runner/io/VariableReferencePicker.tsx`
- Create: `web/src/components/runner/io/VariableReferencePicker.test.js`
- Create: `web/src/components/runner/io/MixedVariableTextInput.tsx`
- Create: `web/src/components/runner/io/MixedVariableTextInput.test.js`
- Modify: `web/src/components/runner/node-config/InputTab.tsx`
- Modify: `web/src/components/runner/NodeConfigModal.tsx`
- Modify: `web/src/components/runner/runnerStudio.css`

- [x] 写失败测试：输入参数行支持 `key`、`label`、`type`、`value_source`、`value`、`required`、`description`。
- [x] 写失败测试：`value_source` 只允许 `constant`、`variable_reference`、`expression`。
- [x] 写失败测试：变量引用必须来自服务端变量作用域结果。
- [x] 实现排序、复制、删除、重复名校验。
- [x] 混合变量文本显示变量高亮。

验证：

```bash
cd web && npm test -- ioTypes.test.js InputParamList.test.js VariableReferencePicker.test.js MixedVariableTextInput.test.js
```

期望：配置 `cmd.run` 或 `shell.run` 常见参数时不需要直接编辑 YAML。

## 13. 结构化输出参数编辑器

**Files:**
- Create: `web/src/components/runner/io/OutputParamList.tsx`
- Create: `web/src/components/runner/io/OutputParamList.test.js`
- Create: `web/src/components/runner/io/OutputParamRow.tsx`
- Create: `web/src/components/runner/io/ExtractSourceSelect.tsx`
- Create: `web/src/components/runner/io/JsonPathEditor.tsx`
- Create: `web/src/components/runner/io/JsonPathEditor.test.js`
- Create: `web/src/components/runner/io/outputTypes.js`
- Modify: `web/src/components/runner/node-config/OutputTab.tsx`
- Modify: `web/src/components/runner/NodeConfigModal.tsx`
- Modify: `web/src/components/runner/runnerStudio.css`

- [x] 写失败测试：输出变量行支持 `key`、`type`、`extract_source`、`extract_rule`、`description`。
- [x] 写失败测试：`extract_source` 只允许 `stdout_text`、`stdout_jsonpath`、`stderr_text`、`exit_code`、`export_var`、`approval_result`、`subflow_output`。
- [x] 写失败测试：JSONPath 提取规则非法时字段级报错。
- [x] 输出变量写回 graph node outputs。
- [x] 输出变量可被下游变量 picker 使用。

验证：

```bash
cd web && npm test -- OutputParamList.test.js JsonPathEditor.test.js
```

期望：节点输出显式可见、可校验、可被下游引用。

## 14. YAML 与 Diff 弹窗

**Files:**
- Create: `web/src/components/runner/YamlDiffModal.tsx`
- Create: `web/src/components/runner/YamlDiffModal.test.js`
- Create: `web/src/components/runner/YamlEditorPane.tsx`
- Create: `web/src/components/runner/GraphDiffSummary.tsx`
- Create: `web/src/components/runner/GraphDiffSummary.test.js`

- [x] 写失败测试：YAML 弹窗不会常驻主页面。
- [x] 写失败测试：Graph -> YAML 和 YAML -> Graph 调用同源 Runner Studio API。
- [x] 展示执行语义 diff 和 UI layout diff。
- [x] 不可线性化 graph 必须显示语义冲突，不允许生成顺序假象。

验证：

```bash
cd web && npm test -- YamlDiffModal.test.js GraphDiffSummary.test.js
```

期望：YAML 是兼容视图，不打断普通用户主流程。

## 15. 运行抽屉与变量检查

**Files:**
- Create: `web/src/components/runner/runStateReducer.js`
- Create: `web/src/components/runner/runStateReducer.test.js`
- Create: `web/src/components/runner/RunLogDrawer.tsx`
- Create: `web/src/components/runner/RunLogDrawer.test.js`
- Create: `web/src/components/runner/VariableInspectDrawer.tsx`
- Create: `web/src/components/runner/VariableInspectDrawer.test.js`
- Create: `web/src/components/runner/NodeRunDetailModal.tsx`

- [x] 写失败测试：SSE run event 可归并成 node/edge/host/log state。
- [x] 日志抽屉显示 stdout、stderr、SSE、审批事件、重试轨迹。
- [x] 变量检查抽屉显示输入变量、输出变量、运行态导出变量、最近节点结果。
- [x] 单击节点显示最近运行摘要。
- [x] 节点详情弹窗显示单节点完整结果。

验证：

```bash
cd web && npm test -- runStateReducer.test.js RunLogDrawer.test.js VariableInspectDrawer.test.js
```

期望：运行态信息不和画布争抢主视野。

## 16. AI 助手

**Files:**
- Create: `web/src/components/runner/ai/aiRunnerApi.js`
- Create: `web/src/components/runner/ai/aiRunnerApi.test.js`
- Create: `web/src/components/runner/ai/RunnerAiAssistantModal.tsx`
- Create: `web/src/components/runner/ai/RunnerAiAssistantModal.test.js`
- Create: `web/src/components/runner/ai/AiDiffPreview.tsx`
- Create: `internal/server/runner_studio_ai.go`
- Create: `internal/server/runner_studio_ai_test.go`

- [x] 写失败测试：AI 生成 workflow 返回结构化 `graph_patch` 和 `diff_summary`。
- [x] 写失败测试：AI patch node 只能写入 draft。
- [x] 写失败测试：AI 失败解释不修改 graph。
- [x] 使用现有 LLM 配置，不在前端硬编码 URL、apikey、model。
- [x] AI 结果应用前展示 diff，应用后调用 graph validate。

验证：

```bash
go test ./internal/server -run 'TestRunnerStudioAI' -count=1
cd web && npm test -- aiRunnerApi.test.js RunnerAiAssistantModal.test.js
```

期望：AI 产物可审计、可校验、不可绕过发布门禁。

## 17. 发布审阅与审计

**Files:**
- Create: `web/src/components/runner/PublishReviewModal.tsx`
- Create: `web/src/components/runner/PublishReviewModal.test.js`
- Modify: `pkg/runner/server/api/visual_workflow_audit_test.go`
- Modify: `internal/server/runner_studio_api.go`
- Modify: `internal/server/runner_studio_api_test.go`

- [x] 写失败测试：发布弹窗展示 diff、风险摘要、校验结果、发布说明。
- [x] 写失败测试：没有当前 `validated_graph_hash` 时发布按钮不可用。
- [x] 写失败测试：AI draft 未人工确认时不可发布。
- [x] 发布 API 写审计：actor、time、action、graph_hash、diff。
- [x] 发布成功后刷新 workflow 状态为 `published`。

验证：

```bash
go test ./internal/server -run 'TestRunnerStudio.*Publish' -count=1
cd pkg/runner && go test ./server/api -run 'TestVisualWorkflowAudit.*Publish' -count=1
cd web && npm test -- PublishReviewModal.test.js
```

期望：发布与审计是一套机制。

## 18. 浏览器 E2E 与真实流程

**Files:**
- Create: `web/tests/runner-studio.spec.js`
- Modify: `web/tests/App.navigation.erp-sre.spec.js`
- Modify: `web/tests/router.erp-sre.spec.js`

- [x] 写 Playwright 用例：打开 `/runner`，看到 Runner Studio 而不是入口跳板。
- [x] 用例覆盖：新建空白 workflow -> 拖入 3 个节点 -> 配输入输出 -> 校验 -> Dry Run。
- [x] 用例覆盖：AI 生成 workflow draft -> 查看 diff -> 应用 -> 校验失败时不覆盖当前 graph。
- [x] 用例覆盖：发布审阅弹窗要求 graph hash 和发布说明。

验证：

```bash
cd web && npm run test:ui -- tests/runner-studio.spec.js --project=chromium
```

期望：真实浏览器主流程可跑通。

## 19. 全量验证

**Files:**
- Modify: `pkg/runner/RUNNER_STUDIO_TODO.md`

- [x] 标记所有已完成任务状态。
- [x] 跑 Go 后端验证。
- [x] 跑前端单测。
- [x] 跑前端构建。
- [x] 跑 Playwright 主流程。

验证：

```bash
cd pkg/runner && go test ./... -count=1
go test ./internal/server -count=1
cd web && npm test
cd web && npm run build
cd web && npm run test:ui -- tests/runner-studio.spec.js --project=chromium
```

期望：所有验证通过，`/runner` 主应用原生 Runner Studio 可用，文档状态与实际实现一致。

## 20. 2026-05-05 浏览器批注修复

**Files:**
- Modify: `web/src/pages/RunnerStudioPage.tsx`
- Modify: `web/src/pages/RunnerStudioPage.test.js`
- Modify: `web/src/components/runner/RunnerStudioShell.tsx`
- Modify: `web/src/components/runner/RunnerStudioShell.test.js`
- Modify: `web/src/components/runner/RunnerCanvas.tsx`
- Modify: `web/src/components/runner/RunnerCanvas.test.js`
- Modify: `web/src/components/runner/CanvasToolbar.tsx`
- Modify: `web/src/components/runner/CanvasToolbar.test.js`
- Modify: `web/src/components/runner/WorkflowQuickList.tsx`
- Create: `web/src/components/runner/fallbackActionCatalog.js`
- Modify: `web/src/components/runner/runnerStudio.css`
- Modify: `web/tests/runner-studio.spec.js`

- [x] 404 错误不再裸露 `Request failed with status 404`，改为解释主应用 API 未接入、旧 ai-server 二进制或路由未重载，并给出重启/上游配置提示。
- [x] `/runner` 初始只显示工作流库，不自动打开未选择工作流的空画布和运行抽屉。
- [x] 选择或新建工作流后进入编辑态，隐藏工作流列表，只保留返回工作流库按钮。
- [x] 画布连线改为基于节点端口坐标绘制，并提供输出端口到输入端口的手动连接交互。
- [x] 运行抽屉从固定底部区域改为右侧运行详情侧拉框，不再占用或覆盖画布主区域。
- [x] 补齐组件测试、页面测试、Playwright 回归测试和 in-app browser 实操验证。
- [x] Runner Studio API 404/503 时进入“本地编排模式”，不阻断工作流创建和画布编辑。
- [x] 本地编排模式提示支持手动关闭，避免长期占用首屏空间。
- [x] API action catalog 不可用时启用内置生产节点库，至少包含 Command、Shell Script、Stored Script、人工审批、条件分支、等待事件。
- [x] 节点库改为参考 Dify 的画布左侧浮动加号；点击后弹出可搜索节点面板，避免空白“动作”栏。
- [x] 外层页面改为纵向 flex 布局，Shell 填满剩余高度，运行详情改为 overlay 侧拉层且不再覆盖节点库按钮。
- [x] 删除画布内重复的 workflow/status/count 头部行，编辑页只保留顶部工作流标题和状态。
- [x] 画布左侧工具栏支持全屏编排/退出全屏，适合复杂工作流横向展开。
- [x] 重新构建并重启 8080 ai-server，确认 `/api/runner-studio/*` 已接入主应用路由；当前若无 Runner upstream，返回明确 503 配置提示而不是旧路由 404。

验证：

```bash
go test ./internal/server -run 'TestRunnerStudio' -count=1
cd web && npm test -- RunnerStudioPage.test.js RunnerStudioShell.test.js RunnerCanvas.test.js CanvasToolbar.test.js
cd web && npm test
cd web && npm run build
cd web && npm run test:ui -- tests/runner-studio.spec.js --project=chromium
```

期望：批注中的问题都有测试覆盖；真实 8080 页面进入 `/runner` 时先展示工作流库，新建后进入编排页，画布可全屏，运行详情通过侧拉框查看，返回按钮可回到列表。
