package opsmanuals

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	core "aiops-v2/internal/opsmanual"
	"aiops-v2/internal/tooling"
)

type turnSearchContextCache struct {
	mu          sync.Mutex
	results     map[string]core.SearchOpsManualsResult
	resolutions map[string]core.ParamResolutionResult
}

func newTurnSearchContextCache() *turnSearchContextCache {
	return &turnSearchContextCache{
		results:     map[string]core.SearchOpsManualsResult{},
		resolutions: map[string]core.ParamResolutionResult{},
	}
}

func (c *turnSearchContextCache) remember(ctx context.Context, result core.SearchOpsManualsResult) {
	if c == nil {
		return
	}
	key := turnSearchContextKey(ctx)
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results[key] = cloneSearchOpsManualsResult(result)
}

func (c *turnSearchContextCache) rememberParamResolution(ctx context.Context, result core.ParamResolutionResult) {
	if c == nil {
		return
	}
	key := turnSearchContextKey(ctx)
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolutions[key] = cloneParamResolutionResult(result)
}

func (c *turnSearchContextCache) enrichParamResolutionRequest(ctx context.Context, req core.ResolveOpsManualParamsRequest) core.ResolveOpsManualParamsRequest {
	if c == nil {
		return enrichResolveRequestFromExecutionContext(ctx, req)
	}
	key := turnSearchContextKey(ctx)
	if key == "" {
		return enrichResolveRequestFromExecutionContext(ctx, req)
	}
	c.mu.Lock()
	result, ok := c.results[key]
	c.mu.Unlock()
	if !ok {
		return enrichResolveRequestFromExecutionContext(ctx, req)
	}
	if operationFrameEmpty(req.OperationFrame) {
		req.OperationFrame = cloneOperationFrame(result.OperationFrame)
	}
	if strings.TrimSpace(req.RequestText) == "" {
		req.RequestText = result.OperationFrame.RawText
	}
	if strings.TrimSpace(req.ManualID) == "" && len(result.Manuals) > 0 {
		req.ManualID = result.Manuals[0].Manual.ID
	}
	if strings.TrimSpace(req.WorkflowID) == "" && len(result.Manuals) > 0 {
		req.WorkflowID = result.Manuals[0].BoundWorkflowID
		if req.WorkflowID == "" {
			req.WorkflowID = result.Manuals[0].Manual.WorkflowRef.WorkflowID
		}
	}
	if req.KnownParams == nil {
		req.KnownParams = map[string]any{}
	}
	if len(result.Manuals) > 0 {
		for key, value := range parametersFromSearchContext(result.OperationFrame, result.Manuals[0].Manual) {
			if !valuePresent(req.KnownParams[key]) {
				req.KnownParams[key] = value
			}
		}
	}
	return enrichResolveRequestFromExecutionContext(ctx, req)
}

func enrichSearchRequestFromExecutionContext(ctx context.Context, req core.SearchOpsManualsRequest) core.SearchOpsManualsRequest {
	execCtx, ok := tooling.ToolExecutionContextFrom(ctx)
	if !ok {
		return req
	}
	hostID := strings.TrimSpace(execCtx.HostID)
	if hostID == "" {
		return req
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	ensureHostMetadata(req.Metadata, hostID)
	if operationFrameEmpty(req.OperationFrame) {
		return req
	}
	if req.OperationFrame.Metadata == nil {
		req.OperationFrame.Metadata = map[string]any{}
	}
	ensureHostMetadata(req.OperationFrame.Metadata, hostID)
	if len(req.OperationFrame.TargetScope.Hosts) == 0 {
		req.OperationFrame.TargetScope.Hosts = []string{hostID}
	}
	return req
}

func enrichResolveRequestFromExecutionContext(ctx context.Context, req core.ResolveOpsManualParamsRequest) core.ResolveOpsManualParamsRequest {
	execCtx, ok := tooling.ToolExecutionContextFrom(ctx)
	if !ok {
		return req
	}
	hostID := strings.TrimSpace(execCtx.HostID)
	if hostID == "" {
		return req
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	ensureHostMetadata(req.Metadata, hostID)
	if req.OperationFrame.Metadata == nil {
		req.OperationFrame.Metadata = map[string]any{}
	}
	ensureHostMetadata(req.OperationFrame.Metadata, hostID)
	if len(req.OperationFrame.TargetScope.Hosts) == 0 {
		req.OperationFrame.TargetScope.Hosts = []string{hostID}
	}
	if req.KnownParams == nil {
		req.KnownParams = map[string]any{}
	}
	if !valuePresent(req.KnownParams["target_host"]) {
		req.KnownParams["target_host"] = hostID
	}
	return req
}

func ensureHostMetadata(metadata map[string]any, hostID string) {
	if metadata == nil || strings.TrimSpace(hostID) == "" {
		return
	}
	for _, key := range []string{"selected_host", "current_host", "aiops.target.hostId"} {
		if !valuePresent(metadata[key]) {
			metadata[key] = hostID
		}
	}
}

func (c *turnSearchContextCache) enrichPreflightRequest(ctx context.Context, req core.PreflightRequest) core.PreflightRequest {
	if c == nil {
		return req
	}
	key := turnSearchContextKey(ctx)
	if key == "" {
		return req
	}
	c.mu.Lock()
	result, ok := c.results[key]
	c.mu.Unlock()
	if !ok {
		return req
	}
	manualID := strings.TrimSpace(req.ManualID)
	if manualID == "" {
		return req
	}
	var matched core.SearchManualHit
	for _, hit := range result.Manuals {
		if strings.TrimSpace(hit.Manual.ID) == manualID {
			matched = hit
			break
		}
	}
	if strings.TrimSpace(matched.Manual.ID) == "" {
		return req
	}
	if operationFrameEmpty(req.OperationFrame) {
		req.OperationFrame = cloneOperationFrame(result.OperationFrame)
	}
	if req.Parameters == nil {
		req.Parameters = map[string]any{}
	}
	c.mu.Lock()
	resolution, hasResolution := c.resolutions[key]
	c.mu.Unlock()
	if hasResolution && (manualID == "" || resolution.ManualID == manualID) {
		if operationFrameEmpty(req.OperationFrame) {
			req.OperationFrame = cloneOperationFrame(resolution.OperationFrame)
		}
		for _, param := range resolution.ResolvedParams {
			if !valuePresent(req.Parameters[param.ID]) {
				req.Parameters[param.ID] = param.Value
			}
			if param.ID == "target_instance" && !valuePresent(req.Parameters["target_instance"]) {
				req.Parameters["target_instance"] = param.Value
			}
		}
	}
	for key, value := range parametersFromSearchContext(result.OperationFrame, matched.Manual) {
		if !valuePresent(req.Parameters[key]) {
			req.Parameters[key] = value
		}
	}
	return req
}

func cloneParamResolutionResult(in core.ParamResolutionResult) core.ParamResolutionResult {
	var out core.ParamResolutionResult
	raw, err := json.Marshal(in)
	if err != nil {
		return in
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return in
	}
	return out
}

func turnSearchContextKey(ctx context.Context) string {
	execCtx, ok := tooling.ToolExecutionContextFrom(ctx)
	if !ok {
		return ""
	}
	sessionID := strings.TrimSpace(execCtx.SessionID)
	turnID := strings.TrimSpace(execCtx.TurnID)
	if sessionID == "" || turnID == "" {
		return ""
	}
	return sessionID + "\x00" + turnID
}

func parametersFromSearchContext(frame core.OperationFrame, manual core.OpsManual) map[string]any {
	out := map[string]any{}
	for key, value := range frame.RequiredParams {
		if valuePresent(value) {
			out[key] = value
		}
	}
	if strings.TrimSpace(frame.Target.Name) != "" {
		out["target_instance"] = strings.TrimSpace(frame.Target.Name)
	}
	if !valuePresent(out["target_instance"]) && searchContextTargetIsHost(frame, manual) && len(frame.TargetScope.Hosts) > 0 && strings.TrimSpace(frame.TargetScope.Hosts[0]) != "" {
		out["target_instance"] = strings.TrimSpace(frame.TargetScope.Hosts[0])
	}
	for _, evidence := range frame.Evidence.Provided {
		evidence = strings.TrimSpace(evidence)
		if evidence != "" {
			out[evidence] = true
		}
	}
	for _, required := range manual.RequiredContext.RequiredInputs {
		required = strings.TrimSpace(required)
		if required == "" || valuePresent(out[required]) {
			continue
		}
		if valuePresent(frame.Metadata[required]) {
			out[required] = frame.Metadata[required]
		}
	}
	return out
}

func searchContextTargetIsHost(frame core.OperationFrame, manual core.OpsManual) bool {
	targetType := strings.TrimSpace(firstNonEmptySearchContext(frame.ObjectType, frame.Target.Type, frame.Operation.TargetType, manual.Operation.TargetType, manual.Applicability.Middleware))
	return strings.EqualFold(targetType, "host") || strings.EqualFold(targetType, "vm")
}

func firstNonEmptySearchContext(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func operationFrameEmpty(frame core.OperationFrame) bool {
	return strings.TrimSpace(frame.RawText) == "" &&
		strings.TrimSpace(frame.Target.Type) == "" &&
		strings.TrimSpace(frame.Target.Name) == "" &&
		strings.TrimSpace(frame.Operation.Action) == "" &&
		strings.TrimSpace(frame.Operation.TargetType) == "" &&
		strings.TrimSpace(frame.Environment.OS) == "" &&
		strings.TrimSpace(frame.Environment.Platform) == "" &&
		strings.TrimSpace(frame.Environment.ExecutionSurface) == "" &&
		len(frame.TargetScope.Hosts) == 0 &&
		len(frame.RequiredParams) == 0 &&
		len(frame.Metadata) == 0 &&
		len(frame.Evidence.Provided) == 0 &&
		len(frame.Evidence.Missing) == 0
}

func cloneSearchOpsManualsResult(in core.SearchOpsManualsResult) core.SearchOpsManualsResult {
	var out core.SearchOpsManualsResult
	raw, err := json.Marshal(in)
	if err != nil {
		return in
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return in
	}
	return out
}

func cloneOperationFrame(in core.OperationFrame) core.OperationFrame {
	var out core.OperationFrame
	raw, err := json.Marshal(in)
	if err != nil {
		return in
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return in
	}
	return out
}

func valuePresent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}
