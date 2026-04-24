package api

import (
	"encoding/json"
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
		"name":        item.Name,
		"description": item.Description,
		"version":     item.Version,
		"labels":      item.Labels,
		"created_at":  item.CreatedAt,
		"updated_at":  item.UpdatedAt,
		"yaml":        string(item.RawYAML),
		"parsed":      parsed,
	})
}

func (h *workflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		YAML        string            `json:"yaml"`
		Labels      map[string]string `json:"labels"`
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
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.create", req.Name, map[string]any{
		"name":        req.Name,
		"description": req.Description,
		"labels":      req.Labels,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"name": req.Name,
	})
}

func (h *workflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var req struct {
		Description string            `json:"description"`
		YAML        string            `json:"yaml"`
		Labels      map[string]string `json:"labels"`
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
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.update", name, map[string]any{
		"name":        name,
		"description": req.Description,
		"labels":      req.Labels,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"name": name,
	})
}

func (h *workflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if err := h.svc.Delete(r.Context(), name); err != nil {
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
	item, err := h.svc.Get(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if err := h.svc.Validate(r.Context(), item.RawYAML); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":   name,
		"valid":  true,
		"errors": []string{},
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
