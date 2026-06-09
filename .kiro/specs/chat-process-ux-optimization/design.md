# Chat Process UX 优化 — 技术设计

## 1. 设计目标

参考 Codex-style 结构化流式输出设计原则：
- 结构稳定：不从 Markdown/HTML/JSON 字符串猜 UI 状态
- 流式自然：reasoning、tool、final answer 边产生边展示
- 安全可信：不把模型输出的 HTML 直接渲染到页面
- 贴合现有架构：继续使用 AssistantTransport，不新增 SSE/WebSocket

当前生产路径：
TurnItem → AiopsTransportState → AssistantTransport data stream → assistant-ui React

## 2. 问题分析

### 2.1 "处理中"无输出
- 根因：reasoning stream events (EventReasoningSummaryDelta) 未写入 TurnSnapshot
- Transport 轮询检测不到 fingerprint 变化
- 前端过滤了 "calling model" 文本

### 2.2 停止按钮无效
- Stop 命令通过新 HTTP 请求发送 aiops.stop
- 调用 requestTurnCancel 设置 cancel flag
- 但 chatModel.Stream() 的 context 未被及时取消
- 原始 streaming 请求的 polling loop 继续运行

### 2.3 HTML 原始输出
- 工具返回网页 HTML 内容
- outputPreview/text 字段包含原始 HTML
- ProcessTranscript 直接渲染未经过滤

## 3. 后端改动

### 3.1 推理流实时传输 (eino_kernel.go)
在 runHostIterationLoop 的 onReasoning 回调中：
- 每收到 reasoning delta，更新 TurnSnapshot 的 reasoning AgentItem.text
- 节流 persistTurnSnapshot 调用（每 100ms 批量写入）
- 确保 transport polling 检测到 fingerprint 变化

伪代码：
```go
var lastReasoningPersist time.Time
onReasoning := func(event ReasoningStreamEvent) {
    // 更新 snapshot 中的 reasoning item text
    updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusRunning, current.Summary)
    // 节流持久化
    if time.Since(lastReasoningPersist) > 100*time.Millisecond {
        k.persistTurnSnapshot(session, snapshot)
        lastReasoningPersist = time.Now()
    }
}
```

### 3.2 停止按钮修复 (eino_kernel.go)
- requestTurnCancel 已有 registered cancel function
- 确保 cancel function 立即调用 runCancel() 取消 runCtx
- chatModel.Stream() 使用 runCtx，context 取消后 stream.Recv() 返回 error
- 在 generateModelResponse 中检测 context.Canceled 并快速返回

### 3.3 HTML 过滤 (transport_projector.go)
新增 sanitizeOutputPreview 函数：
```go
func sanitizeOutputPreview(content string) string {
    // 检测 HTML 内容
    if isHTMLContent(content) {
        // 去除 HTML 标签
        stripped := stripHTMLTags(content)
        // 截断
        if len(stripped) > 200 {
            return stripped[:200] + "…"
        }
        return stripped
    }
    // 非 HTML 内容正常截断
    if len(content) > 500 {
        return content[:500] + "…"
    }
    return content
}

func isHTMLContent(s string) bool {
    trimmed := strings.TrimSpace(s)
    return strings.HasPrefix(trimmed, "<!DOCTYPE") || 
           strings.HasPrefix(trimmed, "<html") ||
           strings.HasPrefix(trimmed, "<HTML")
}
```

在 projectTurnItem 的 tool_call/tool_result 分支中调用：
```go
block.OutputPreview = sanitizeOutputPreview(outputPreviewForTransportToolBlock(blockKind, tool))
block.Text = sanitizeOutputPreview(summarizeTransportToolText(blockKind, tool, item.Payload))
```

## 4. 前端改动

### 4.1 ProcessTranscript 重构为 Codex 时间线
- 移除折叠面板包裹
- 每个 process block 渲染为时间线条目：
  - Thinking text: 黑色粗体，流式追加
  - Tool calls: 灰色摘要行 + emoji 图标
  - Search: 可展开 URL 列表

### 4.2 即时反馈
- turnStatus === "working" 且无 blocks 时显示 "正在思考"
- reasoning text 到达后替换占位符

### 4.3 HTML 过滤 (前端兜底)
```typescript
function stripHtml(text: string): string {
  if (!text) return "";
  // 检测 HTML 内容
  if (/^\s*<!DOCTYPE|^\s*<html/i.test(text)) {
    return text.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim().slice(0, 200);
  }
  return text;
}
```

### 4.4 停止按钮 UX
- 点击后立即显示 "正在停止" 状态
- 禁用 composer
- 2s 超时后如果仍未停止，显示 "强制停止" 选项

## 5. 数据流图

```
用户发送消息
  → POST /assistant-transport {commands: [{type: "add-message", ...}]}
  → TransportCommandHandler.Apply
    → ChatService.SendMessage
      → go asyncTurnRunner.run(req)
        → EinoKernel.RunTurn
          → runHostIterationLoop
            → [每次迭代]
              → PromptCompiler.Compile (4层 prompt)
              → generateModelResponse (stream)
                → onReasoning: 更新 reasoning AgentItem + persistSnapshot [NEW]
                → onFinalDelta: emit EventAssistantFinalDelta
              → dispatch tools
                → appendAgentItem(tool_call) + persistSnapshot
                → execute tool
                → appendAgentItem(tool_result) + persistSnapshot
            → [完成]
              → appendAgentItem(final_answer) + persistSnapshot
  → streamAssistantTransportState (10ms polling)
    → source.Get(sessionID)
    → ProjectTurnSnapshot → AiopsTransportState
      → sanitizeOutputPreview [NEW]
    → DiffStateOps → stream ops to client
  → Frontend AssistantTransport
    → AiopsThread → ProcessTranscript (Codex timeline) [REDESIGNED]
```

## 6. 不做事项
- 不新增 SSE/WebSocket 通道
- 不引入 StructuredResponsePatch
- 不改变 transport schema version (aiops.transport.v1)
- 不引入 AgentEventProjection/codexProcessTranscript (已迁移到 React)
- 不从 assistant 文本解析结构化状态

## 7. 测试策略

### 后端
- `transport_projector_test.go`: HTML 过滤、reasoning 增量更新
- `eino_kernel_test.go`: stop 传播、reasoning persist 节流
- `assistant_transport_api_test.go`: stop 命令端到端

### 前端
- `ProcessTranscript` 组件测试: 时间线渲染、HTML 过滤、即时反馈
- Playwright snapshot: 覆盖 thinking/tool/search/final 各状态
- Stop button: 模拟长时间运行的 turn，验证停止行为

## 8. 验收标准
- 发送消息后 ≤500ms 内显示 "正在思考"
- 停止按钮 ≤2s 内终止 turn
- 工具输出不显示原始 HTML
- 工具调用显示为紧凑摘要行
- Playwright snapshot 测试通过
