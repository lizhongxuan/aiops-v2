# aiops-v2 AI Reasoning & Prompt Trace 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-05a-ai-reasoning-prompt-trace-frontend-design.zh.md](2026-05-11-aiops-v2-05a-ai-reasoning-prompt-trace-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-05-ai-reasoning-prompt-trace-module-design.zh.md](2026-05-11-aiops-v2-05-ai-reasoning-prompt-trace-module-design.zh.md)

## 1. 目标

把现有 `PromptTracePage.tsx` 从本地 trace 文件浏览器升级为 AI Reasoning & Prompt Trace 治理工作台：支持对话/Case/DebugCase 入口、Reasoning Runs、Prompt Trace Explorer、Output Routing、Tool Visibility、Prompt Diff、Eval Reports，以及权限裁剪和敏感信息防泄露。

## 2. 实施顺序

```text
AI Reasoning / Prompt Trace API view-model
  -> Prompt Trace normalize 和安全裁剪
  -> Prompt Trace Explorer 升级
  -> Reasoning Runs 工作台
  -> Output Routing 和 Tool Visibility
  -> Chat / Protocol / Case 嵌入入口
  -> Prompt Compiler Preview
  -> Eval Reports
  -> 测试和视觉校验
```

先做 trace 可追溯、权限裁剪和工具治理解释，再做 Eval 和 Prompt Compiler Preview。不要先做 Raw trace 展示增强，也不要在前端推断生产动作是否可执行。

## 3. 文件地图

新增：

- `web/src/api/aiReasoning.ts`：AI Reasoning runs、Prompt Compiler Preview、Eval Reports API client 和类型。
- `web/src/api/aiReasoning.test.ts`：ReasoningOutput、PromptTrace、EvalReport normalize 测试。
- `web/src/api/promptTraces.ts`：迁移 `web/src/api/promptTraces.js`，支持新旧 API 兼容。
- `web/src/pages/AIReasoningPage.tsx`：AI Reasoning 工作台。
- `web/src/pages/PromptTraceDetailPage.tsx`：Prompt Trace 详情页，MVP 可由 `AIReasoningPage` 的 trace tab 承载。
- `web/src/pages/EvalReportsPage.tsx`：Eval 报告列表和运行入口。
- `web/src/pages/EvalReportDetailPage.tsx`：Eval 报告详情。
- `web/src/components/reasoning/ReasoningContextBar.tsx`：case、session、turn、trace、output type、risk 筛选。
- `web/src/components/reasoning/ReasoningSummaryStrip.tsx`：turn、trace、action draft、denied tools、eval 指标。
- `web/src/components/reasoning/ReasoningRunsTable.tsx`：AI 推理记录表。
- `web/src/components/reasoning/ReasoningOutputDrawer.tsx`：ReasoningOutput 详情。
- `web/src/components/reasoning/PromptTraceList.tsx`：Trace 列表。
- `web/src/components/reasoning/PromptTraceHeader.tsx`：Trace header 和 fingerprint。
- `web/src/components/reasoning/PromptLayerPanel.tsx`：固定 prompt layer 顺序展示。
- `web/src/components/reasoning/PromptMessagesPanel.tsx`：provider messages 展示。
- `web/src/components/reasoning/PromptEvidenceContextPanel.tsx`：Evidence、OpsGraph、Experience、Memory refs。
- `web/src/components/reasoning/PromptModelOutputPanel.tsx`：模型输出和 ReasoningOutput。
- `web/src/components/reasoning/PromptToolCallsPanel.tsx`：工具调用、审批、结果引用。
- `web/src/components/reasoning/PromptGovernancePanel.tsx`：RBAC、hidden tools、approval、policy decisions。
- `web/src/components/reasoning/PromptDiffPanel.tsx`：prompt/tool/evidence/experience diff。
- `web/src/components/reasoning/PromptRawPanel.tsx`：授权 reviewer 的 raw JSON/Markdown/diff。
- `web/src/components/reasoning/ToolVisibilityPanel.tsx`：visible tools 和 hidden tools denied。
- `web/src/components/reasoning/OutputRoutingMatrix.tsx`：输出类型到下游模块的路由矩阵。
- `web/src/components/reasoning/PromptCompilerPreview.tsx`：prompt compile 预览。
- `web/src/components/reasoning/EvalRunDialog.tsx`：运行 Eval 表单。
- `web/src/components/reasoning/EvalReportTable.tsx`：Eval 报告列表。
- `web/src/components/reasoning/EvalReportDetail.tsx`：Eval 报告详情。
- `web/src/components/reasoning/reasoningViewModels.ts`：排序、分组、权限裁剪、违规检测、diff 摘要。
- `web/src/components/reasoning/reasoningViewModels.test.ts`：view-model 单测。
- `web/src/components/reasoning/reasoningComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/PromptTracePage.tsx`：使用 `promptTraces.ts`，拆分组件，接入权限裁剪和新 tabs。
- `web/src/utils/promptTraceViewModel.js`：迁移或扩展为 `reasoningViewModels.ts`，保留兼容导出。
- `web/src/chat/components/AiopsThread.tsx`：assistant turn 增加 Prompt Trace 入口和 ReasoningOutput badge。
- `web/src/chat/components/ProcessTranscript.tsx`：过程块增加 trace link、governance warning、evidence refs 摘要。
- `web/src/pages/ProtocolWorkspacePage.tsx`：右侧 Main Agent Process 增加 Prompt Trace 和 Output Routing 摘要。
- `web/src/pages/IncidentWorkbenchPage.tsx`：Case 上下文栏增加 Reasoning Runs 摘要。
- `web/src/pages/ApprovalManagementPage.tsx`：ActionProposal 或审批详情补充 source PromptTrace 链接。
- `web/src/app/navigation.ts`：增加 `/ai-reasoning`、`/ai-reasoning/evals`、Prompt Trace 详情路由元数据。
- `web/src/router.tsx`：注册 AI Reasoning 和 Eval 页面。
- `web/src/pages/complexPages.test.tsx`：补充 AI Reasoning、Prompt Trace、Eval 路由测试。

## 4. Task 1：建立 AI Reasoning API 与类型

- [ ] 新增 `web/src/api/aiReasoning.ts`。
- [ ] 定义 `ReasoningOutputType`、`PromptLayerView`、`PromptTraceView`、`ReasoningOutputView`、`ToolVisibilityView`、`HiddenToolDeniedView`、`ToolCallTraceView`、`GovernanceDecisionView`、`EvalReportView`。
- [ ] 实现 `listReasoningRuns(params)`，请求 `GET /api/v1/ai/reasoning-runs`。
- [ ] 实现 `getReasoningRun(runId)`，请求 `GET /api/v1/ai/reasoning-runs/{run_id}`。
- [ ] 实现 `compilePromptPreview(payload)`，请求 `POST /api/v1/ai/compile-prompt`。
- [ ] 实现 `runEval(payload)`，请求 `POST /api/v1/evals/run`。
- [ ] 实现 `listEvalReports(params)`，请求 `GET /api/v1/evals/reports`。
- [ ] 实现 `getEvalReport(reportId)`，请求 `GET /api/v1/evals/reports/{id}`。
- [ ] 实现 `getEvalReportCases(reportId)`，请求 `GET /api/v1/evals/reports/{id}/cases`。
- [ ] normalize 时补齐 `routingStatus`、`risk`、`governanceDecisions`、`supportingEvidenceRefs`、`contradictingEvidenceRefs` 默认值。
- [ ] 新增 `web/src/api/aiReasoning.test.ts`，覆盖 answer、action_proposal_draft、workflow_draft、violation、eval report normalize。
- [ ] 运行 `npm --prefix web test -- aiReasoning.test.ts`。

## 5. Task 2：迁移 Prompt Trace API 和 view-model

- [ ] 新增 `web/src/api/promptTraces.ts`，迁移 `fetchPromptTraces` 和 `fetchPromptTraceFile`。
- [ ] 增加 `getPromptTrace(traceId)`，优先请求 `GET /api/v1/prompt-traces/{trace_id}`。
- [ ] 增加 `listPromptTraces(params)`，优先请求 `GET /api/v1/prompt-traces`，兼容当前 `/api/v1/debug/model-input-traces`。
- [ ] 增加 `getPromptTraceDiff(traceId, params)`、`getPromptTraceRaw(traceId, kind)`、`updatePromptTraceEvalLabels(traceId, labels)`。
- [ ] 扩展 `parsePromptTrace` 或新增 `normalizePromptTraceView`，输出 `PromptTraceView`。
- [ ] 支持旧 trace 文件字段：`modelInput`、`visibleTools`、`promptFingerprint`、`prompt.tools`、`metadata`。
- [ ] 新增敏感字段裁剪：secret、token、password、cookie、authorization、private key、连接串。
- [ ] 保留 `web/src/api/promptTraces.js` 兼容导出，或统一修改调用点后删除。
- [ ] 扩展 `promptTraceViewModel.test.js` 或新增 `reasoningViewModels.test.ts`，覆盖旧字段兼容和裁剪。
- [ ] 运行 `npm --prefix web test -- promptTraceViewModel.test.js reasoningViewModels.test.ts`。

## 6. Task 3：实现 reasoning view-model

- [ ] 新增 `web/src/components/reasoning/reasoningViewModels.ts`。
- [ ] 实现 `detectOutputViolation(output)`：自然语言 answer 包含写动作但没有 draft 时返回 violation。
- [ ] 实现 `buildOutputRouting(output)`：把 output type 映射到 Incident、Governed Action、Execution Fabric、Middleware Repair、Learning、Verification。
- [ ] 实现 `groupPromptLayers(trace)`：按 System Policy、Product Invariants、User RBAC、Case Summary、Evidence Summary、OpsGraph Context、Activated Experience、Runbook/Workflow Metadata、Tool Visibility、Recent Transcript、User Turn 固定顺序分组。
- [ ] 实现 `summarizeToolVisibility(trace)`：统计 visible、hidden denied、high risk、destructive、approval required。
- [ ] 实现 `buildGovernanceWarnings(trace)`：输出 RBAC clip、tool hidden、approval required、action blocked、output violation。
- [ ] 实现 `summarizePromptDiff(diff)`：输出 changed layers、added/removed tools、evidence refs、experience refs、fingerprints。
- [ ] 单测覆盖输出违规、工具隐藏原因、prompt layer 顺序、diff 摘要。

## 7. Task 4：升级 Prompt Trace Explorer

- [ ] 新增 `PromptTraceList.tsx`。
- [ ] 新增 `PromptTraceHeader.tsx`。
- [ ] 新增 `PromptLayerPanel.tsx`。
- [ ] 新增 `PromptMessagesPanel.tsx`。
- [ ] 新增 `PromptEvidenceContextPanel.tsx`。
- [ ] 新增 `PromptModelOutputPanel.tsx`。
- [ ] 新增 `PromptToolCallsPanel.tsx`。
- [ ] 新增 `PromptGovernancePanel.tsx`。
- [ ] 新增 `PromptDiffPanel.tsx`。
- [ ] 新增 `PromptRawPanel.tsx`。
- [ ] 修改 `PromptTracePage.tsx`，使用 `promptTraces.ts`，去掉页面内裸 `fetch`。
- [ ] Tabs 包含 Overview、Layers、Messages、Evidence & Context、Model Output、Tool Calls、Governance、Diff、Raw。
- [ ] Raw tab 仅 reviewer 可见，默认折叠。
- [ ] 单测覆盖 trace 列表过滤、layer 展示、raw 权限隐藏、敏感字段不渲染。

## 8. Task 5：实现 AI Reasoning 工作台

- [ ] 新增 `AIReasoningPage.tsx`。
- [ ] 新增 `ReasoningContextBar.tsx`。
- [ ] 新增 `ReasoningSummaryStrip.tsx`。
- [ ] 新增 `ReasoningRunsTable.tsx`。
- [ ] 新增 `ReasoningOutputDrawer.tsx`。
- [ ] 页面 tabs 包含 Reasoning Runs、Prompt Trace Explorer、Output Routing、Tool Visibility、Prompt Diff、Eval Reports。
- [ ] URL query 支持 `caseId`、`sessionId`、`turnId`、`traceId`、`outputType`、`risk`、`model`、`toolName`、`evidenceRef`、`experienceRef`、`governanceDecision`、`evalLabel`、`tab`。
- [ ] Reasoning Runs 表格列包括 Turn、User Intent、Output Type、Model、Evidence、Tools、Governance、Risk、Action。
- [ ] 详情 Drawer 展示 ReasoningOutput、confidence、supporting/contradicting evidence、risk、downstream target、governance decisions、PromptTrace link。
- [ ] 单测覆盖 `/ai-reasoning?caseId=...&traceId=...` 落点。

## 9. Task 6：实现 Output Routing 和 Tool Visibility

- [ ] 新增 `OutputRoutingMatrix.tsx`。
- [ ] 展示 output type 到 Incident、Governed Action、Execution Fabric、Middleware Repair、Learning、Verification 的路由。
- [ ] 对 `answer` 中包含写动作但没有 draft 的输出显示 `output type violation`。
- [ ] 新增 `ToolVisibilityPanel.tsx`。
- [ ] Visible Tools 列包括 tool name、server、risk、mutating、approval requirement、result budget、visible reason。
- [ ] Hidden Tools Denied 列包括 tool name、denied reason、policy rule、RBAC scope、resource scope、risk、replacement suggestion。
- [ ] 高风险和破坏性工具必须显示 approval requirement 或 blocked reason。
- [ ] 单测覆盖 hidden tools、destructive blocked、answer violation。

## 10. Task 7：嵌入 Chat / Protocol / Case

- [ ] `AiopsThread.tsx` 的 assistant turn 增加 Prompt Trace 链接。
- [ ] `AiopsThread.tsx` 展示 ReasoningOutput badge 和 governance warning badge。
- [ ] `ProcessTranscript.tsx` 的过程块增加 trace link、evidence refs 摘要、tool visibility warning。
- [ ] `ProtocolWorkspacePage.tsx` 的 Main Agent Process 增加 Prompt Trace 和 Output Routing 摘要。
- [ ] `IncidentWorkbenchPage.tsx` 的上下文栏增加 Reasoning Runs 摘要和当前 hypothesis 对应 trace。
- [ ] `ApprovalManagementPage.tsx` 的审批详情展示 source PromptTrace、model output type、evidence refs、risk reason。
- [ ] 所有链接使用 `/ai-reasoning?traceId=...` 或 `/ai-reasoning/prompt-traces/:traceId`。
- [ ] 单测覆盖 Chat、Protocol、Case、Approval 中 trace link 的 URL。

## 11. Task 8：实现 Prompt Compiler Preview

- [ ] 新增 `PromptCompilerPreview.tsx`。
- [ ] 表单字段包括 caseId、sessionId、user turn、target hosts/resource scope、evidence refs、enabled experience packs、desired output type、model。
- [ ] 提交调用 `compilePromptPreview(payload)`。
- [ ] 展示 prompt layers、token estimate、visible tools、hidden tools denied、redaction report、prompt fingerprint、warnings。
- [ ] Preview 不能创建 case、ActionProposal、Workflow 或写入 trace，按钮文案必须表达只读预览。
- [ ] 单测覆盖 compile 成功、redaction warning、只读限制。

## 12. Task 9：实现 Eval Reports

- [ ] 新增 `EvalReportsPage.tsx`。
- [ ] 新增 `EvalReportDetailPage.tsx`。
- [ ] 新增 `EvalRunDialog.tsx`。
- [ ] 新增 `EvalReportTable.tsx`。
- [ ] 新增 `EvalReportDetail.tsx`。
- [ ] Eval Run 表单字段包括 eval suite、case category、baseline prompt fingerprint、candidate prompt fingerprint、tool registry version、experience index version、model、sample size。
- [ ] 报告列表列包括 Report、Change、Pass Rate、Safety、Regression、Action。
- [ ] 报告详情展示 summary、failed cases、regression matrix、expected vs actual ReasoningOutput、evidence citation correctness、action governance correctness、missing evidence handling、trace links、suggested fixes。
- [ ] 失败 case 能跳到 Prompt Trace 和 Case。
- [ ] 单测覆盖运行 Eval、报告列表、失败 case trace link。

## 13. Task 10：导航、路由和兼容入口

- [ ] 修改 `web/src/app/navigation.ts`，增加 `/ai-reasoning`、`/ai-reasoning/evals`、`/ai-reasoning/evals/:reportId`、`/ai-reasoning/prompt-traces/:traceId`。
- [ ] 修改 `web/src/router.tsx` 注册 `AIReasoningPage`、`PromptTraceDetailPage`、`EvalReportsPage`、`EvalReportDetailPage`。
- [ ] `/debug/prompts` 继续可用，复用 `PromptTracePage` 或跳转到 `/ai-reasoning?tab=trace`。
- [ ] 文案明确 `/debug/prompts` 是 LLM Prompt Trace，不是 04 模块的业务 Debug Trace。
- [ ] 扩展 `complexPages.test.tsx`，覆盖新路由不显示 placeholder。

## 14. Task 11：测试与视觉检查

- [ ] 新增 `reasoningComponents.test.tsx`，覆盖 ReasoningRunsTable、PromptLayerPanel、PromptToolCallsPanel、PromptGovernancePanel、ToolVisibilityPanel、OutputRoutingMatrix、EvalReportTable。
- [ ] 扩展 `ChatPage.test.tsx`，覆盖 assistant turn 的 Prompt Trace 入口。
- [ ] 扩展 `ProcessTranscript.test.tsx`，覆盖 trace link、governance warning、evidence refs。
- [ ] 运行 `npm --prefix web test -- aiReasoning.test.ts reasoningViewModels.test.ts reasoningComponents.test.tsx promptTraceViewModel.test.js`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含页面视觉变更，启动 dev server 并用浏览器截图检查 `/ai-reasoning`、`/debug/prompts`、`/ai-reasoning/evals` 的桌面和移动宽度。
- [ ] 检查 `/ai-reasoning?traceId=...&tab=trace`、`/ai-reasoning?caseId=...&tab=runs`、`/ai-reasoning/evals/:reportId` 能正确落点。

## 15. 交付检查

- [ ] `PromptTracePage.tsx` 不直接调用裸 `fetch`。
- [ ] 每轮 AI turn 在 Chat、Protocol Workspace、Case 工作台中都有 Prompt Trace 入口。
- [ ] Prompt Trace 详情能展示 layers、tools、hiddenToolsDenied、evidenceRefs、opsGraphQueryRefs、activatedExperienceRefs、modelOutput、toolCalls、governanceDecisions、diff、raw。
- [ ] 普通用户看不到完整系统策略、敏感 evidence、SecretRef 解密结果或 raw chain-of-thought。
- [ ] 写动作必须路由到 ActionProposal、Workflow draft 或 RepairPlan draft。
- [ ] `answer` 中包含写动作但没有 draft 时显示治理违规。
- [ ] Tool Visibility 能解释工具可见和不可见原因。
- [ ] Prompt Compiler Preview 不触发生产动作，不写 case。
- [ ] Eval Reports 能展示 pass rate、安全失败、回归、新失败和失败 trace。
- [ ] 页面不渲染 token、password、cookie、authorization header、私钥、完整连接串。
- [ ] `/debug/prompts` 保持兼容，且不和 04 模块业务 Debug Trace 混用。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
