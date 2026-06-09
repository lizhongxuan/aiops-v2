# 实施计划：Chat 对话过程 UX 优化

## 概述

基于 Codex-style 纯文本时间线设计，分后端传输层改造、前端组件重构、停止按钮修复三条主线推进。每条主线内部按增量方式构建，确保每一步都可验证。最终通过 Playwright 快照测试锁定视觉基线。

## 任务

- [x] 1. 后端：Transport Projector HTML 过滤
  - [x] 1.1 在 `internal/appui/transport_projector.go` 中新增 `isHTMLContent` 函数
    - 检测字符串是否以 `<!DOCTYPE`、`<html`、`<HTML` 开头
    - 支持前导空白字符
    - _需求: 14.1_

  - [x] 1.2 在 `internal/appui/transport_projector.go` 中新增 `stripHTMLTags` 函数
    - 使用正则移除所有 HTML 标签
    - 保留标签间的文本内容
    - 合并多余空白为单个空格
    - _需求: 14.2, 14.5_

  - [x] 1.3 在 `internal/appui/transport_projector.go` 中新增 `sanitizeOutputPreview` 函数
    - 调用 `isHTMLContent` 检测
    - HTML 内容：调用 `stripHTMLTags` 后截断至 200 字符并追加 "…"
    - 非 HTML 内容：截断至 500 字符
    - _需求: 14.1, 14.2_

  - [x] 1.4 在 `projectTurnItem` 的 `TurnItemTypeToolCall`/`TurnItemTypeToolResult` 分支中应用 `sanitizeOutputPreview`
    - 对 `block.OutputPreview` 和 `block.Text` 字段调用过滤
    - 确保 search 类型的 query 文本不被误过滤
    - _需求: 14.1, 14.4, 15.1_

  - [x] 1.5 在 `internal/appui/transport_projector_test.go` 中新增 HTML 过滤测试
    - 测试 `isHTMLContent` 对各种 HTML 开头的检测
    - 测试 `sanitizeOutputPreview` 对 HTML 内容的剥离和截断
    - 测试非 HTML 内容不被误处理
    - 测试 `projectTurnItem` 输出的 block 不含原始 HTML
    - _需求: 14.1, 14.2, 14.5_

- [x] 2. 后端：推理流实时持久化
  - [x] 2.1 在 `internal/runtimekernel/eino_kernel.go` 的 `runHostIterationLoop` 中修改 `onReasoning` 回调
    - 每收到 reasoning delta，更新 TurnSnapshot 中 reasoning AgentItem 的 text 字段（增量追加）
    - 更新 AgentItem 的 status 为 `ItemStatusRunning`
    - _需求: 12.1_

  - [x] 2.2 在 `onReasoning` 回调中实现 persistTurnSnapshot 节流逻辑
    - 维护 `lastReasoningPersist time.Time` 变量
    - 仅当距上次持久化 ≥ 100ms 时调用 `persistTurnSnapshot`
    - 确保 TurnSnapshot.UpdatedAt 被更新以触发 fingerprint 变化
    - _需求: 12.2, 12.3, 12.4_

  - [x] 2.3 确保模型调用完成时 reasoning AgentItem 状态更新为 completed
    - 在 `generateModelResponse` 返回后，将 reasoning item status 设为 `ItemStatusCompleted`
    - 执行最终一次 `persistTurnSnapshot`
    - _需求: 12.6_

  - [x] 2.4 在 `internal/runtimekernel/eino_kernel_test.go` 中新增推理流持久化测试
    - 验证 reasoning delta 更新 AgentItem text
    - 验证节流逻辑：100ms 内多次 delta 仅触发一次 persist
    - 验证完成后 status 变为 completed
    - _需求: 12.1, 12.2, 12.6_

- [x] 3. 后端：停止按钮修复
  - [x] 3.1 在 `internal/runtimekernel/eino_kernel.go` 中确保 `requestTurnCancel` 立即调用已注册的 cancel 函数
    - 检查 `inFlightTurnCancel` 映射中是否存在对应 turn 的 cancel func
    - 若存在则立即调用，取消 `runCtx`
    - 若不存在则写入 `pendingTurnCancel` 映射
    - _需求: 13.2, 13.3_

  - [x] 3.2 在 `generateModelResponse` 中检测 `context.Canceled` 错误并快速返回
    - `chatModel.Stream()` 使用 `runCtx`，context 取消后 `stream.Recv()` 返回 error
    - 检测到 `context.Canceled` 或 `context.DeadlineExceeded` 时立即退出循环
    - 返回适当的取消状态
    - _需求: 13.2, 13.4_

  - [x] 3.3 确保 Turn 进入模型调用阶段时检查 `pendingTurnCancel`
    - 在 `generateModelResponse` 开始前检查是否有待处理的取消请求
    - 若有则跳过模型调用，直接标记 Turn 为 canceled
    - _需求: 13.3_

  - [x] 3.4 在 `internal/runtimekernel/eino_kernel_test.go` 中新增停止传播测试
    - 验证 cancel 函数被立即调用
    - 验证 pending cancel 在 Turn 进入模型调用时被执行
    - 验证 context 取消后 stream 循环退出
    - _需求: 13.1, 13.2, 13.3_

- [x] 4. 检查点 — 确保后端测试通过
  - 运行 `go test ./internal/appui/... ./internal/runtimekernel/...` 确保所有测试通过
  - 如有问题请询问用户

- [x] 5. 前端：ProcessTranscript 重构为 Codex 时间线
  - [x] 5.1 重构 `web/src/chat/components/ProcessTranscript.tsx` 移除折叠面板包裹
    - 移除外层 collapsible button 和 ChevronDown 切换逻辑
    - 所有 process blocks 直接展示在消息区域内，无需手动展开
    - 保留 `data-testid="aiops-process-transcript"` 用于测试
    - _需求: 5.4, 1.3_

  - [x] 5.2 实现 Thinking_Text 渲染组件
    - 对 kind 为 `reasoning` 的 block，渲染为黑色加粗纯文本（font-weight: 500, color: slate-900, text-[15px], leading-7）
    - 无卡片/边框/阴影/背景色
    - 支持流式文本追加渲染
    - 过滤 "calling model" 文本，替换为 "正在思考"
    - _需求: 1.1, 1.2, 1.3, 1.6, 11.3, 11.4_

  - [x] 5.3 实现 Tool_Summary_Line 渲染组件
    - 对 kind 为 `command`/`tool`/`file`/`search`/`mcp` 的 block，渲染为灰色单行文本（color: slate-400/500）
    - 根据类型添加 emoji 图标前缀：🔍 搜索、⚙️ 命令、✏️ 文件编辑、📂 文件探索
    - 单行展示，总长度超过 80 字符时截断加省略号
    - 无卡片/边框/Badge 包裹
    - _需求: 2.1, 2.2, 2.3, 2.4, 2.5, 15.1, 15.2_

  - [x] 5.4 实现同类工具操作合并逻辑
    - 连续相同 kind 的工具类 block 合并为一行摘要
    - 显示数量统计，如 "📂 已探索 6 个文件，3 次搜索"
    - 不跨越 reasoning block 合并
    - _需求: 3.1, 3.2, 3.4_

  - [x] 5.5 实现搜索结果可展开详情
    - 搜索类型的 Tool_Summary_Line 行尾显示展开箭头
    - 点击展开显示 URL 列表（灰色文本，每 URL 一行）
    - 再次点击收起
    - 添加 `aria-expanded` 属性
    - _需求: 7.1, 7.2, 7.3, 7.4, 2.6, 3.3_

  - [x] 5.6 添加可访问性属性
    - Process_Timeline 添加 `aria-live="polite"`
    - 展开/折叠按钮支持键盘操作（Enter/Space）
    - Thinking_Status 动画遵循 `prefers-reduced-motion`
    - _需求: 8.2, 8.3, 8.4, 8.5_

- [x] 6. 前端：即时反馈与状态指示器
  - [x] 6.1 实现 "正在思考" 占位状态
    - 当 `turnStatus === "working"` 且无可见 blocks 时，显示 "正在思考" + 呼吸动画
    - 当首个有效 reasoning 文本到达时，替换占位动画为实际内容
    - _需求: 11.1, 11.5, 6.1_

  - [x] 6.2 实现状态指示器切换逻辑
    - 模型生成思考文本时显示 "正在思考" + 旋转图标
    - 执行工具调用时更新为 "正在执行"
    - Turn 完成时隐藏指示器
    - 内联显示在时间线当前活动步骤位置
    - _需求: 6.1, 6.2, 6.3, 6.4_

  - [x] 6.3 确保流式内容平滑渲染
    - 新内容以自然文本流入方式渲染，不使用复杂动画
    - 不导致页面跳动或闪烁
    - 自动滚动到最新内容
    - _需求: 5.1, 5.2, 5.3, 1.4_

- [x] 7. 前端：HTML 过滤兜底
  - [x] 7.1 在 `web/src/chat/components/ProcessTranscript.tsx` 中新增 `stripHtml` 工具函数
    - 检测 HTML 内容（DOCTYPE/html 标签开头）
    - 移除所有 HTML 标签，合并空白，截断至 200 字符
    - 非 HTML 内容原样返回
    - _需求: 14.3, 14.4_

  - [x] 7.2 在 `readableBlockSummary` 和 `NativeProcessText` 中应用 `stripHtml`
    - 对 `block.text`、`block.outputPreview` 字段调用过滤
    - 确保即使后端未完全过滤，前端也不渲染原始 HTML
    - _需求: 14.3, 15.5_

- [x] 8. 前端：停止按钮 UX 优化
  - [x] 8.1 实现停止按钮 "正在停止" 状态
    - 用户点击 Stop 后立即将按钮文本更新为 "正在停止"
    - 按钮进入 disabled 状态
    - 禁用 composer 输入
    - _需求: 13.5_

  - [x] 8.2 实现 2 秒超时强制停止逻辑
    - 点击停止后启动 2s 计时器
    - 若 2s 内未收到 `turnStatus === "canceled"` 状态，显示 "强制停止" 按钮
    - 收到 canceled 状态后过渡到已停止 UI
    - _需求: 13.1, 13.5_

- [x] 9. 检查点 — 确保前端编译通过
  - 运行 `cd web && npm run build` 确保无编译错误
  - 如有问题请询问用户

- [x] 10. 前端：Playwright 快照测试
  - [x] 10.1 在 `web/tests/e2e/chat-native-process-snapshot.spec.js` 中新增/更新 fixture 数据
    - 通过 `web/tests/helpers/uiFixtureHarness.js` 提供确定性测试数据
    - 覆盖状态：思考文本流式中、工具摘要行已完成、搜索结果已展开、最终回复已渲染
    - _需求: 9.1, 9.2, 9.3_

  - [x] 10.2 新增快照测试用例覆盖 Codex 时间线各状态
    - 思考文本流式中状态快照
    - 工具摘要行（含合并）已完成状态快照
    - 搜索结果展开状态快照
    - 即时反馈 "正在思考" 占位状态快照
    - 停止按钮 "正在停止" 状态快照
    - _需求: 9.3, 9.4_

  - [x] 10.3 新增前端组件单元测试
    - `stripHtml` 函数测试：HTML 检测、标签剥离、截断
    - 合并逻辑测试：连续同类 block 合并、不跨 reasoning 合并
    - 即时反馈测试：无 blocks 时显示占位、有效文本到达后替换
    - _需求: 14.3, 3.1, 11.1_

- [x] 11. 最终检查点 — 确保所有测试通过
  - 运行 `go test ./internal/appui/... ./internal/runtimekernel/...`
  - 运行 `cd web && npm run build`
  - 运行 `cd web && npm run test:ui:snapshots`
  - 确保所有测试通过，如有问题请询问用户

## 备注

- 标记 `*` 的子任务为可选，可跳过以加速 MVP 交付
- 每个任务引用了具体的需求编号以确保可追溯性
- 检查点任务确保增量验证
- 所有实现遵循现有 AssistantTransport 架构，不引入被禁止的遗留模式（需求 10）
- 前端改动需遵循 AGENTS.md 中的 UI Snapshot Coverage 规则
