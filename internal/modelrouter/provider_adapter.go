package modelrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"aiops-v2/internal/promptinput"
	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ProviderAdapter interface {
	Call(ctx context.Context, req ProviderRequestSnapshot, onFinalDelta func(string), onReasoning func(ReasoningStreamEvent)) (ProviderResponse, error)
}

type EinoProviderAdapter struct {
	model          ChatModel
	tools          []einotool.BaseTool
	extraOptions   []einomodel.Option
	requestTimeout time.Duration
}

type EinoProviderAdapterOption func(*EinoProviderAdapter)

func WithEinoTools(tools []einotool.BaseTool) EinoProviderAdapterOption {
	return func(adapter *EinoProviderAdapter) {
		adapter.tools = append([]einotool.BaseTool(nil), tools...)
	}
}

func WithEinoModelOptions(options ...einomodel.Option) EinoProviderAdapterOption {
	return func(adapter *EinoProviderAdapter) {
		adapter.extraOptions = append([]einomodel.Option(nil), options...)
	}
}

func WithEinoRequestTimeoutMs(timeoutMs int) EinoProviderAdapterOption {
	return func(adapter *EinoProviderAdapter) {
		adapter.requestTimeout = modelResponseTimeout(timeoutMs)
	}
}

func NewEinoProviderAdapter(model ChatModel, options ...EinoProviderAdapterOption) *EinoProviderAdapter {
	adapter := &EinoProviderAdapter{model: model, requestTimeout: defaultModelResponseTimeout}
	for _, option := range options {
		if option != nil {
			option(adapter)
		}
	}
	return adapter
}

func (a *EinoProviderAdapter) Call(ctx context.Context, req ProviderRequestSnapshot, onFinalDelta func(string), onReasoning func(ReasoningStreamEvent)) (ProviderResponse, error) {
	if a == nil || a.model == nil {
		return ProviderResponse{}, fmt.Errorf("provider adapter model is required")
	}
	if err := validateCanonicalProviderInput(req.Input); err != nil {
		return ProviderResponse{}, err
	}
	messages, audit, err := ModelInputItemsToEinoMessages(req.Input)
	if err != nil {
		return ProviderResponse{}, err
	}
	req.ProviderMessagesHash = audit.ProviderMessagesHash
	req.MessageAudit = &audit
	req.ComputeHashes()
	started := time.Now()
	response, err := generateEinoModelResponseWithTimeout(ctx, a.model, messages, a.tools, a.requestTimeout, onFinalDelta, onReasoning, a.extraOptions...)
	finished := time.Now()
	providerResp := ProviderResponse{
		RequestID:    req.ModelInputHash,
		StartedAt:    started,
		FinishedAt:   finished,
		FinishReason: einoModelResponseFinishReason(response),
		Usage:        providerUsageFromEino(response),
	}
	if response != nil {
		providerResp.Output = response.Content
		providerResp.ReasoningContent = response.ReasoningContent
		providerResp.ToolCalls = providerToolCallsFromEino(response.ToolCalls)
		providerResp.NativeWebSearchEvents = ProviderNativeWebSearchEventsFromExtra(response.Extra)
	}
	if err != nil {
		return providerResp, err
	}
	return providerResp, nil
}

func validateCanonicalProviderInput(items []promptinput.ModelInputItem) error {
	if err := promptinput.ValidateModelInputCausalOrder(items); err != nil {
		return fmt.Errorf("canonical model input causal order: %w", err)
	}
	if err := promptinput.ValidateModelInputLogicalOrder(items, true); err != nil {
		return fmt.Errorf("canonical model input logical order: %w", err)
	}
	return nil
}

func generateEinoModelResponse(
	ctx context.Context,
	chatModel ChatModel,
	input []*schema.Message,
	toolPool []einotool.BaseTool,
	onFinalDelta func(string),
	onReasoning func(ReasoningStreamEvent),
	extraOptions ...einomodel.Option,
) (*schema.Message, error) {
	return generateEinoModelResponseWithTimeout(ctx, chatModel, input, toolPool, defaultModelResponseTimeout, onFinalDelta, onReasoning, extraOptions...)
}

func generateEinoModelResponseWithTimeout(
	ctx context.Context,
	chatModel ChatModel,
	input []*schema.Message,
	toolPool []einotool.BaseTool,
	timeout time.Duration,
	onFinalDelta func(string),
	onReasoning func(ReasoningStreamEvent),
	extraOptions ...einomodel.Option,
) (*schema.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = defaultModelResponseTimeout
	}
	requestStarted := time.Now()
	modelCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ctx = modelCtx

	toolInfos, err := toolInfosFromPool(ctx, toolPool)
	if err != nil {
		return nil, fmt.Errorf("tool info: %w", err)
	}
	opts := modelOptionsForTools(toolInfos, extraOptions...)

	stream, streamErr := chatModel.Stream(ctx, input, opts...)
	if streamErr == nil && stream != nil {
		defer stream.Close()
		chunks := make([]*schema.Message, 0, 8)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, readableModelResponseTimeoutError(err, timeout, time.Since(requestStarted))
				}
				return nil, err
			}
			if msg == nil {
				continue
			}
			if onReasoning != nil && len(msg.Extra) > 0 {
				event, err := ParseOpenAIReasoningExtra(msg.Extra, false)
				if err != nil {
					return nil, err
				}
				if event != nil {
					onReasoning(*event)
				}
			}
			if onFinalDelta != nil && msg.Content != "" {
				onFinalDelta(msg.Content)
			}
			chunks = append(chunks, msg)
		}
		response, err := schema.ConcatMessages(chunks)
		if err != nil {
			return nil, err
		}
		attachConcatenatedResponseMeta(response, chunks)
		if isEmptyAssistantResponse(response) {
			return generateFallbackResponse(ctx, chatModel, input, opts, onFinalDelta)
		}
		return response, nil
	}

	response, err := chatModel.Generate(ctx, input, opts...)
	if err != nil {
		if streamErr != nil {
			return nil, readableModelResponseTimeoutError(streamErr, timeout, time.Since(requestStarted))
		}
		return nil, readableModelResponseTimeoutError(err, timeout, time.Since(requestStarted))
	}
	if isEmptyAssistantResponse(response) {
		if fallback := fallbackResponseFromToolEvidence(input); fallback != nil {
			if onFinalDelta != nil && fallback.Content != "" {
				onFinalDelta(fallback.Content)
			}
			return fallback, nil
		}
		return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls")
	}
	if onFinalDelta != nil && response.Content != "" {
		onFinalDelta(response.Content)
	}
	return response, nil
}

const defaultModelResponseTimeout = 5 * time.Minute
const modelConnectionTimeoutVisibleMessage = "模型服务连接超时，未能建立连接。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。"

func modelResponseTimeout(timeoutMs int) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return defaultModelResponseTimeout
}

type userVisibleModelTimeoutError struct {
	message string
	cause   error
}

func (e userVisibleModelTimeoutError) Error() string {
	return e.message
}

func (e userVisibleModelTimeoutError) Unwrap() error {
	return e.cause
}

func (e userVisibleModelTimeoutError) Timeout() bool {
	return true
}

func readableModelResponseTimeoutError(err error, timeout time.Duration, elapsed time.Duration) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if !errors.Is(err, context.DeadlineExceeded) && !isTimeoutLikeError(err) {
		return err
	}
	if !usedConfiguredTimeoutBudget(timeout, elapsed) {
		return userVisibleModelTimeoutError{message: modelConnectionTimeoutVisibleMessage, cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) || isTimeoutLikeError(err) {
		return userVisibleModelTimeoutError{
			message: fmt.Sprintf("模型响应超时：%s 内未收到模型完整响应。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。", timeout),
			cause:   err,
		}
	}
	return err
}

func isTimeoutLikeError(err error) bool {
	var timeoutErr interface {
		Timeout() bool
	}
	return errors.As(err, &timeoutErr) && timeoutErr.Timeout()
}

func usedConfiguredTimeoutBudget(timeout time.Duration, elapsed time.Duration) bool {
	if timeout <= 0 || elapsed <= 0 {
		return false
	}
	if timeout <= 100*time.Millisecond {
		return elapsed >= timeout/2
	}
	margin := timeout / 20
	if margin < time.Second {
		margin = time.Second
	}
	if margin > 10*time.Second {
		margin = 10 * time.Second
	}
	return elapsed >= timeout-margin
}

func formatObservedModelTimeout(elapsed time.Duration) time.Duration {
	if elapsed <= 0 {
		return 0
	}
	if elapsed < time.Second {
		rounded := elapsed.Round(time.Millisecond)
		if rounded <= 0 {
			return time.Millisecond
		}
		return rounded
	}
	return elapsed.Round(time.Second)
}

func attachConcatenatedResponseMeta(response *schema.Message, chunks []*schema.Message) {
	if response == nil {
		return
	}
	var latest *schema.ResponseMeta
	for i := len(chunks) - 1; i >= 0; i-- {
		if chunks[i] == nil || chunks[i].ResponseMeta == nil {
			continue
		}
		latest = chunks[i].ResponseMeta
		break
	}
	if latest == nil {
		return
	}
	if response.ResponseMeta == nil {
		cp := *latest
		response.ResponseMeta = &cp
		return
	}
	if strings.TrimSpace(response.ResponseMeta.FinishReason) == "" && strings.TrimSpace(latest.FinishReason) != "" {
		response.ResponseMeta.FinishReason = latest.FinishReason
	}
	if response.ResponseMeta.Usage == nil && latest.Usage != nil {
		response.ResponseMeta.Usage = latest.Usage
	}
}

func generateFallbackResponse(
	ctx context.Context,
	chatModel ChatModel,
	input []*schema.Message,
	opts []einomodel.Option,
	onFinalDelta func(string),
) (*schema.Message, error) {
	const fallbackAttempts = 2
	var response *schema.Message

	for attempt := 0; attempt < fallbackAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var err error
		response, err = chatModel.Generate(ctx, input, opts...)
		if err != nil {
			return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls; generate fallback failed: %w", err)
		}
		if !isEmptyAssistantResponse(response) {
			break
		}
		if len(opts) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err = chatModel.Generate(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls; no-tool generate fallback failed: %w", err)
		}
		if !isEmptyAssistantResponse(response) {
			break
		}
	}
	if isEmptyAssistantResponse(response) {
		if fallback := fallbackResponseFromToolEvidence(input); fallback != nil {
			if onFinalDelta != nil && fallback.Content != "" {
				onFinalDelta(fallback.Content)
			}
			return fallback, nil
		}
		return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls")
	}
	if onFinalDelta != nil && response.Content != "" {
		onFinalDelta(response.Content)
	}
	return response, nil
}

func modelOptionsForTools(toolInfos []*schema.ToolInfo, extraOptions ...einomodel.Option) []einomodel.Option {
	opts := make([]einomodel.Option, 0, len(extraOptions)+2)
	if len(toolInfos) == 0 {
		return append(opts, extraOptions...)
	}
	opts = append(opts,
		einomodel.WithTools(toolInfos),
		einomodel.WithToolChoice(schema.ToolChoiceAllowed),
	)
	return append(opts, extraOptions...)
}

func isEmptyAssistantResponse(msg *schema.Message) bool {
	if msg == nil {
		return true
	}
	return strings.TrimSpace(msg.Content) == "" &&
		len(msg.ToolCalls) == 0 &&
		len(msg.MultiContent) == 0 &&
		len(msg.AssistantGenMultiContent) == 0
}

func fallbackResponseFromToolEvidence(input []*schema.Message) *schema.Message {
	userRequest := latestUserContent(input)
	content := latestSuccessfulToolEvidenceContent(input)
	if content == "" {
		return nil
	}
	fallback := fallbackTextFromToolEvidence(userRequest, content)
	if strings.TrimSpace(fallback) == "" {
		return nil
	}
	return schema.AssistantMessage(fallback, nil)
}

func latestUserContent(input []*schema.Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		msg := input[i]
		if msg == nil || msg.Role != schema.User {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			return text
		}
	}
	return ""
}

func latestSuccessfulToolEvidenceContent(input []*schema.Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		msg := input[i]
		if msg == nil || msg.Role != schema.Tool {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || toolEvidenceLooksFailed(content) {
			continue
		}
		return content
	}
	return ""
}

func toolEvidenceLooksFailed(content string) bool {
	var obj map[string]any
	if json.Unmarshal([]byte(content), &obj) == nil {
		if errorValue, ok := obj["error"]; ok && strings.TrimSpace(fmt.Sprint(errorValue)) != "" {
			return true
		}
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["status"])))
		return status == "error" || status == "failed" || status == "failure"
	}
	lower := strings.ToLower(content)
	return strings.Contains(lower, "tool not found") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "failed to") ||
		strings.Contains(lower, "执行失败")
}

func fallbackTextFromToolEvidence(userRequest, content string) string {
	if answer, ok := scalarJSONFieldAnswerFromRequest(userRequest, content); ok {
		return answer
	}
	sanitized := sanitizeToolEvidenceForFallback(content)
	if sanitized == "" {
		return ""
	}
	return "已获取工具结果：\n\n" + truncateRunes(sanitized, 1200)
}

func scalarJSONFieldAnswerFromRequest(userRequest, content string) (string, bool) {
	var obj map[string]any
	if json.Unmarshal([]byte(content), &obj) != nil || len(obj) == 0 {
		return "", false
	}
	request := normalizePromptLookupText(userRequest)
	for key, value := range obj {
		if isSensitiveEvidenceKey(key) || !requestMentionsJSONField(request, key) {
			continue
		}
		text, ok := scalarJSONValueText(value)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		return fmt.Sprintf("%s 字段是 %s。", key, text), true
	}
	return "", false
}

func requestMentionsJSONField(normalizedRequest, key string) bool {
	normalizedKey := normalizePromptLookupText(key)
	if normalizedKey != "" && strings.Contains(normalizedRequest, normalizedKey) {
		return true
	}
	switch strings.ToLower(key) {
	case "model":
		return strings.Contains(normalizedRequest, "模型")
	case "provider":
		return strings.Contains(normalizedRequest, "供应商") || strings.Contains(normalizedRequest, "提供商") || strings.Contains(normalizedRequest, "接入")
	case "status":
		return strings.Contains(normalizedRequest, "状态")
	default:
		return false
	}
}

func scalarJSONValueText(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case float64, bool:
		return fmt.Sprint(v), true
	case nil:
		return "", false
	default:
		return "", false
	}
}

func sanitizeToolEvidenceForFallback(content string) string {
	var value any
	if json.Unmarshal([]byte(content), &value) == nil {
		redacted := redactSensitiveEvidenceValue(value)
		encoded, err := json.MarshalIndent(redacted, "", "  ")
		if err == nil {
			return string(encoded)
		}
	}
	return redactSensitiveEvidenceText(content)
}

func redactSensitiveEvidenceValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if isSensitiveEvidenceKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactSensitiveEvidenceValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, redactSensitiveEvidenceValue(child))
		}
		return out
	default:
		return value
	}
}

func isSensitiveEvidenceKey(key string) bool {
	normalized := normalizePromptLookupText(key)
	return strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "authorization")
}

func normalizePromptLookupText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "", ":", "", "/", "")
	return replacer.Replace(value)
}

func redactSensitiveEvidenceText(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if isSensitiveEvidenceLine(line) {
			lines[i] = "[REDACTED]"
		}
	}
	return strings.Join(lines, "\n")
}

func isSensitiveEvidenceLine(line string) bool {
	normalized := normalizePromptLookupText(line)
	return strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "authorization")
}

func toolInfosFromPool(ctx context.Context, toolPool []einotool.BaseTool) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(toolPool))
	for _, baseTool := range toolPool {
		if baseTool == nil {
			continue
		}
		info, err := baseTool.Info(ctx)
		if err != nil {
			return nil, err
		}
		if info != nil {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

func einoModelResponseFinishReason(response *schema.Message) string {
	if response == nil || response.ResponseMeta == nil {
		return ""
	}
	return strings.TrimSpace(response.ResponseMeta.FinishReason)
}

func providerUsageFromEino(response *schema.Message) ProviderUsage {
	if response == nil || response.ResponseMeta == nil || response.ResponseMeta.Usage == nil {
		return ProviderUsage{}
	}
	usage := response.ResponseMeta.Usage
	return ProviderUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

func providerToolCallsFromEino(toolCalls []schema.ToolCall) []promptinput.ModelInputToolCall {
	out := make([]promptinput.ModelInputToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, promptinput.ModelInputToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	return out
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}
