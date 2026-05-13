# aiops-v2 OpsGraph & ERP Business Context 前端实施 TODO

日期：2026-05-11
状态：实施任务清单
来源设计：[2026-05-11-aiops-v2-03a-opsgraph-business-context-frontend-design.zh.md](2026-05-11-aiops-v2-03a-opsgraph-business-context-frontend-design.zh.md)
来源模块：[2026-05-11-aiops-v2-03-opsgraph-business-context-module-design.zh.md](2026-05-11-aiops-v2-03-opsgraph-business-context-module-design.zh.md)

## 1. 目标

把现有 `OpsGraphPage.tsx` 从实体检索和邻域展示升级为 OpsGraph & ERP Business Context 前端工作台：支持 Entity Search、Impact Map、Root Cause Path、Asset Map、Patch Review，以及 Case、ERP Health、Debug Trace、Experience Pack Review 的嵌入式复用。

## 2. 实施顺序

```text
OpsGraph API view-model
  -> 工作台骨架
  -> 实体搜索和详情
  -> 业务影响图
  -> 根因路径
  -> 资产匹配
  -> 图谱补丁审核
  -> Case / ERP / Debug / Experience 集成
  -> 测试和视觉校验
```

先做只读图谱工作台和边证据解释，再接入 patch 审核。不要先做复杂自由拖拽画布或前端自行推理根因。

## 3. 文件地图

新增：

- `web/src/api/opsgraph.ts`：OpsGraph API client、类型、兼容当前 `/lookup`、`/neighborhood`、`/business-impact` 的 view-model adapter。
- `web/src/api/opsgraph.test.ts`：OpsGraph API normalize 和兼容字段测试。
- `web/src/components/opsgraph/OpsGraphSearchBar.tsx`：实体、case、trace、ERP job 查询栏。
- `web/src/components/opsgraph/OpsGraphSummaryStrip.tsx`：业务影响、关键路径、可用资产、待审补丁和低置信边指标。
- `web/src/components/opsgraph/OpsGraphEntityTable.tsx`：实体搜索结果表格。
- `web/src/components/opsgraph/OpsGraphEntityCard.tsx`：实体摘要卡片。
- `web/src/components/opsgraph/OpsGraphImpactMap.tsx`：业务影响图和 Impact Summary。
- `web/src/components/opsgraph/OpsGraphPathExplorer.tsx`：根因候选路径列表和路径详情。
- `web/src/components/opsgraph/OpsGraphAssetMap.tsx`：Runbook、Workflow、Experience Pack、Memory、EvalCase 匹配表。
- `web/src/components/opsgraph/OpsGraphPatchReviewPanel.tsx`：图谱补丁列表、详情和审核入口。
- `web/src/components/opsgraph/OpsGraphDetailDrawer.tsx`：实体、边、证据详情 Drawer。
- `web/src/components/opsgraph/OpsGraphEvidenceRefs.tsx`：证据引用和脱敏显示组件。
- `web/src/components/opsgraph/OpsGraphLayeredGraph.tsx`：固定层级图谱组件。
- `web/src/components/opsgraph/opsgraphViewModels.ts`：前端派生状态、排序、分组、文案和边质量计算。
- `web/src/components/opsgraph/opsgraphViewModels.test.ts`：view-model 单测。
- `web/src/components/opsgraph/opsgraphComponents.test.tsx`：核心组件渲染测试。

修改：

- `web/src/pages/OpsGraphPage.tsx`：改为消费 `web/src/api/opsgraph.ts` 和拆分组件。
- `web/src/api/opsgraph.js`：保留兼容导出或迁移到 TypeScript 后删除旧实现。
- `web/src/pages/ERPHealthPage.tsx`：增加 OpsGraph 业务上下文摘要和跳转入口。
- `web/src/pages/IncidentWorkbenchPage.tsx`：在 Case 上下文栏复用 OpsGraph Impact / RCA / Asset 摘要组件。
- `web/src/pages/complexPagesApi.ts`：停止继续扩展 OpsGraph API，必要时保留兼容 re-export。
- `web/src/pages/complexPages.test.tsx`：补充 `/opsgraph`、ERP 跳转、case 跳转、权限裁剪测试。
- `web/src/app/navigation.ts`：确认 OpsGraph 导航文案和排序。
- `web/src/router.tsx`：确认 `/opsgraph` 路由支持 query 参数落点。

## 4. Task 1：建立 OpsGraph API 与类型

- [ ] 新增 `web/src/api/opsgraph.ts`。
- [ ] 定义 `OpsGraphEntityType`、`GraphEntityView`、`GraphEdgeView`、`EvidenceRefView`、`OpsGraphNeighborhoodView`、`ImpactMapView`、`RootCausePathView`、`AssetMatchView`、`OpsGraphPatchView`。
- [ ] 实现 `lookupOpsGraph(params)`，兼容当前 `POST /api/v1/opsgraph/lookup`。
- [ ] 实现 `getOpsGraphEntity(entityId)`，请求 `GET /api/v1/opsgraph/entities/{id}`。
- [ ] 实现 `getOpsGraphNeighborhood(entityId, params)`，优先请求 `GET /api/v1/opsgraph/entities/{id}/neighbors`，兼容当前 `/neighborhood`。
- [ ] 实现 `queryOpsGraph(payload)`，请求 `POST /api/v1/opsgraph/query`。
- [ ] 实现 `getImpactMap(payload)`，请求 `POST /api/v1/opsgraph/impact`，兼容当前 `/business-impact`。
- [ ] 实现 `getRootCausePaths(payload)`，请求 `POST /api/v1/opsgraph/root-cause-paths`。
- [ ] 实现 `getExperienceMatches(payload)`，请求 `POST /api/v1/opsgraph/experience-match`。
- [ ] 实现 `listOpsGraphPatches(params)`、`getOpsGraphPatch(patchId)`、`reviewOpsGraphPatch(patchId, payload)`。
- [ ] 所有 normalize 函数必须补齐 `redactionStatus`、`confidence`、`evidenceRefs`、`automationEligible` 默认值。
- [ ] 新增 `web/src/api/opsgraph.test.ts`，覆盖旧字段和新字段都能 normalize。
- [ ] 运行 `npm --prefix web test -- opsgraph.test.ts`。

## 5. Task 2：重构 OpsGraph 工作台骨架

- [ ] 新增 `web/src/components/opsgraph` 目录。
- [ ] 新增 `OpsGraphSearchBar.tsx`，支持 entity、case、trace、ERP job、manual query。
- [ ] 新增 `OpsGraphSummaryStrip.tsx`，展示业务影响数量、关键路径数量、资产匹配数量、待审 patch 数量、低置信边数量。
- [ ] 新增 `OpsGraphDetailDrawer.tsx`，支持 Entity、Edge、Evidence 三类详情。
- [ ] 修改 `OpsGraphPage.tsx`，结构改成 Header、Context Bar、Summary Strip、Tabs、Detail Drawer。
- [ ] Tabs 使用现有 UI 组件，包含 Entity Search、Impact Map、Root Cause Path、Asset Map、Patch Review。
- [ ] URL query 支持 `entityId`、`caseId`、`traceId`、`erpJobId`、`tab`、`environment`、`timeWindow`、`source`、`confidence`。
- [ ] Loading 和 error 状态按 tab 隔离，不让 Impact Map 失败影响 Search Tab。
- [ ] 单测覆盖 query 参数初始化和 tab 切换。

## 6. Task 3：实现实体搜索和详情

- [ ] 新增 `OpsGraphEntityTable.tsx`。
- [ ] 搜索结果列包括 Entity、业务上下文、技术上下文、数据源、置信度、更新时间、操作。
- [ ] 点击实体后加载实体详情、邻域、业务影响摘要和资产匹配摘要。
- [ ] 新增 `OpsGraphEntityCard.tsx`，展示 id、type、displayName、environment、owner、sourceRefs、redactionStatus。
- [ ] Detail Drawer 的 Entity Detail 显示业务上下文、技术上下文、关联 case、Runbook、Workflow、Experience Pack。
- [ ] 权限不足实体只显示 id、type、hash 和受限原因。
- [ ] 单测覆盖搜索结果、实体选择、受限实体裁剪。

## 7. Task 4：实现 Impact Map

- [ ] 新增 `OpsGraphImpactMap.tsx`。
- [ ] 新增 `OpsGraphLayeredGraph.tsx`，使用固定层级布局展示 Host/Pod/Middleware -> Service -> APIRoute -> BusinessCapability -> ERPModule -> Tenant/SLO/ERPJob。
- [ ] Impact Summary 表格列包括 Business Capability、Impact Level、Evidence、Confidence、Active Cases、Suggested Entry。
- [ ] 节点状态支持 healthy、degraded、critical、unknown、restricted。
- [ ] 边必须展示 source 和 confidence 摘要。
- [ ] 低置信边使用虚线或明确标识，并且不参与自动化动作建议。
- [ ] 右侧业务证据栏展示 ERP 指标、SLO、active cases 和 evidenceRefs。
- [ ] 单测覆盖低置信边样式、业务影响排序、受限证据不渲染正文。

## 8. Task 5：实现 Root Cause Path

- [ ] 新增 `OpsGraphPathExplorer.tsx`。
- [ ] 支持 ERP module、ERP job、business error code、Debug trace、case、BusinessCapability、manual symptom query 作为输入。
- [ ] 候选路径列表列包括 Rank、Path、Confidence、Evidence、Blocking Unknowns、Suggested Next。
- [ ] 路径详情展示分段路径图、每条边的 source、confidence、validFrom、validTo、updatedAt、evidenceRefs。
- [ ] 展示 supportingEvidence 和 contradictingEvidence。
- [ ] 未确认路径文案必须是“候选路径”，不能显示为“确定根因”。
- [ ] Suggested Next 只生成打开 case、补充观测、匹配资产或创建 ActionProposal 的入口。
- [ ] 单测覆盖候选路径文案、反证展示、权限受限证据裁剪。

## 9. Task 6：实现 Asset Map

- [ ] 新增 `OpsGraphAssetMap.tsx`。
- [ ] 展示 RunbookVersion、WorkflowVersion、ExperiencePack、Memory、EvalCase、Historical Case、VerificationSpec。
- [ ] 匹配表格列包括 Asset、Match Reason、Status、Risk、Compatibility、Last Outcome、Action。
- [ ] 详情 Drawer 展示适用范围、禁用条件、所需权限、审批等级、输入变量、host label 需求、最近成功和失败案例。
- [ ] disabled asset 必须显示禁用原因，不能显示执行入口。
- [ ] 所有执行入口只创建或跳转 Governed Action Plane 的 ActionProposal。
- [ ] 单测覆盖 disabled asset、risk 排序、ActionProposal 入口。

## 10. Task 7：实现 Patch Review

- [ ] 新增 `OpsGraphPatchReviewPanel.tsx`。
- [ ] 补丁列表列包括 Patch、Operations、Risk、Evidence、Status、Reviewer、Action。
- [ ] 补丁详情展示变更摘要、operation before / after、证据引用、受影响业务能力、服务、中间件、Runbook、Experience Pack。
- [ ] 审核 Dialog 支持 approve 和 reject。
- [ ] approve / reject 都必须填写审核意见。
- [ ] 高风险 patch 审核前必须展示受影响业务能力和低置信边。
- [ ] 权限不足用户只能查看，不能审批。
- [ ] 调用 `reviewOpsGraphPatch(patchId, payload)`，成功后刷新 patch 列表和详情。
- [ ] 单测覆盖审核意见必填、权限不足隐藏审批按钮、失败时保留用户输入。

## 11. Task 8：实现证据和脱敏组件

- [ ] 新增 `OpsGraphEvidenceRefs.tsx`。
- [ ] 支持 `visible`、`redacted`、`restricted` 三种显示状态。
- [ ] `restricted` 只显示 evidenceId、source、observedAt 和受限原因。
- [ ] 不允许通过 tooltip、title、aria-label 泄露敏感正文。
- [ ] 对 request body、cookie、token、password、用户输入原文做前端兜底隐藏。
- [ ] 所有 Entity Detail、Edge Detail、Impact Map、Root Cause Path、Patch Review 都复用该组件。
- [ ] 单测覆盖敏感字段不会出现在 DOM。

## 12. Task 9：嵌入 Case / ERP / Debug / Experience 页面

- [ ] 在 `IncidentWorkbenchPage.tsx` 或 Case 上下文栏中复用 Impact Summary、Root Cause Path Summary、Asset Match Summary。
- [ ] Case 页面只显示摘要和跳转入口，不嵌入完整大图。
- [ ] 在 `ERPHealthPage.tsx` 增加 ERP module 到 BusinessCapability 的映射摘要、受影响服务和中间件列表、Root Cause Path 跳转。
- [ ] Debug 事件完成后生成 `/opsgraph?traceId=...&tab=root-cause` 链接。
- [ ] Experience Pack Review 页面展示匹配实体、适用边、禁用边和 OpsGraphPatch preview。
- [ ] 所有嵌入组件使用同一个 `web/src/api/opsgraph.ts` 和 view-model，不复制请求逻辑。
- [ ] 单测覆盖 Case、ERP、Debug 跳转 URL 和摘要裁剪。

## 13. Task 10：测试与视觉检查

- [ ] 扩展 `web/src/pages/complexPages.test.tsx`，覆盖 `/opsgraph` 五个 tabs。
- [ ] 新增 `web/src/components/opsgraph/opsgraphComponents.test.tsx`，覆盖 Search、Impact、RCA、Asset、Patch Review、Detail Drawer。
- [ ] 新增 `web/src/components/opsgraph/opsgraphViewModels.test.ts`，覆盖边质量计算、低置信边、候选路径排序、资产禁用规则。
- [ ] 运行 `npm --prefix web test -- opsgraph.test.ts opsgraphViewModels.test.ts opsgraphComponents.test.tsx`。
- [ ] 运行 `npm --prefix web test`。
- [ ] 运行 `npm --prefix web run build`。
- [ ] 如果本轮包含页面视觉变更，启动 dev server 并用浏览器截图检查 `/opsgraph` 的桌面和移动宽度。
- [ ] 检查 `/opsgraph?entityId=...&tab=impact`、`/opsgraph?traceId=...&tab=root-cause`、`/opsgraph?patchId=...&tab=patch-review` 能正确落点。

## 14. 交付检查

- [ ] `OpsGraphPage.tsx` 不直接调用裸 `fetch`。
- [ ] `complexPagesApi.ts` 不继续扩展 OpsGraph 新 API。
- [ ] 图谱边在所有视图中都能展示 source、confidence、updatedAt、validFrom、validTo、evidenceRefs。
- [ ] 低置信边在视觉上明确区分，不能作为自动修复建议的唯一依据。
- [ ] Root Cause Path 未确认前只显示为候选路径。
- [ ] ERP 租户、订单、用户、请求 payload、cookie、token、password、用户输入原文不会出现在 DOM。
- [ ] Patch Review 无审核意见不能提交。
- [ ] 高风险 patch 审核前能看到受影响业务能力和证据质量。
- [ ] Case、ERP Health、Debug Trace、Experience Pack Review 能复用 OpsGraph 摘要组件或跳转到 `/opsgraph`。
- [ ] `npm --prefix web test` 通过。
- [ ] `npm --prefix web run build` 通过。
