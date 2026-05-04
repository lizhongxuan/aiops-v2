# Runner Studio 实施工作清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `/runner` 落地为主应用原生一体化的 Runner Studio，提供简洁的 Dify 风格工作流编排体验、结构化参数输入输出编辑、AI 辅助、发布治理和运行闭环。

**Architecture:** 主应用前端直接承载 `workflow shell + canvas + modal/drawer`，复用现有 runner graph/API 语义，不再保留单独 Runner UI 作为用户入口。参数模型改为结构化输入输出定义，YAML 退回为兼容视图与导入导出格式。

**Tech Stack:** Vue 3、现有 `web/` 路由与状态管理、Runner graph API、Naive UI、拖拽画布组件、Monaco YAML、现有 LLM 接口。

---

## 0. 范围与约束

- [ ] `http://127.0.0.1:8080/runner` 必须成为最终产品入口。
- [ ] 旧的 `RunnerUiEntryPage` 只能作为临时过渡代码，不再作为最终交互方案。
- [ ] 不保留“主应用入口页 + 外部 Runner UI”这套长期形态。
- [ ] 主页面必须简洁，复杂配置统一改为弹窗和抽屉。
- [ ] `graph` 是唯一编排语义，YAML 只作为兼容视图。
- [ ] AI 生成和 AI 修改只能生成 `draft`，允许 `dry-run`，不允许直接发布。

验收：

- [ ] 不再出现“点击 Runner 编排后不知道如何拖拽”的问题。
- [ ] 主界面默认只聚焦工作流选择、画布编排、运行三个主任务。

## 1. 主应用 Runner Shell 收敛

**Files:**
- Modify: `aiops-v2/web/src/router.js`
- Modify: `aiops-v2/web/src/App.vue`
- Create: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Test: `aiops-v2/web/src/pages/RunnerStudioPage.test.js`
- Test: `aiops-v2/web/tests/router.erp-sre.spec.js`

- [ ] 把 `/runner` 路由指向新的 `RunnerStudioPage`。
- [ ] 移除当前“打开 Runner UI / 启动 runner server”的跳板式说明内容。
- [ ] 在主应用内建立 Runner Studio 顶层壳层，包括顶栏、左栏、画布区、底部抽屉挂载点。
- [ ] 保留对后端不可用状态的明确反馈，但不能退回为跳板式页面。
- [ ] 更新路由与页面测试，验证 `/runner` 渲染的是 Studio 页面而非外部入口说明。

验收：

- [ ] 进入 `/runner` 直接看到工作流编排主界面。
- [ ] 页面在 Runner API 不可用时会显示可恢复错误态，而不是要求用户自行理解双入口。

## 2. 工作流选择与管理弹窗

**Files:**
- Create: `aiops-v2/web/src/components/runner/WorkflowManagerModal.vue`
- Create: `aiops-v2/web/src/components/runner/WorkflowQuickList.vue`
- Create: `aiops-v2/web/src/components/runner/workflowManagerState.js`
- Modify: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Modify: `aiops-v2/web/src/api/runnerUi.js`
- Test: `aiops-v2/web/src/components/runner/WorkflowManagerModal.test.js`

- [ ] 主界面左侧只显示最近使用 / 收藏工作流。
- [ ] 完整工作流列表、搜索、筛选、归档、克隆、历史版本统一进入 `WorkflowManagerModal`。
- [ ] 新建入口固定为：空白、模板、YAML 导入、克隆、AI 生成。
- [ ] 当前工作流切换时处理脏状态确认，不允许静默丢失草稿。
- [ ] 支持最近使用排序和收藏固定。

验收：

- [ ] 用户在主界面不会被完整列表淹没。
- [ ] 工作流切换、创建、克隆都有清晰入口。

## 3. 画布主界面简化

**Files:**
- Create: `aiops-v2/web/src/components/runner/RunnerCanvas.vue`
- Create: `aiops-v2/web/src/components/runner/CanvasToolbar.vue`
- Create: `aiops-v2/web/src/components/runner/NodeActionMenu.vue`
- Modify: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Test: `aiops-v2/web/src/components/runner/RunnerCanvas.test.js`

- [ ] 复用现有拖拽、连线、缩放、自动布局能力，但收敛为主应用组件。
- [ ] 主画布默认不显示复杂右栏参数面板。
- [ ] 单击节点只显示摘要。
- [ ] 双击节点打开配置弹窗。
- [ ] 右键节点打开动作菜单：复制、删除、禁用/启用、单节点试跑、查看最近运行、AI 修复参数。
- [ ] 左侧保留节点库和模板片段，不在画布周围堆叠二级控制面板。

验收：

- [ ] 新用户打开页面后能直觉理解“从左拖到中间，再双击配置”。
- [ ] 画布主视野不会被右侧大面板压缩。

## 4. 节点配置弹窗

**Files:**
- Create: `aiops-v2/web/src/components/runner/NodeConfigModal.vue`
- Create: `aiops-v2/web/src/components/runner/node-config/BasicTab.vue`
- Create: `aiops-v2/web/src/components/runner/node-config/InputTab.vue`
- Create: `aiops-v2/web/src/components/runner/node-config/OutputTab.vue`
- Create: `aiops-v2/web/src/components/runner/node-config/AdvancedTab.vue`
- Create: `aiops-v2/web/src/components/runner/node-config/RunAiTab.vue`
- Modify: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Test: `aiops-v2/web/src/components/runner/NodeConfigModal.test.js`

- [ ] 节点配置统一收敛到一个弹窗，固定五个页签：基础、输入、输出、高级、运行与 AI。
- [ ] `基础` 页签支持 `name/action/targets/retries/timeout`。
- [ ] `高级` 页签默认折叠低频项：`when`、`continue_on_error`、`env`、`workdir`、`failure edge`、`join/loop/subflow`。
- [ ] 节点标题和说明在弹窗内统一编辑，不在画布上直接承载复杂文字。
- [ ] 节点级校验问题在弹窗内就地展示。

验收：

- [ ] 节点配置不再依赖常驻右栏。
- [ ] 同一节点的高频字段和低频字段分层明确。

## 5. 结构化输入参数编辑器

**Files:**
- Create: `aiops-v2/web/src/components/runner/io/InputParamList.vue`
- Create: `aiops-v2/web/src/components/runner/io/InputParamRow.vue`
- Create: `aiops-v2/web/src/components/runner/io/ValueSourceSwitch.vue`
- Create: `aiops-v2/web/src/components/runner/io/VariableReferencePicker.vue`
- Create: `aiops-v2/web/src/components/runner/io/MixedVariableTextInput.vue`
- Create: `aiops-v2/web/src/components/runner/io/ioTypes.js`
- Test: `aiops-v2/web/src/components/runner/io/InputParamList.test.js`

- [ ] 输入参数改为列表行编辑，不再以 YAML 大文本块为主入口。
- [ ] 每行至少支持：`key`、`label`、`type`、`value_source`、`value`、`required`、`description`。
- [ ] `value_source` 固定为 `constant`、`variable_reference`、`expression`。
- [ ] 变量引用必须通过 picker 选择，而非手写路径。
- [ ] 文本型字段支持混合变量高亮。
- [ ] 支持排序、复制、删除、重复名校验。

验收：

- [ ] 配置 `cmd.run` 或 `shell.run` 的常见参数时，不需要直接编辑 YAML。
- [ ] 用户可以用 picker 选择上游变量填入参数。

## 6. 结构化输出参数编辑器

**Files:**
- Create: `aiops-v2/web/src/components/runner/io/OutputParamList.vue`
- Create: `aiops-v2/web/src/components/runner/io/OutputParamRow.vue`
- Create: `aiops-v2/web/src/components/runner/io/ExtractSourceSelect.vue`
- Create: `aiops-v2/web/src/components/runner/io/JsonPathEditor.vue`
- Test: `aiops-v2/web/src/components/runner/io/OutputParamList.test.js`

- [ ] 输出变量改为显式声明，不依赖隐式 `expect_vars` 约定。
- [ ] 每行至少支持：`key`、`type`、`extract_source`、`extract_rule`、`description`。
- [ ] 首版支持的 `extract_source`：`stdout_text`、`stdout_jsonpath`、`stderr_text`、`exit_code`、`export_var`、`approval_result`、`subflow_output`。
- [ ] 输出变量支持命名冲突校验和类型校验。
- [ ] 输出变量可用于下游变量引用选择器。

验收：

- [ ] 用户能在 UI 上直接声明某个节点输出什么变量、从哪里提取。
- [ ] 下游节点配置参数时能引用这些输出变量。

## 7. YAML / Diff 弹窗

**Files:**
- Create: `aiops-v2/web/src/components/runner/YamlDiffModal.vue`
- Create: `aiops-v2/web/src/components/runner/YamlEditorPane.vue`
- Create: `aiops-v2/web/src/components/runner/GraphDiffSummary.vue`
- Modify: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Test: `aiops-v2/web/src/components/runner/YamlDiffModal.test.js`

- [ ] YAML 作为兼容视图保留，但不常驻主界面。
- [ ] 提供 `Graph -> YAML` 和 `YAML -> Graph` 双向预览与校验。
- [ ] 在弹窗内集中展示 diff、编译错误和语义冲突。
- [ ] 不可线性化的 graph 不得伪装成顺序 YAML。

验收：

- [ ] 高级用户仍可检查和编辑 YAML。
- [ ] 普通用户不被 YAML 文本打断主流程。

## 8. 底部运行抽屉与节点运行详情

**Files:**
- Create: `aiops-v2/web/src/components/runner/RunLogDrawer.vue`
- Create: `aiops-v2/web/src/components/runner/VariableInspectDrawer.vue`
- Create: `aiops-v2/web/src/components/runner/NodeRunDetailModal.vue`
- Create: `aiops-v2/web/src/components/runner/runStateReducer.js`
- Modify: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Test: `aiops-v2/web/src/components/runner/RunLogDrawer.test.js`

- [ ] 日志抽屉显示：stdout、stderr、SSE、审批事件、重试轨迹。
- [ ] 变量检查抽屉显示：输入变量、输出变量、运行态导出变量、最近节点结果。
- [ ] 单击节点显示最近一次运行摘要。
- [ ] 节点详情弹窗显示单节点详细执行结果。
- [ ] 抽屉默认收起，运行时可自动展开。

验收：

- [ ] 运行态信息不再和画布抢占主视野。
- [ ] 用户能从画布快速钻取到单节点执行详情。

## 9. AI 助手

**Files:**
- Create: `aiops-v2/web/src/components/runner/ai/RunnerAiAssistantModal.vue`
- Create: `aiops-v2/web/src/components/runner/ai/AiDiffPreview.vue`
- Create: `aiops-v2/web/src/components/runner/ai/aiRunnerApi.js`
- Modify: `aiops-v2/web/src/pages/RunnerStudioPage.vue`
- Test: `aiops-v2/web/src/components/runner/ai/RunnerAiAssistantModal.test.js`

- [ ] 页面级支持自然语言生成工作流草稿。
- [ ] 节点级支持补齐参数和修复校验问题。
- [ ] 运行后支持解释失败原因。
- [ ] AI 修改只生成 draft diff，不得静默改已发布版本。
- [ ] AI 操作结果必须带可审计说明。

验收：

- [ ] AI 可以帮助用户从空白工作流起步。
- [ ] AI 结果不会越过发布门禁。

## 10. 发布审阅与审计

**Files:**
- Create: `aiops-v2/web/src/components/runner/PublishReviewModal.vue`
- Modify: `aiops-v2/internal/server/erp_sre_api.go`
- Modify: `aiops-v2/internal/server/http.go`
- Modify: `aiops-v2/pkg/runner/server/api/router.go`
- Modify: `aiops-v2/pkg/runner/server/service/` 下发布与审计相关服务
- Test: `aiops-v2/pkg/runner/server/api/visual_workflow_audit_test.go`
- Test: `aiops-v2/web/src/components/runner/PublishReviewModal.test.js`

- [ ] 发布前统一进入审阅弹窗。
- [ ] 审阅弹窗集中展示：diff、风险摘要、校验结果、发布说明。
- [ ] AI 变更、YAML 导入、结构化表单改动都进入统一审计。
- [ ] 审计数据至少包含：操作者、时间、动作、结构化 diff。

验收：

- [ ] 发布与审计是一套机制，而不是多个分散开关。
- [ ] 高风险工作流发布前能看到完整变更摘要。

## 11. 后端参数 schema 与变量契约

**Files:**
- Modify: `aiops-v2/pkg/runner/workflow/visual/**`
- Modify: `aiops-v2/pkg/runner/server/api/visual_workflow_handler.go`
- Modify: `aiops-v2/pkg/runner/server/service/**`
- Create: `aiops-v2/web/src/api/runnerStudioClient.js`
- Test: `aiops-v2/pkg/runner/server/api/visual_workflow_handler_test.go`
- Test: `aiops-v2/pkg/runner/workflow/visual/**` 对应单测

- [ ] 为 action catalog 增加输入输出 schema 描述能力。
- [ ] 为 graph node 增加结构化输入输出字段映射。
- [ ] 将 UI 的结构化参数编辑与后端 graph schema 对齐。
- [ ] 保证 Graph <-> YAML round-trip 不丢失输入输出语义。

验收：

- [ ] 前端结构化表单不是前端私有状态，而是后端契约的一部分。
- [ ] YAML 导入后能还原输入输出结构。

## 12. 测试与验收

**Files:**
- Modify: `aiops-v2/web/tests/` 下 Runner 相关 e2e
- Modify: `aiops-v2/web/src/components/runner/**` 对应测试目录
- Modify: `aiops-v2/pkg/runner/server/api/` 测试
- Modify: `aiops-v2/pkg/runner/workflow/visual/` 测试

- [ ] 组件测试覆盖：工作流管理、节点配置、输入输出编辑器、变量 picker、发布弹窗。
- [ ] 流程测试覆盖：新建 -> 拖拽 -> 配输入输出 -> 校验 -> Dry Run -> 运行。
- [ ] e2e 测试覆盖：从空白工作流到成功运行。
- [ ] 使用真实 LLM 配置验证 AI 生成、AI 补参数和失败解释链路。
- [ ] 用 Playwright 回归验证 `/runner` 首屏和关键主流程。

验收：

- [ ] 主流程在真实浏览器里可跑通。
- [ ] 文档状态与页面实际状态一致，不再出现“文档完成但入口不可用”的偏差。
