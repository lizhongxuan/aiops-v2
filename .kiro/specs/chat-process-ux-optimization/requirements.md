# 需求文档：Chat 对话过程 UX 优化

## 简介

优化 AI Chat 的对话过程展示体验，参考 OpenAI Codex 原生应用的交互风格。核心设计理念：**纯文本时间线**——思考文本（thinking）和工具调用摘要（tool summaries）以线性时间线交替排列，无卡片/边框包裹，保持极简美学。最终回复紧随时间线之后，以变更摘要行收尾。

## 术语表

- **Process_Timeline**：对话过程时间线区域，以线性列表展示 AI 在生成最终回复前的所有中间步骤（思考文本 + 工具摘要行交替排列）
- **Thinking_Text**：思考文本行，黑色加粗纯文本，直接陈述 AI 正在做什么及原因，无卡片/边框包裹
- **Tool_Summary_Line**：工具调用摘要行，灰色单行文本带图标前缀，紧凑展示工具操作结果
- **Final_Response**：最终回复区域，展示 AI 给用户的最终答案内容
- **Change_Summary**：变更摘要行，展示本次操作的文件变更统计（如"4 个文件已更改 +105 -12"）
- **Thinking_Status**：正在思考状态指示器，在模型生成思考文本时显示
- **AiopsProcessBlock**：传输层中表示单个过程块的数据结构，包含 kind、status、text 等字段
- **AiopsTransportState**：传输层状态对象，承载会话的完整结构化流式数据
- **Turn**：一次完整的用户提问 → AI 回复的交互轮次
- **Streaming**：流式传输，服务端逐步推送数据、前端实时渲染的模式
- **Transport_Projector**：传输投影器，负责将 TurnSnapshot 中的 AgentItems 转换为 AiopsTransportState 结构
- **TurnSnapshot**：轮次快照，后端维护的当前 Turn 完整状态，包含 AgentItems、Lifecycle、FinalOutput 等
- **EventReasoningSummaryDelta**：推理摘要增量事件，模型在生成过程中产生的思考文本流式片段
- **assistantTransportTurnFingerprint**：传输轮次指纹，用于检测 TurnSnapshot 是否发生变化的哈希字符串
- **persistTurnSnapshot**：持久化轮次快照操作，将 TurnSnapshot 变更写入 SessionState 并触发 fingerprint 更新
- **Stop_Button**：停止按钮，用户在 Turn 运行期间点击以终止当前模型调用和工具执行
- **inFlightTurnCancel**：运行时内部维护的活跃 Turn 取消函数映射，key 为 sessionID+turnID
- **HTML_Sanitization**：HTML 过滤/清洗，将原始 HTML 内容转换为安全的纯文本摘要

## 需求

### 需求 1：思考文本纯文本时间线展示

**用户故事：** 作为用户，我希望在 AI 思考时能实时看到思考内容以纯文本形式流式输出在时间线中，以便了解 AI 正在分析什么问题、采用什么思路。

#### 验收标准

1. WHILE Turn 处于 working 状态且存在 kind 为 reasoning 的 AiopsProcessBlock，THE Thinking_Text SHALL 以流式方式逐步渲染该 block 的 text 内容
2. THE Thinking_Text SHALL 使用黑色加粗纯文本样式（font-weight: medium/500+，color: slate-900，font-size: 15px，line-height: 7）直接渲染在时间线中
3. THE Thinking_Text SHALL 不使用任何卡片、边框、阴影或背景色包裹——仅为时间线中的一个纯文本行
4. WHILE 新的思考文本持续到达，THE Process_Timeline SHALL 自动滚动到最新内容位置
5. WHEN Turn 状态从 working 变为 completed，THE Thinking_Text SHALL 停止流式渲染并保持最终状态可查看
6. THE Thinking_Text 内容风格 SHALL 为直接陈述 AI 正在做什么及原因（如"我会先核对文档和项目约束，然后直接改现有集成"），不使用第三人称描述

### 需求 2：工具调用单行灰色摘要展示

**用户故事：** 作为用户，我希望工具调用以紧凑的单行灰色摘要形式展示在时间线中，以便在不占用过多屏幕空间的前提下了解 AI 执行了哪些操作。

#### 验收标准

1. WHEN 一个 kind 为 command、tool、file、search 或 mcp 的 AiopsProcessBlock 完成时，THE Tool_Summary_Line SHALL 以单行灰色文本（color: slate-400/500）渲染在时间线中
2. THE Tool_Summary_Line SHALL 以图标前缀开头，根据操作类型使用对应图标：🔍 搜索、⚙️ 命令执行、✏️ 文件编辑、📂 文件探索
3. THE Tool_Summary_Line SHALL 不使用任何卡片、边框、阴影、Badge 或背景色包裹——仅为时间线中的一个纯文本行
4. THE Tool_Summary_Line SHALL 在单行内展示操作摘要文本，格式如："🔍 已搜索网页 3 次"、"⚙️ 已运行 curl -L ..."、"✏️ 已编辑 1 个文件"、"📂 已探索 2 个文件"
5. WHEN Tool_Summary_Line 包含命令文本时，THE Tool_Summary_Line SHALL 在摘要中内联显示命令内容（过长时截断并加省略号）
6. WHEN Tool_Summary_Line 为搜索类型时，THE Tool_Summary_Line SHALL 支持点击展开显示搜索结果 URL 列表（带展开箭头指示）
7. WHILE AiopsProcessBlock 的 status 为 running，THE Tool_Summary_Line SHALL 显示为进行中状态文本（如"正在搜索网页..."）

### 需求 3：同类工具操作合并摘要

**用户故事：** 作为用户，我希望连续的同类型工具操作被合并为一行摘要，以便时间线保持简洁不冗长。

#### 验收标准

1. WHEN 同一 Turn 中存在连续多个相同 kind 的工具类 AiopsProcessBlock，THE Process_Timeline SHALL 将它们合并为一行 Tool_Summary_Line
2. THE 合并后的 Tool_Summary_Line SHALL 在摘要中显示各操作的数量统计，格式如："📂 已探索 6 个文件，3 次搜索，1 个列表"
3. WHEN 合并摘要行包含搜索操作时，THE Tool_Summary_Line SHALL 支持点击展开显示所有搜索结果的 URL 列表
4. THE Process_Timeline SHALL 按 AiopsProcessBlock 的接收时间顺序决定合并范围——仅合并时间上连续的同类操作，不跨越 Thinking_Text 合并

### 需求 4：最终回复与变更摘要

**用户故事：** 作为用户，我希望 AI 的最终回复紧随时间线之后展示，并以变更摘要行总结本次操作结果，以便快速了解最终成果。

#### 验收标准

1. THE Final_Response SHALL 在 Process_Timeline 下方渲染，保持从上到下"思考/工具时间线 → 最终回复"的阅读顺序
2. THE Final_Response SHALL 使用标准文本样式（15px、line-height: 7、color: slate-900）作为消息主体内容呈现
3. WHEN Turn 包含 process 块但尚未产生 final 文本时，THE Final_Response 区域 SHALL 不渲染任何占位内容
4. WHEN Turn 中存在文件变更操作时，THE Change_Summary SHALL 在时间线末尾（Final_Response 之前）显示变更统计行，格式如："4 个文件已更改 +105 -12"
5. THE Change_Summary SHALL 使用与 Tool_Summary_Line 一致的灰色文本样式

### 需求 5：流式文本与平滑过渡

**用户故事：** 作为用户，我希望时间线中的内容出现是平滑自然的，不产生页面跳动。

#### 验收标准

1. WHEN 新的 Thinking_Text 或 Tool_Summary_Line 出现在时间线中时，THE Process_Timeline SHALL 以自然文本流入的方式渲染，不使用复杂的淡入/滑入动画
2. THE Process_Timeline 中新内容的出现 SHALL 不导致页面内容跳动或闪烁
3. WHILE Thinking_Text 正在流式输出时，THE Thinking_Text SHALL 以逐字/逐词流式渲染，模拟打字效果
4. THE Process_Timeline SHALL 不使用整体折叠面板包裹——所有步骤直接展示在消息区域内，无需用户手动展开即可查看

### 需求 6：状态指示器

**用户故事：** 作为用户，我希望通过"正在思考"状态指示器了解 AI 当前的工作状态。

#### 验收标准

1. WHILE Turn 处于 working 状态且模型正在生成思考文本，THE Thinking_Status SHALL 显示"正在思考"状态指示器（旋转图标 + 文字）
2. WHEN 模型开始执行工具调用时，THE Thinking_Status SHALL 更新为"正在执行"状态
3. WHEN Turn 状态变为 completed，THE Thinking_Status SHALL 隐藏或替换为完成状态标记
4. THE Thinking_Status SHALL 以内联方式显示在时间线当前活动步骤的位置，不使用固定顶部栏

### 需求 7：搜索结果可展开详情

**用户故事：** 作为用户，我希望搜索操作的结果 URL 可以按需展开查看，以便了解 AI 查阅了哪些信息来源。

#### 验收标准

1. WHEN Tool_Summary_Line 为搜索类型且包含 results 数据时，THE Tool_Summary_Line SHALL 在行尾显示展开箭头（chevron）指示可展开
2. WHEN 用户点击搜索类型的 Tool_Summary_Line 时，THE Process_Timeline SHALL 在该行下方展开显示搜索结果 URL 列表
3. THE 展开的搜索结果列表 SHALL 使用与 Tool_Summary_Line 一致的灰色文本样式（slate-400），每个 URL 占一行
4. WHEN 用户再次点击时，THE 搜索结果列表 SHALL 收起折叠

### 需求 8：响应式布局与可访问性

**用户故事：** 作为用户，我希望优化后的 UI 在不同屏幕尺寸下都能正常使用，且对辅助技术友好。

#### 验收标准

1. THE Process_Timeline SHALL 在视口宽度 ≥ 768px 和 < 768px 两种情况下均保持可读性和可操作性
2. THE 搜索结果展开/折叠按钮 SHALL 具有 aria-expanded 属性，正确反映当前展开状态
3. THE Process_Timeline SHALL 具有 aria-live="polite" 属性，使屏幕阅读器能感知流式内容更新
4. THE Process_Timeline 中所有可交互元素 SHALL 支持键盘操作（Enter/Space 触发展开/折叠）
5. THE Thinking_Status 动画 SHALL 遵循 prefers-reduced-motion 媒体查询，在用户偏好减少动画时使用静态替代

### 需求 9：Playwright 快照测试覆盖

**用户故事：** 作为开发者，我希望所有 UI 变更都有 Playwright 快照测试覆盖，以便在后续修改中检测到意外的视觉回归。

#### 验收标准

1. WHEN 对 Process_Timeline、Thinking_Text、Tool_Summary_Line 或 Final_Response 组件进行 UI 变更时，THE 测试套件 SHALL 包含对应的 Playwright 快照测试用例
2. THE 快照测试 SHALL 通过 fixture-driven 方式（使用 uiFixtureHarness）提供确定性的测试数据，不依赖实时 LLM 调用
3. THE 快照测试 SHALL 覆盖以下状态：思考文本流式中、工具摘要行已完成、搜索结果已展开、变更摘要行已渲染、最终回复已渲染
4. THE 快照测试 SHALL 在 `npm run test:ui:snapshots` 命令下通过

### 需求 10：传输层兼容性约束

**用户故事：** 作为开发者，我希望 UI 优化完全基于现有 AiopsTransportState 结构实现，不引入被禁止的遗留模式，以保持架构一致性。

#### 验收标准

1. THE 实现 SHALL 仅通过 AiopsTransportState 和 AssistantTransport state ops 获取过程数据，不引入 StructuredResponsePatch、emit_response_events 或 legacy agent_event reducers
2. THE 实现 SHALL 不添加 page-local SSE/WebSocket 流、AgentEventProjection selectors 或 codexProcessTranscript 模式
3. THE 实现 SHALL 不从 assistant final text 中解析 Markdown heading 来派生 process UI
4. IF AiopsTransportState.schemaVersion 需要变更，THEN THE 变更 SHALL 同步更新 transport types、runtime/converter tests、snapshot fixtures 和 README guardrails

### 需求 11：思考阶段即时反馈

**用户故事：** 作为用户，我希望在发送请求后能立即看到"正在思考"的状态反馈，而不是长时间停留在无内容的"处理中"状态，以便确认系统正在正常工作。

#### 验收标准

1. WHEN Turn 进入 working 状态且当前无可见的 AiopsProcessBlock 时，THE Process_Timeline SHALL 立即显示"正在思考..."占位文本并附带呼吸动画指示器
2. WHEN reasoning 类型的流式增量文本（EventReasoningSummaryDelta）到达时，THE Process_Timeline SHALL 在 100ms 内将增量文本渲染到对应的 reasoning AiopsProcessBlock 中
3. THE Process_Timeline SHALL 不向用户展示"calling model"内部状态文本；WHEN AiopsProcessBlock 的 text 为"calling model"时，THE Process_Timeline SHALL 将其替换为"正在思考..."用户友好文本
4. WHILE Turn 处于 working 状态且仅存在 text 为"calling model"的 reasoning AiopsProcessBlock 时，THE Process_Timeline SHALL 将该 block 视为"正在等待模型响应"并显示思考占位动画
5. WHEN 首个有效 reasoning 文本（非"calling model"）到达时，THE 思考占位动画 SHALL 被替换为实际的 Thinking_Text 流式内容

### 需求 12：推理流实时传输至 Transport 状态

**用户故事：** 作为用户，我希望模型的推理文本能实时流式传输到前端，而不是等待整个模型调用完成后才一次性出现，以便获得流畅的思考过程可视化体验。

#### 验收标准

1. WHEN 模型产生 reasoning delta 事件（EventReasoningSummaryDelta）时，THE Transport_Projector SHALL 将增量文本实时追加到对应 Turn 的 reasoning AiopsProcessBlock.text 字段中
2. WHEN reasoning delta 更新了 AiopsProcessBlock.text 时，THE TurnSnapshot SHALL 更新其 UpdatedAt 时间戳，使 assistantTransportTurnFingerprint 产生变化
3. THE 传输轮询循环（streamAssistantTransportState）SHALL 通过 fingerprint 变化检测到 reasoning 文本更新，并将增量内容通过 state ops 推送至前端
4. THE reasoning 文本从 LLM 产生到前端渲染的端到端延迟 SHALL 不超过 100ms（不含网络传输时间）
5. THE 实现 SHALL 遵循现有 AssistantTransport 结构化流式架构——通过更新 TurnSnapshot 中的 AgentItems 并触发 persistTurnSnapshot 来反映 reasoning 增量，不引入独立的 SSE/WebSocket 旁路通道
6. WHEN 模型调用完成时，THE reasoning AiopsProcessBlock 的 status SHALL 从 running 更新为 completed，且 text 字段包含完整的推理摘要文本

### 需求 13：停止按钮可靠终止当前 Turn

**用户故事：** 作为用户，我希望在 AI 处理过程中点击停止按钮能可靠地终止当前操作，而不是按钮无响应或操作继续运行，以便在不需要继续等待时能立即中断。

#### 验收标准

1. WHEN 用户点击 Stop_Button 时，THE 系统 SHALL 在 2 秒内完成当前 Turn 的取消操作，无论模型是否正在流式输出
2. WHEN Stop_Button 被点击时，THE CancelTurn 操作 SHALL 通过 inFlightTurnCancel 映射中注册的 cancel 函数立即调用 context.Cancel()，终止活跃的 chatModel.Stream() goroutine
3. IF inFlightTurnCancel 中尚未注册对应 Turn 的 cancel 函数（Turn 尚未进入模型调用阶段），THEN THE 系统 SHALL 将取消请求存入 pendingTurnCancel 映射，并在 Turn 进入模型调用时立即检查并执行取消
4. WHEN context 被取消后，THE streamAssistantTransportState 轮询循环 SHALL 通过 TurnSnapshot.Lifecycle 变为 TurnLifecycleCanceled 检测到终止状态，并立即结束轮询返回最终状态
5. THE 前端 SHALL 在用户点击 Stop_Button 后立即将按钮状态更新为"正在停止..."（disabled 状态），并在收到 turnStatus 变为 canceled 的 state op 后将 UI 过渡到已停止状态
6. WHEN Turn 被成功取消后，THE TurnSnapshot SHALL 记录 Lifecycle 为 TurnLifecycleCanceled、Error 为取消原因，并通过 persistTurnSnapshot 触发 fingerprint 更新

### 需求 14：工具输出 HTML 内容过滤

**用户故事：** 作为用户，我希望在过程时间线中看到的工具输出是干净的文本摘要，而不是原始 HTML 代码，以便获得清晰可读的执行过程展示。

#### 验收标准

1. WHEN 工具结果的 outputPreview 或 text 字段包含 HTML 内容（检测标志：包含 DOCTYPE 声明、html 标签、head/body 标签）时，THE Transport_Projector SHALL 在生成 AiopsProcessBlock 前对该内容执行 HTML_Sanitization
2. THE HTML_Sanitization SHALL 移除所有 HTML 标签并将内容截断为不超过 200 个字符的纯文本摘要
3. THE Process_Timeline 前端组件 SHALL 对 AiopsProcessBlock 的 text 和 outputPreview 字段执行二次 HTML 过滤，确保即使后端未完全过滤，前端也不会渲染原始 HTML
4. THE 实现 SHALL 遵循安全原则"不把模型输出的 HTML 直接渲染到页面"——任何来自工具输出的内容在展示前必须经过标签剥离处理
5. WHEN HTML 内容被过滤后，THE 显示文本 SHALL 保留有意义的文本内容（如页面标题、正文片段），而非仅显示空白或无意义字符

### 需求 15：工具调用展示为紧凑摘要

**用户故事：** 作为用户，我希望工具调用在过程时间线中仅显示紧凑的一行摘要（工具名 + 简短输入描述），而不是展示完整的工具输出内容，以便时间线保持简洁可扫读。

#### 验收标准

1. WHEN 工具调用在 Process_Timeline 中展示时，THE Tool_Summary_Line SHALL 仅显示：图标 + 工具名称 + 简短输入摘要（总长度不超过 80 个字符）
2. THE Tool_Summary_Line SHALL 不在时间线中内联展示完整的工具输出内容
3. IF 用户点击/展开某个 Tool_Summary_Line，THEN THE Process_Timeline MAY 显示经过 HTML 过滤且截断至 500 字符的工具输出详情
4. WHEN 工具输出详情被展开显示时，THE 详情内容 SHALL 经过 HTML_Sanitization 处理，不包含任何原始 HTML 标签
5. THE readableBlockSummary 函数 SHALL 对 command 类型的 block 仅返回命令文本（block.command），不返回 block.text 中可能包含的完整输出内容
