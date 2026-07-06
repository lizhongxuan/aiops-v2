package modelrouter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/promptinput"

	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type emptyResponseModel struct{}

func (m *emptyResponseModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant}, nil
}

func (m *emptyResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *emptyResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

type recordingProviderChatModel struct {
	input []*schema.Message
}

func (m *recordingProviderChatModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.input = append([]*schema.Message(nil), input...)
	return schema.AssistantMessage("ok", nil), nil
}

func (m *recordingProviderChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *recordingProviderChatModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestEinoProviderAdapterCallsModelThroughSnapshot(t *testing.T) {
	model := &recordingProviderChatModel{}
	adapter := NewEinoProviderAdapter(model)
	resp, err := adapter.Call(context.Background(), ProviderRequestSnapshot{
		Provider: "openai",
		Model:    "gpt-4.1",
		Input: []promptinput.ModelInputItem{{
			ID:           "user-1",
			ProviderRole: promptinput.ProviderRoleUser,
			Content:      "hello",
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if resp.Output != "ok" || resp.RequestID == "" {
		t.Fatalf("provider response = %#v, want output and request id", resp)
	}
	if len(model.input) != 1 || model.input[0].Role != schema.User || model.input[0].Content != "hello" {
		t.Fatalf("model input = %#v, want converted user message", model.input)
	}
}

func TestEinoProviderAdapterPreservesReasoningContentForThinkingMode(t *testing.T) {
	model := &streamingResponseModel{chunks: []*schema.Message{
		{Role: schema.Assistant, ReasoningContent: "先检查工具状态。"},
		schema.AssistantMessage("需要继续检查。", nil),
	}}
	adapter := NewEinoProviderAdapter(model)
	resp, err := adapter.Call(context.Background(), ProviderRequestSnapshot{
		Provider: "deepseek",
		Model:    "deepseek-v4-pro",
		Input: []promptinput.ModelInputItem{{
			ID:           "user-1",
			ProviderRole: promptinput.ProviderRoleUser,
			Content:      "检查 agent",
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if resp.Output != "需要继续检查。" {
		t.Fatalf("Output = %q, want final content", resp.Output)
	}
	if resp.ReasoningContent != "先检查工具状态。" {
		t.Fatalf("ReasoningContent = %q, want provider reasoning preserved", resp.ReasoningContent)
	}
}

func TestModelInputItemsToEinoMessagesPreservesAssistantReasoningContent(t *testing.T) {
	messages, _, err := ModelInputItemsToEinoMessages([]promptinput.ModelInputItem{{
		ID:               "assistant-1",
		ProviderRole:     promptinput.ProviderRoleAssistant,
		Content:          "需要继续检查。",
		ReasoningContent: "先检查工具状态。",
	}})
	if err != nil {
		t.Fatalf("ModelInputItemsToEinoMessages() error = %v", err)
	}
	if len(messages) != 1 || messages[0].Role != schema.Assistant {
		t.Fatalf("messages = %#v, want one assistant message", messages)
	}
	if messages[0].ReasoningContent != "先检查工具状态。" {
		t.Fatalf("ReasoningContent = %q, want preserved on provider message", messages[0].ReasoningContent)
	}
}

func TestGenerateModelResponseRejectsEmptyAssistantMessage(t *testing.T) {
	_, err := generateEinoModelResponse(context.Background(), &emptyResponseModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected empty model response error")
	}
	if !strings.Contains(err.Error(), "empty model response") {
		t.Fatalf("error = %v, want empty model response", err)
	}
}

func TestGenerateModelResponseUsesLatestToolEvidenceWhenModelStaysEmpty(t *testing.T) {
	var deltas []string
	msg, err := generateEinoModelResponse(
		context.Background(),
		&emptyResponseModel{},
		[]*schema.Message{
			schema.UserMessage("Tell me current model name only. Do not reveal or mention any api key."),
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-model-config",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "get_current_model_config",
					Arguments: `{}`,
				},
			}}),
			schema.ToolMessage(`{"apiKeySet":true,"baseURL":"https://example.invalid/v1","model":"glm-4.7","provider":"zhipu"}`, "call-model-config"),
		},
		nil,
		func(delta string) {
			deltas = append(deltas, delta)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg == nil || !strings.Contains(msg.Content, "glm-4.7") {
		t.Fatalf("fallback content = %q, want model evidence", msg.Content)
	}
	if strings.Contains(strings.ToLower(msg.Content), "apikey") || strings.Contains(msg.Content, "example.invalid") {
		t.Fatalf("fallback leaked sensitive or irrelevant config details: %q", msg.Content)
	}
	if got := strings.Join(deltas, ""); got != msg.Content {
		t.Fatalf("deltas = %q, want %q", got, msg.Content)
	}
}

type streamingResponseModel struct {
	chunks []*schema.Message
}

func (m *streamingResponseModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called when streaming is available")
}

func (m *streamingResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](len(m.chunks) + 1)
	go func() {
		defer sw.Close()
		for _, chunk := range m.chunks {
			sw.Send(chunk, nil)
		}
	}()
	return sr, nil
}

func (m *streamingResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

type emptyStreamGenerateResponseModel struct {
	generateCalls int
}

func (m *emptyStreamGenerateResponseModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	m.generateCalls++
	return schema.AssistantMessage("fallback final", nil), nil
}

func (m *emptyStreamGenerateResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](2)
	go func() {
		defer sw.Close()
		sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: "thinking"}, nil)
	}()
	return sr, nil
}

func (m *emptyStreamGenerateResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseFallsBackToGenerateWhenStreamIsEmpty(t *testing.T) {
	model := &emptyStreamGenerateResponseModel{}
	var deltas []string

	msg, err := generateEinoModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		func(delta string) {
			deltas = append(deltas, delta)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "fallback final" {
		t.Fatalf("response content = %q, want fallback final", msg.Content)
	}
	if model.generateCalls != 1 {
		t.Fatalf("Generate calls = %d, want 1", model.generateCalls)
	}
	if got := strings.Join(deltas, "|"); got != "fallback final" {
		t.Fatalf("stream deltas = %q, want fallback final", got)
	}
}

type emptyToolOptionsGenerateResponseModel struct {
	toolOptionGenerateCalls   int
	noToolOptionGenerateCalls int
}

func (m *emptyToolOptionsGenerateResponseModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if len(opts) > 0 {
		m.toolOptionGenerateCalls++
		return &schema.Message{Role: schema.Assistant}, nil
	}
	m.noToolOptionGenerateCalls++
	return schema.AssistantMessage("no-tool fallback final", nil), nil
}

func (m *emptyToolOptionsGenerateResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](2)
	go func() {
		defer sw.Close()
		sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: "thinking"}, nil)
	}()
	return sr, nil
}

func (m *emptyToolOptionsGenerateResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

type staticToolInfo struct{}

func (staticToolInfo) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name:        "noop",
		Desc:        "No-op test tool.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
	}, nil
}

func TestGenerateModelResponseFallsBackWithoutToolOptionsWhenToolOptionsStayEmpty(t *testing.T) {
	model := &emptyToolOptionsGenerateResponseModel{}
	var deltas []string

	msg, err := generateEinoModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		[]einotool.BaseTool{staticToolInfo{}},
		func(delta string) {
			deltas = append(deltas, delta)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "no-tool fallback final" {
		t.Fatalf("response content = %q, want no-tool fallback final", msg.Content)
	}
	if model.toolOptionGenerateCalls != 1 {
		t.Fatalf("tool-option Generate calls = %d, want 1", model.toolOptionGenerateCalls)
	}
	if model.noToolOptionGenerateCalls != 1 {
		t.Fatalf("no-tool-option Generate calls = %d, want 1", model.noToolOptionGenerateCalls)
	}
	if got := strings.Join(deltas, "|"); got != "no-tool fallback final" {
		t.Fatalf("stream deltas = %q, want no-tool fallback final", got)
	}
}

type retryEmptyToolOptionsGenerateResponseModel struct {
	toolOptionGenerateCalls   int
	noToolOptionGenerateCalls int
}

func (m *retryEmptyToolOptionsGenerateResponseModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if len(opts) > 0 {
		m.toolOptionGenerateCalls++
		return &schema.Message{Role: schema.Assistant}, nil
	}
	m.noToolOptionGenerateCalls++
	if m.noToolOptionGenerateCalls == 1 {
		return &schema.Message{Role: schema.Assistant}, nil
	}
	return schema.AssistantMessage("retry no-tool fallback final", nil), nil
}

func (m *retryEmptyToolOptionsGenerateResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](2)
	go func() {
		defer sw.Close()
		sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: "thinking"}, nil)
	}()
	return sr, nil
}

func (m *retryEmptyToolOptionsGenerateResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseRetriesEmptyNoToolFallbackOnce(t *testing.T) {
	model := &retryEmptyToolOptionsGenerateResponseModel{}
	var deltas []string

	msg, err := generateEinoModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		[]einotool.BaseTool{staticToolInfo{}},
		func(delta string) {
			deltas = append(deltas, delta)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "retry no-tool fallback final" {
		t.Fatalf("response content = %q, want retry no-tool fallback final", msg.Content)
	}
	if model.toolOptionGenerateCalls != 2 {
		t.Fatalf("tool-option Generate calls = %d, want 2", model.toolOptionGenerateCalls)
	}
	if model.noToolOptionGenerateCalls != 2 {
		t.Fatalf("no-tool-option Generate calls = %d, want 2", model.noToolOptionGenerateCalls)
	}
	if got := strings.Join(deltas, "|"); got != "retry no-tool fallback final" {
		t.Fatalf("stream deltas = %q, want retry no-tool fallback final", got)
	}
}

func TestGenerateModelResponseStreamsChunksAndConcatsFinalMessage(t *testing.T) {
	model := &streamingResponseModel{
		chunks: []*schema.Message{
			schema.AssistantMessage("第一段", nil),
			schema.AssistantMessage("\n\n", nil),
			schema.AssistantMessage("第二段", nil),
		},
	}

	var deltas []string
	msg, err := generateEinoModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		func(delta string) {
			deltas = append(deltas, delta)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "第一段\n\n第二段" {
		t.Fatalf("response content = %q, want concatenated stream", msg.Content)
	}
	if got, want := strings.Join(deltas, "|"), "第一段|\n\n|第二段"; got != want {
		t.Fatalf("stream deltas = %q, want %q", got, want)
	}
}

type noToolOptionModel struct{}

func (m *noToolOptionModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called when streaming is available")
}

func (m *noToolOptionModel) Stream(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if len(opts) != 0 {
		return nil, errors.New("unexpected tool options without tools")
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		sw.Send(schema.AssistantMessage("final", nil), nil)
	}()
	return sr, nil
}

func (m *noToolOptionModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseOmitsToolOptionsWhenNoTools(t *testing.T) {
	msg, err := generateEinoModelResponse(context.Background(), &noToolOptionModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, nil, nil)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "final" {
		t.Fatalf("response content = %q, want final", msg.Content)
	}
}

type optionCaptureModel struct {
	maxTokens int
}

func (m *optionCaptureModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called when streaming is available")
}

func (m *optionCaptureModel) Stream(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	common := model.GetCommonOptions(&model.Options{}, opts...)
	if common != nil && common.MaxTokens != nil {
		m.maxTokens = *common.MaxTokens
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		sw.Send(schema.AssistantMessage("bounded final", nil), nil)
	}()
	return sr, nil
}

func (m *optionCaptureModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponsePassesExtraOptionsWithoutTools(t *testing.T) {
	capture := &optionCaptureModel{}
	msg, err := generateEinoModelResponse(
		context.Background(),
		capture,
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		nil,
		nil,
		model.WithMaxTokens(321),
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "bounded final" {
		t.Fatalf("response content = %q, want bounded final", msg.Content)
	}
	if capture.maxTokens != 321 {
		t.Fatalf("captured maxTokens = %d, want 321", capture.maxTokens)
	}
}

func TestGenerateModelResponseEmitsOnlyReasoningSummaryEvents(t *testing.T) {
	model := &streamingResponseModel{
		chunks: []*schema.Message{
			{
				Role: schema.Assistant,
				Extra: map[string]any{
					"method": "item/reasoning/summaryTextDelta",
					"params": map[string]any{
						"threadId":     "thread_1",
						"turnId":       "turn_1",
						"itemId":       "reasoning_1",
						"summaryIndex": float64(0),
						"delta":        "我会先查看项目结构。",
					},
				},
			},
			{
				Role: schema.Assistant,
				Extra: map[string]any{
					"method": "item/reasoning/textDelta",
					"params": map[string]any{
						"threadId":     "thread_1",
						"turnId":       "turn_1",
						"itemId":       "reasoning_1",
						"contentIndex": float64(0),
						"delta":        "raw hidden",
					},
				},
			},
			schema.AssistantMessage("final", nil),
		},
	}

	var reasoning []ReasoningStreamEvent
	msg, err := generateEinoModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		nil,
		func(event ReasoningStreamEvent) {
			reasoning = append(reasoning, event)
		},
	)
	if err != nil {
		t.Fatalf("generateEinoModelResponse returned error: %v", err)
	}
	if msg.Content != "final" {
		t.Fatalf("response content = %q, want final", msg.Content)
	}
	if len(reasoning) != 1 {
		t.Fatalf("reasoning events length = %d, want 1: %+v", len(reasoning), reasoning)
	}
	if reasoning[0].Method != "item/reasoning/summaryTextDelta" || reasoning[0].Delta != "我会先查看项目结构。" {
		t.Fatalf("reasoning[0] = %+v, want summary delta", reasoning[0])
	}
}

// cancelStreamModel simulates a streaming model that blocks until context is canceled.
type cancelStreamModel struct{}

func (m *cancelStreamModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called")
}

func (m *cancelStreamModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		// Block until context is canceled, then send the context error.
		<-ctx.Done()
		sw.Send(nil, ctx.Err())
	}()
	return sr, nil
}

func (m *cancelStreamModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

type wrappedObservedTimeoutError struct{}

func (wrappedObservedTimeoutError) Error() string {
	return `Post "https://provider.invalid/v1/chat/completions": net/http: TLS handshake timeout`
}

func (wrappedObservedTimeoutError) Timeout() bool {
	return true
}

func (wrappedObservedTimeoutError) Temporary() bool {
	return true
}

func (wrappedObservedTimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}

type observedNetworkTimeoutModel struct {
	delay time.Duration
}

func (m *observedNetworkTimeoutModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate fallback should not replace stream network timeout")
}

func (m *observedNetworkTimeoutModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	timer := time.NewTimer(m.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, wrappedObservedTimeoutError{}
	}
}

func (m *observedNetworkTimeoutModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseReturnsContextCanceledImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := generateEinoModelResponse(ctx, &cancelStreamModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestGenerateModelResponseTimesOutWhenProviderDoesNotReturn(t *testing.T) {
	t.Setenv("AIOPS_LLM_REQUEST_TIMEOUT_MS", "25")

	started := time.Now()
	_, err := generateEinoModelResponseWithTimeout(context.Background(), &cancelStreamModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, 25*time.Millisecond, nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s, want prompt request-level timeout", elapsed)
	}
}

func TestModelResponseTimeoutIgnoresDeprecatedEnvAndDefaultsLong(t *testing.T) {
	t.Setenv("AIOPS_LLM_REQUEST_TIMEOUT_MS", "25")
	t.Setenv("AIOPS_LLM_REQUEST_TIMEOUT", "25ms")

	if got := modelResponseTimeout(0); got < 5*time.Minute {
		t.Fatalf("default model response timeout = %s, want at least 5m for long operational analysis", got)
	}
}

func TestGenerateModelResponseWrapsProviderDeadlineWithReadableTimeout(t *testing.T) {
	t.Setenv("AIOPS_LLM_REQUEST_TIMEOUT_MS", "25")

	_, err := generateEinoModelResponseWithTimeout(context.Background(), &cancelStreamModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, 25*time.Millisecond, nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "模型响应超时") || !strings.Contains(err.Error(), "25ms") {
		t.Fatalf("error = %q, want readable model timeout with duration", err.Error())
	}
}

func TestGenerateModelResponseSanitizesObservedNetworkTimeout(t *testing.T) {
	_, err := generateEinoModelResponseWithTimeout(
		context.Background(),
		&observedNetworkTimeoutModel{delay: 20 * time.Millisecond},
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		5*time.Minute,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected network timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	got := err.Error()
	if strings.Contains(got, "5m0s") {
		t.Fatalf("error = %q, must not report configured 5m budget for observed network timeout", got)
	}
	for _, forbidden := range []string{"provider.invalid", "chat/completions", "Post ", "TLS handshake timeout", "i/o timeout", "约 20ms", "约 20s"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("error = %q, must not expose raw provider timeout detail %q", got, forbidden)
		}
	}
	for _, want := range []string{"模型服务连接超时", "上下文较大", "稍后重试"} {
		if !strings.Contains(got, want) {
			t.Fatalf("error = %q, want %q", got, want)
		}
	}
}

func TestEinoProviderAdapterUsesConfiguredRequestTimeout(t *testing.T) {
	adapter := NewEinoProviderAdapter(&cancelStreamModel{}, WithEinoRequestTimeoutMs(25))

	started := time.Now()
	_, err := adapter.Call(context.Background(), ProviderRequestSnapshot{
		Provider: "openai",
		Model:    "gpt-5.4",
		Input: []promptinput.ModelInputItem{{
			ID:           "user-1",
			ProviderRole: promptinput.ProviderRoleUser,
			Content:      "ping",
		}},
	}, nil, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "25ms") {
		t.Fatalf("error = %v, want readable 25ms deadline", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("timeout took %s, want configured request timeout", elapsed)
	}
}
