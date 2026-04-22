package spanstream

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
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

	resp, err := cc.summaryModel.Generate(ctx, prompt)
	if err != nil {
		return "", err
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
	// System instruction for summarization
	systemMsg := &schema.Message{
		Role: schema.System,
		Content: "You are a concise summarizer. Given a conversation or tool output, " +
			"produce a brief summary (1-2 sentences) that captures the key findings or actions. " +
			"Focus on results, errors, and important data points. Do not include pleasantries.",
	}

	// Build the content to summarize
	var contentBuilder strings.Builder
	if span != nil {
		contentBuilder.WriteString(fmt.Sprintf("Task: %s (type: %s)\n\n", span.Name, span.Type))
	}
	for _, msg := range messages {
		contentBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	userMsg := &schema.Message{
		Role:    schema.User,
		Content: "Summarize the following conversation/output:\n\n" + contentBuilder.String(),
	}

	return []*schema.Message{systemMsg, userMsg}
}
