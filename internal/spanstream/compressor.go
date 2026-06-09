package spanstream

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modeltrace"
)

// ContextCompressor performs asynchronous context compression by summarizing
// verbose span content using a low-latency LLM model. It runs in background
// goroutines without blocking the main ReAct loop, and delivers summaries
// via channels. The summary replaces original verbose logs to achieve token savings.
type ContextCompressor struct {
	// summaryModel is a low-latency/cheap model used for summary extraction.
	summaryModel model.ChatModel

	// workerPool is a channel-based semaphore for concurrency control.
	// The capacity of this channel determines the max concurrent compressions.
	workerPool chan struct{}
}

// NewContextCompressor creates a ContextCompressor with the given summary model
// and maximum concurrency for background compression workers.
func NewContextCompressor(summaryModel model.ChatModel, maxConcurrency int) *ContextCompressor {
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	pool := make(chan struct{}, maxConcurrency)
	for i := 0; i < maxConcurrency; i++ {
		pool <- struct{}{}
	}
	return &ContextCompressor{
		summaryModel: summaryModel,
		workerPool:   pool,
	}
}

// Message represents a conversation message to be compressed.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompressAsync asynchronously compresses the given messages for a span.
// It returns a channel that will receive exactly one summary string when
// compression completes. If compression fails, the channel receives an
// empty string. This method does not block the caller.
func (cc *ContextCompressor) CompressAsync(ctx context.Context, span *Span, messages []Message) <-chan string {
	resultCh := make(chan string, 1)

	go func() {
		defer close(resultCh)
		summary, err := cc.Compress(ctx, span, messages)
		if err != nil {
			resultCh <- ""
			return
		}
		resultCh <- summary
	}()

	return resultCh
}

// Compress synchronously compresses the provided messages into a summary.
// It honors the compressor's concurrency limit and returns an error when the
// context is canceled, the compressor is misconfigured, or the model fails.
func (cc *ContextCompressor) Compress(ctx context.Context, span *Span, messages []Message) (string, error) {
	if cc == nil {
		return "", fmt.Errorf("context compressor is nil")
	}

	acquired, err := cc.acquireWorker(ctx)
	if err != nil {
		return "", err
	}
	defer cc.releaseWorker(acquired)

	return cc.compress(ctx, span, messages)
}

func (cc *ContextCompressor) acquireWorker(ctx context.Context) (bool, error) {
	if cc == nil || cc.workerPool == nil {
		return false, nil
	}

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-cc.workerPool:
		return true, nil
	}
}

func (cc *ContextCompressor) releaseWorker(acquired bool) {
	if !acquired || cc == nil || cc.workerPool == nil {
		return
	}
	cc.workerPool <- struct{}{}
}

// compress performs the actual compression by calling the summary model.
func (cc *ContextCompressor) compress(ctx context.Context, span *Span, messages []Message) (string, error) {
	if len(messages) == 0 {
		return "", nil
	}
	if cc.summaryModel == nil {
		return "", fmt.Errorf("summary model is nil")
	}

	// Build the prompt for summarization
	prompt := cc.buildSummaryPrompt(span, messages)
	_, _ = modeltrace.Write(modeltrace.Request{
		Kind:       "spanstream_compressor",
		TraceID:    compressorTraceID(span),
		Prompt:     compressorPromptTrace(prompt),
		ModelInput: prompt,
	})

	resp, err := cc.summaryModel.Generate(ctx, prompt)
	if err != nil {
		return "", err
	}
	if len(resp.ToolCalls) > 0 {
		return "", fmt.Errorf("summary model attempted tool call during context compression")
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return "", nil
	}

	// Update the span's summary field
	if span != nil {
		span.Summary = summary
	}

	return summary, nil
}

// buildSummaryPrompt constructs the messages for the summary extraction call.
func (cc *ContextCompressor) buildSummaryPrompt(span *Span, messages []Message) []*schema.Message {
	systemMsg := &schema.Message{
		Role: schema.System,
		Content: strings.Join([]string{
			"CRITICAL: Respond with TEXT ONLY. Do NOT call tools.",
			"请为 AIOps 长会话生成可继续工作的上下文摘要，必须保留：",
			"1. 用户当前目标和最新约束",
			"2. 当前事故 / 服务 / 主机 / 时间窗",
			"3. 已确认事实和 evidenceRefs",
			"4. 已排除假设",
			"5. 当前最可能 root cause / hypotheses",
			"6. 已执行工具和关键结果摘要",
			"7. pending approvals / denied approvals / action token 状态",
			"8. Runner / OpsManual / MCP / Skills 当前状态",
			"9. 仍需继续做的下一步",
			"10. 用户明确反馈和偏好",
		}, "\n"),
	}

	var contentBuilder strings.Builder
	if span != nil {
		contentBuilder.WriteString(fmt.Sprintf("Task: %s (type: %s)\n\n", span.Name, span.Type))
	}
	for _, msg := range messages {
		contentBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	userMsg := &schema.Message{
		Role:    schema.User,
		Content: "Summarize the following conversation/output as an AIOps continuation summary. Include transcript/ref hints and keep evidence references auditable.\n\n" + contentBuilder.String(),
	}

	return []*schema.Message{systemMsg, userMsg}
}

func compressorTraceID(span *Span) string {
	if span == nil {
		return ""
	}
	if strings.TrimSpace(span.ID) != "" {
		return span.ID
	}
	return span.Name
}

func compressorPromptTrace(prompt []*schema.Message) modeltrace.Prompt {
	trace := modeltrace.Prompt{}
	if len(prompt) > 0 && prompt[0] != nil {
		trace.System = prompt[0].Content
	}
	if len(prompt) > 1 && prompt[1] != nil {
		trace.Dynamic = prompt[1].Content
	}
	return trace
}
