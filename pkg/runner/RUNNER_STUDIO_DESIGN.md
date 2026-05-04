# Runner Studio 主应用一体化设计方案

> 状态：已确认
> 适用范围：`http://127.0.0.1:8080/runner`
> 关系说明：本方案补充并收敛现有 `VISUAL_WORKFLOW_UI_DESIGN.md`，重点解决主应用入口分裂、页面过重、参数输入输出难用的问题。

## 1. 目标

`/runner` 不再是跳板页，也不再要求用户感知独立 `:8090` 服务。最终交付的是主应用内原生一体化的 Runner Studio，能力包括：

- 工作流选择、创建、克隆、导入、归档、版本管理。
- Dify 风格的可视化拖拽编排。
- 简洁主界面，复杂配置通过弹窗和抽屉展开。
- 结构化参数输入输出编辑，不依赖大段 YAML 文本。
- AI 辅助创建、补参数、解释失败原因。
- 运行日志、变量检查、审批轨迹、审计记录闭环。

## 2. 非目标

- 不复刻 Dify 全量产品形态，不引入与 Runner 无关的 LLM App Builder 概念。
- 不维护第二套执行 DSL。`graph` 是唯一编排语义，YAML 是兼容视图与导入导出格式。
- 不保留“主应用入口页 + 外部 Runner UI”这套长期产品形态。

## 3. 现状问题

当前实现存在四个核心问题：

1. `/runner` 仍是入口壳页，真正的拖拽编排能力分散在 `pkg/runner/server/ui/frontend/`，用户路径断裂。
2. 现有 workbench 顶栏和常驻面板过重，第一次使用时不清楚主操作路径。
3. 参数配置仍偏 YAML/自由文本，不适合生产级维护，也不利于 AI 辅助修复。
4. 运行态、变量态、审批态、版本发布态没有在主应用内形成一个统一工作台。

## 4. 设计原则

### 4.1 一个页面，一套产品边界

用户进入 `/runner` 后，应直接完成完整主流程：

`选择工作流 -> 拖拽编排 -> 配置参数 -> 校验 -> Dry Run -> 发布 -> 运行 -> 查看结果`

### 4.2 主界面只保留高频动作

主页面只承载三件事：

- 选工作流
- 编排工作流
- 运行工作流

所有低频复杂动作都进入弹窗或抽屉：

- 工作流管理
- 节点高级配置
- YAML / diff
- 发布审阅
- AI 修复说明
- 变量检查和日志

### 4.3 参数输入输出结构化

不再把参数配置设计成“写大段 YAML”。节点配置默认使用结构化字段编辑器，支持类型、变量引用、表达式、提取规则和字段级校验。

### 4.4 AI 只生成和修改草稿

AI 生成或 AI 修改结果一律落到 `draft`。允许 `dry-run`，不允许直接 `publish`。

## 5. 页面信息架构

## 5.1 顶栏

顶栏只保留高频操作：

- 当前工作流名称、版本、状态。
- `新建`
- `导入 YAML`
- `AI 生成`
- `保存草稿`
- `校验`
- `Dry Run`
- `运行`
- `发布`

顶栏不常驻展示复杂状态复选框、长文本说明或大块输入框。

## 5.2 左侧栏

左侧栏只保留三块内容：

- 最近使用 / 收藏工作流
- 节点库
- 模板片段

完整工作流管理不在左侧展开，而是进入“工作流管理”弹窗。

## 5.3 中央画布

中央区域是编排主视图，只负责：

- 拖拽节点
- 连线
- 缩放 / 小地图 / 自动布局
- 节点状态展示

交互约束：

- 单击节点：显示摘要
- 双击节点：打开节点配置弹窗
- 右键节点：打开节点动作菜单

## 5.4 底部抽屉

底部使用双抽屉，而不是常驻右侧面板：

- 运行日志抽屉：stdout / stderr / SSE / 审批事件 / 重试轨迹
- 变量检查抽屉：输入变量、输出变量、运行态导出变量、最近一次节点结果

默认收起，运行时可自动展开。

## 5.5 弹窗体系

复杂能力统一进弹窗：

- 工作流管理弹窗
- 节点配置弹窗
- YAML / diff 弹窗
- 发布审阅弹窗
- AI 助手弹窗
- 节点运行详情弹窗

## 6. 核心用户流程

## 6.1 新建工作流

工作流创建入口固定为五种：

- 空白工作流
- 模板创建
- 从 YAML 导入
- 克隆现有
- AI 生成

AI 创建的结果直接形成 `draft graph`，并附带生成说明与 diff 摘要。

## 6.2 拖拽编排

左侧节点库支持拖拽到画布。首版正式支持的节点能力包括：

- `start`
- `end`
- `action`
- `condition`
- `parallel`
- `join`
- `manual_approval`
- `subflow`
- `loop`
- `handler`

模板片段用于拖入常见组合：

- 审批后执行
- 失败回滚
- 并行恢复
- 健康检查
- 人工确认后切流

## 6.3 节点配置

节点配置弹窗固定为五个页签：

- `基础`
- `输入`
- `输出`
- `高级`
- `运行与 AI`

### 基础

用于编辑：

- `name`
- `action`
- `targets`
- `timeout`
- `retries`

### 输入

结构化参数列表编辑，不让用户优先写 YAML。

### 输出

显式声明节点输出变量及提取规则。

### 高级

默认折叠，放低频复杂项：

- `when`
- `continue_on_error`
- `failure edge`
- `env`
- `workdir`
- `join strategy`
- `loop config`
- `subflow binding`

### 运行与 AI

展示：

- 最近一次试跑结果
- 当前节点校验问题
- AI 补参数建议
- 变更 diff

## 6.4 发布与运行

工作流生命周期统一为：

`draft -> validated -> published`

发布流程：

1. 保存草稿
2. 校验
3. Dry Run
4. 打开发布审阅弹窗
5. 填写发布说明
6. 发布

运行流程：

- 已发布工作流可直接运行
- `draft` 工作流可 `dry-run`
- AI 产物不可直接发布

## 7. 参数输入输出模型

## 7.1 输入参数模型

每个输入参数是一行，字段固定：

- `key`
- `label`
- `type`
- `value_source`
- `value`
- `required`
- `description`

`value_source` 仅允许三种：

- `constant`
- `variable_reference`
- `expression`

## 7.2 输出参数模型

每个输出参数是一行，字段固定：

- `key`
- `type`
- `extract_source`
- `extract_rule`
- `description`

`extract_source` 首版支持：

- `stdout_text`
- `stdout_jsonpath`
- `stderr_text`
- `exit_code`
- `export_var`
- `approval_result`
- `subflow_output`

## 7.3 变量引用

参考 Dify 的变量引用交互，Runner 必须提供变量选择器，而不是要求用户手写路径。

能力要求：

- 可浏览可用变量来源。
- 可区分系统变量、工作流变量、上游节点输出变量。
- 可显示变量类型。
- 可跳转到变量来源节点。

## 7.4 混合变量文本

命令、脚本文本、通知文案等字段支持混合变量文本，例如：

```text
echo {{ vars.backup_id }}
```

界面里变量片段需要高亮显示。

## 7.5 类型系统

首版正式支持：

- `string`
- `number`
- `boolean`
- `object`
- `array[string]`
- `duration`
- `host_list`
- `env_map`

## 7.6 校验规则

输入输出字段级校验必须覆盖：

- 重名校验
- 空值校验
- 类型校验
- 变量引用可达性校验
- 提取规则合法性校验

## 8. AI 辅助策略

AI 入口有三类：

- 页面级：自然语言生成工作流
- 节点级：补齐参数、修复校验问题
- 运行后：解释失败原因和下一步建议

约束：

- AI 输出必须是结构化草稿和 diff。
- AI 不得静默修改已发布版本。
- AI 修改必须可审计。

## 9. 运行态与审计

## 9.1 运行态

运行态分三层展示：

- 画布层：节点状态、边高亮、运行动画、失败角标
- 摘要层：节点最近一次运行摘要
- 详情层：日志、变量、审批、重试和错误信息

## 9.2 单节点试跑

允许试跑的节点：

- `action`
- `condition`
- `subflow`

不允许脱离上下文直接试跑的控制节点：

- `join`
- `loop`
- `manual_approval`

## 9.3 审计

以下动作必须记录审计：

- 创建工作流
- 删除 / 归档
- 修改节点
- 导入 YAML
- AI 生成
- AI 修复
- 发布
- 运行
- 审批

审计字段至少包括：

- `actor`
- `time`
- `action`
- `diff`

## 10. 架构边界

## 10.1 前端边界

主应用前端最终收敛为六个稳定单元：

- `runner shell`
- `workflow manager modal`
- `workflow canvas`
- `node config modal`
- `run drawers`
- `ai assistant`

## 10.2 后端边界

后端能力保持单一职责：

- `workflow graph service`
- `catalog/schema service`
- `run/approval service`
- `audit/version service`

## 10.3 模型边界

- `graph` 是唯一执行和编排语义
- `YAML` 是兼容视图
- 不维护第二套编排 DSL

## 11. Dify 参考点

本方案参考了 Dify 的交互模式，而非照抄产品功能：

- 工作流顶栏的轻量操作分层：
  [header-in-normal.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/header/header-in-normal.tsx)
- 节点选中时按需出现的面板体系：
  [panel/index.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/panel/index.tsx)
- 变量检查底部面板：
  [variable-inspect/index.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/variable-inspect/index.tsx)
- Start / End 节点输入输出列表设计：
  [start/panel.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/nodes/start/panel.tsx)
  [end/panel.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/nodes/end/panel.tsx)
- 变量引用选择器：
  [var-reference-picker.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/nodes/_base/components/variable/var-reference-picker.tsx)
- 支持变量高亮的文本输入：
  [support-var-input/index.tsx](/Users/lizhongxuan/Desktop/aiops/dify/web/app/components/workflow/nodes/_base/components/support-var-input/index.tsx)

## 12. 验收标准

设计完成后，产品级验收必须满足：

- 新用户进入 `/runner` 后，不看文档也能完成：
  `新建工作流 -> 拖 3 个节点 -> 配输入输出 -> 校验 -> dry-run`
- 不再出现“页面入口存在，但不知道如何拖拽或配置”的状态。
- 参数输入输出不再依赖大段 YAML 文本编辑。
- AI 结果可审计、可回滚、不可越过发布门禁。
