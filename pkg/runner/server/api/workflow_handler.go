package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"runner/server/service"
	"runner/workflow"
)

type workflowHandler struct {
	svc *service.WorkflowService
}

func NewWorkflowHandler(svc *service.WorkflowService) WorkflowHandler {
	return &workflowHandler{svc: svc}
}

func (h *workflowHandler) List(w http.ResponseWriter, r *http.Request) {
	labels := parseLabelsQuery(strings.TrimSpace(r.URL.Query().Get("labels")))
	items, err := h.svc.List(r.Context(), labels)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *workflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	item, err := h.svc.Get(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	parsed, _ := workflow.Load(item.RawYAML)
	writeJSON(w, http.StatusOK, map[string]any{
		"name":                  item.Name,
		"description":           item.Description,
		"version":               item.Version,
		"labels":                item.Labels,
		"save_note":             item.SaveNote,
		"status":                item.Status,
		"validated_graph_hash":  item.ValidatedGraphHash,
		"validated_layout_hash": item.ValidatedLayoutHash,
		"validated_at":          item.ValidatedAt,
		"validated_by":          item.ValidatedBy,
		"dry_run_graph_hash":    item.DryRunGraphHash,
		"dry_run_layout_hash":   item.DryRunLayoutHash,
		"dry_run_at":            item.DryRunAt,
		"dry_run_by":            item.DryRunBy,
		"published_graph_hash":  item.PublishedGraphHash,
		"published_at":          item.PublishedAt,
		"created_at":            item.CreatedAt,
		"updated_at":            item.UpdatedAt,
		"yaml":                  string(item.RawYAML),
		"parsed":                parsed,
	})
}

func (h *workflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		YAML        string            `json:"yaml"`
		Labels      map[string]string `json:"labels"`
		SaveNote    *string           `json:"save_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := h.svc.Create(r.Context(), &service.WorkflowRecord{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		RawYAML:     []byte(req.YAML),
		Labels:      req.Labels,
		SaveNote:    derefString(req.SaveNote),
		SaveNoteSet: req.SaveNote != nil,
	})
	if err != nil {
		if writeWorkflowGuardError(w, err) {
			return
		}
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.create", req.Name, map[string]any{
		"name":        req.Name,
		"description": req.Description,
		"labels":      req.Labels,
		"save_note":   derefString(req.SaveNote),
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"name":      req.Name,
		"save_note": derefString(req.SaveNote),
		"status":    service.WorkflowStatusDraft,
	})
}

func (h *workflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req struct {
		Description string            `json:"description"`
		YAML        string            `json:"yaml"`
		Labels      map[string]string `json:"labels"`
		SaveNote    *string           `json:"save_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := h.svc.Update(r.Context(), name, &service.WorkflowRecord{
		Name:        name,
		Description: req.Description,
		RawYAML:     []byte(req.YAML),
		Labels:      req.Labels,
		SaveNote:    derefString(req.SaveNote),
		SaveNoteSet: req.SaveNote != nil,
	})
	if err != nil {
		if writeWorkflowGuardError(w, err) {
			return
		}
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.update", name, map[string]any{
		"name":        name,
		"description": req.Description,
		"labels":      req.Labels,
		"save_note":   derefString(req.SaveNote),
	})
	warnings, _ := h.svc.WorkflowReferenceWarnings(r.Context(), name)
	writeJSON(w, http.StatusOK, map[string]any{
		"name":      name,
		"save_note": derefString(req.SaveNote),
		"status":    service.WorkflowStatusDraft,
		"warnings":  warnings,
	})
}

func (h *workflowHandler) Publish(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req struct {
		SaveNote            *string `json:"save_note"`
		RiskAcknowledged    bool    `json:"risk_acknowledged"`
		WarningAcknowledged bool    `json:"warning_acknowledged"`
		ValidatedGraphHash  string  `json:"validated_graph_hash"`
		DryRunGraphHash     string  `json:"dry_run_graph_hash"`
		Diff                any     `json:"diff"`
		RiskSummary         any     `json:"risk_summary"`
		ValidationResult    any     `json:"validation_result"`
		AIDraftConfirmed    bool    `json:"ai_draft_confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := h.svc.Publish(r.Context(), name, service.WorkflowPublishOptions{
		SaveNote:            derefString(req.SaveNote),
		SaveNoteSet:         req.SaveNote != nil,
		RiskAcknowledged:    req.RiskAcknowledged,
		WarningAcknowledged: req.WarningAcknowledged,
		ValidatedGraphHash:  req.ValidatedGraphHash,
		DryRunGraphHash:     req.DryRunGraphHash,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditPayload := map[string]any{
		"name":                 name,
		"save_note":            derefString(req.SaveNote),
		"risk_acknowledged":    req.RiskAcknowledged,
		"warning_acknowledged": req.WarningAcknowledged,
		"ai_draft_confirmed":   req.AIDraftConfirmed,
		"status":               record.Status,
		"validated_graph_hash": record.ValidatedGraphHash,
		"dry_run_graph_hash":   record.DryRunGraphHash,
		"published_graph_hash": record.PublishedGraphHash,
	}
	if req.ValidatedGraphHash != "" {
		auditPayload["review_validated_graph_hash"] = req.ValidatedGraphHash
	}
	if req.DryRunGraphHash != "" {
		auditPayload["review_dry_run_graph_hash"] = req.DryRunGraphHash
	}
	if req.Diff != nil {
		auditPayload["diff"] = req.Diff
	}
	if req.RiskSummary != nil {
		auditPayload["risk_summary"] = req.RiskSummary
	}
	if req.ValidationResult != nil {
		auditPayload["validation_result"] = req.ValidationResult
	}
	auditLog(r, "workflow.publish", name, auditPayload)
	writeJSON(w, http.StatusOK, map[string]any{
		"name":                  record.Name,
		"status":                record.Status,
		"save_note":             record.SaveNote,
		"validated_graph_hash":  record.ValidatedGraphHash,
		"validated_layout_hash": record.ValidatedLayoutHash,
		"validated_at":          record.ValidatedAt,
		"validated_by":          record.ValidatedBy,
		"dry_run_graph_hash":    record.DryRunGraphHash,
		"dry_run_layout_hash":   record.DryRunLayoutHash,
		"dry_run_at":            record.DryRunAt,
		"dry_run_by":            record.DryRunBy,
		"published_graph_hash":  record.PublishedGraphHash,
		"published_at":          record.PublishedAt,
		"updated_at":            record.UpdatedAt,
	})
}

func (h *workflowHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	items, err := h.svc.ListVersions(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": workflowVersionResponses(items),
	})
}

func (h *workflowHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	versionID := strings.TrimSpace(r.PathValue("version_id"))
	version, err := h.svc.GetVersion(r.Context(), name, versionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, workflowVersionResponse(version))
}

func (h *workflowHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	versionID := strings.TrimSpace(r.PathValue("version_id"))
	var req struct {
		SaveNote string `json:"save_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := h.svc.Rollback(r.Context(), name, versionID, service.WorkflowRollbackOptions{SaveNote: req.SaveNote})
	if err != nil {
		if writeWorkflowGuardError(w, err) {
			return
		}
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.rollback", name, map[string]any{
		"name":       name,
		"version_id": versionID,
		"save_note":  req.SaveNote,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"name":       record.Name,
		"status":     record.Status,
		"save_note":  record.SaveNote,
		"updated_at": record.UpdatedAt,
		"yaml":       string(record.RawYAML),
	})
}

func (h *workflowHandler) ExportBundle(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	bundle, err := h.svc.ExportBundle(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.bundle.export", name, map[string]any{
		"name":           name,
		"bundle_version": bundle.BundleVersion,
		"versions_count": len(bundle.Versions),
	})
	writeJSON(w, http.StatusOK, bundle)
}

func (h *workflowHandler) ImportBundle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bundle    service.WorkflowBundle `json:"bundle"`
		Overwrite bool                   `json:"overwrite"`
		SaveNote  string                 `json:"save_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := h.svc.ImportBundle(r.Context(), &req.Bundle, service.WorkflowImportOptions{
		Overwrite: req.Overwrite,
		SaveNote:  req.SaveNote,
	})
	if err != nil {
		if writeWorkflowGuardError(w, err) {
			return
		}
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.bundle.import", record.Name, map[string]any{
		"name":           record.Name,
		"overwrite":      req.Overwrite,
		"save_note":      req.SaveNote,
		"versions_count": len(req.Bundle.Versions),
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"name":       record.Name,
		"status":     record.Status,
		"save_note":  record.SaveNote,
		"updated_at": record.UpdatedAt,
		"yaml":       string(record.RawYAML),
	})
}

func (h *workflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if err := h.svc.Delete(r.Context(), name); err != nil {
		if writeWorkflowGuardError(w, err) {
			return
		}
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.delete", name, map[string]any{"name": name})
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
	})
}

func (h *workflowHandler) Validate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	record, err := h.svc.ValidateWorkflow(r.Context(), name, service.WorkflowValidateOptions{Actor: actorFromRequest(r)})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.validate", name, map[string]any{
		"name":                  name,
		"status":                record.Status,
		"validated_graph_hash":  record.ValidatedGraphHash,
		"validated_layout_hash": record.ValidatedLayoutHash,
		"validated_by":          record.ValidatedBy,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"name":                  name,
		"valid":                 true,
		"errors":                []string{},
		"status":                record.Status,
		"validated_graph_hash":  record.ValidatedGraphHash,
		"validated_layout_hash": record.ValidatedLayoutHash,
		"validated_at":          record.ValidatedAt,
		"validated_by":          record.ValidatedBy,
	})
}

func (h *workflowHandler) DryRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAML string         `json:"yaml"`
		Vars map[string]any `json:"vars"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	yamlContent := strings.TrimSpace(req.YAML)
	if yamlContent == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"valid": false,
			"errors": []map[string]any{
				{
					"type":       "validation",
					"message":    "yaml is required",
					"suggestion": "请提供工作流 YAML 内容。",
				},
			},
			"summary": "未提供工作流 YAML。",
		})
		return
	}

	wf, err := workflow.Load([]byte(yamlContent))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"errors": []map[string]any{
				{
					"type":       "parse",
					"message":    err.Error(),
					"suggestion": "请检查 YAML 语法与缩进。",
				},
			},
			"summary": "YAML 解析失败。",
		})
		return
	}
	if err := wf.Validate(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid":         false,
			"workflow_name": wf.Name,
			"steps_count":   len(wf.Steps),
			"target_hosts":  collectDryRunTargets(wf),
			"actions_used":  collectDryRunActions(wf),
			"agents_status": map[string]any{},
			"warnings":      collectDryRunWarnings(wf),
			"errors": []map[string]any{
				{
					"type":       "validation",
					"message":    err.Error(),
					"suggestion": "请补齐必须字段并检查步骤定义。",
				},
			},
			"summary": "工作流校验未通过。",
		})
		return
	}

	targetHosts := collectDryRunTargets(wf)
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":         true,
		"workflow_name": wf.Name,
		"steps_count":   len(wf.Steps),
		"target_hosts":  targetHosts,
		"actions_used":  collectDryRunActions(wf),
		"agents_status": map[string]any{},
		"warnings":      collectDryRunWarnings(wf),
		"errors":        []map[string]any{},
		"summary":       buildDryRunSummary(wf.Name, len(wf.Steps), len(targetHosts)),
	})
}

func parseLabelsQuery(raw string) map[string]string {
	parts := strings.Split(raw, ",")
	out := map[string]string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeWorkflowGuardError(w http.ResponseWriter, err error) bool {
	var coded interface {
		Code() string
		Message() string
		WorkflowReferences() []service.WorkflowReference
	}
	if !errors.As(err, &coded) {
		return false
	}
	payload := map[string]any{
		"error":   coded.Code(),
		"message": coded.Message(),
	}
	if refs := coded.WorkflowReferences(); len(refs) > 0 {
		payload["references"] = refs
	}
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, service.ErrInvalid):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, payload)
	return true
}

func workflowVersionResponses(items []*service.WorkflowVersionRecord) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, workflowVersionResponse(item))
	}
	return out
}

func workflowVersionResponse(item *service.WorkflowVersionRecord) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":           item.ID,
		"name":         item.Name,
		"description":  item.Description,
		"version":      item.Version,
		"status":       item.Status,
		"save_note":    item.SaveNote,
		"reason":       item.Reason,
		"checksum":     item.Checksum,
		"yaml":         string(item.RawYAML),
		"published_at": item.PublishedAt,
		"created_at":   item.CreatedAt,
	}
}

func collectDryRunTargets(wf workflow.Workflow) []string {
	targets := map[string]struct{}{}
	for _, step := range wf.Steps {
		for _, target := range step.Targets {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			targets[target] = struct{}{}
		}
	}
	if len(targets) == 0 {
		for host := range wf.Inventory.ResolveHosts() {
			targets[host] = struct{}{}
		}
	}
	items := make([]string, 0, len(targets))
	for target := range targets {
		items = append(items, target)
	}
	return items
}

func collectDryRunActions(wf workflow.Workflow) []string {
	actions := map[string]struct{}{}
	for _, step := range wf.Steps {
		action := strings.TrimSpace(step.Action)
		if action == "" {
			continue
		}
		actions[action] = struct{}{}
	}
	items := make([]string, 0, len(actions))
	for action := range actions {
		items = append(items, action)
	}
	return items
}

func collectDryRunWarnings(wf workflow.Workflow) []string {
	hostSet := wf.Inventory.ResolveHosts()
	warnings := make([]string, 0)
	for _, step := range wf.Steps {
		if len(step.Targets) == 0 {
			warnings = append(warnings, "步骤 "+step.Name+" 未声明 targets，执行范围需在运行时进一步确认。")
			continue
		}
		for _, target := range step.Targets {
			target = strings.TrimSpace(target)
			if target == "" {
				continue
			}
			if target == "local" {
				continue
			}
			if _, ok := hostSet[target]; !ok {
				warnings = append(warnings, "目标 "+target+" 未在 inventory 中显式声明，将按运行时地址解析。")
			}
		}
	}
	return warnings
}

func buildDryRunSummary(name string, stepsCount, targetCount int) string {
	workflowName := strings.TrimSpace(name)
	if workflowName == "" {
		workflowName = "未命名工作流"
	}
	return workflowName + " 校验通过，包含 " + strconv.Itoa(stepsCount) + " 个步骤，覆盖 " + strconv.Itoa(targetCount) + " 个目标对象。"
}
