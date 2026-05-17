package opsmanual

import (
	"fmt"
	"strings"
)

func BuildParamRequirements(manual OpsManual, workflowParams []ParamRequirement) []ParamRequirement {
	merged := map[string]ParamRequirement{}
	order := []string{}
	add := func(req ParamRequirement, override bool) {
		req.ID = strings.TrimSpace(req.ID)
		if req.ID == "" {
			return
		}
		if req.Type == "" {
			req.Type = NormalizeParamType(req.ID, "")
		} else {
			req.Type = NormalizeParamType(req.ID, req.Type)
		}
		if req.Label == "" {
			req.Label = DefaultParamLabel(req.ID)
		}
		if req.UIControl == "" {
			req.UIControl = DefaultParamUIControl(req)
		}
		if _, ok := merged[req.ID]; !ok {
			order = append(order, req.ID)
			merged[req.ID] = req
			return
		}
		if override {
			merged[req.ID] = mergeParamRequirement(merged[req.ID], req)
		}
	}

	add(ParamRequirement{
		ID:            "target_host",
		Label:         "目标位置",
		Type:          "host_ref",
		Required:      true,
		DefaultSource: "selected_host",
		ResolverHints: []string{"selected_host", "conversation"},
		AskUserWhen:   []string{"no_candidate"},
	}, true)
	for _, req := range workflowParams {
		add(req, false)
	}
	for _, input := range manual.RequiredContext.RequiredInputs {
		add(ParamRequirement{
			ID:            strings.TrimSpace(input),
			Required:      true,
			ResolverHints: []string{"conversation", "host_readonly", "docker"},
			AskUserWhen:   []string{"no_candidate", "ambiguous"},
		}, true)
	}
	for name, rule := range manual.ParameterRules {
		add(ParamRequirement{
			ID:            name,
			Type:          NormalizeParamType(name, rule.Validation),
			Required:      rule.Required,
			DefaultSource: rule.Source,
			DefaultValue:  rule.DefaultValue,
			ResolverHints: hintsForParameterRule(rule),
			AskUserWhen:   []string{"no_candidate", "ambiguous"},
		}, true)
	}
	for _, req := range paramRequirementsFromMetadata(manual.Metadata["param_requirements"]) {
		add(req, true)
	}

	out := make([]ParamRequirement, 0, len(order))
	for _, id := range order {
		req := merged[id]
		if req.ID == "" {
			continue
		}
		out = append(out, req)
	}
	return out
}

func NormalizeParamType(id, declared string) string {
	declared = strings.ToLower(strings.TrimSpace(declared))
	if declared != "" {
		switch declared {
		case "string":
			declared = "text"
		case "filepath", "file_path", "dir", "directory":
			declared = "path"
		}
	}
	lower := strings.ToLower(strings.TrimSpace(id))
	switch {
	case lower == "target_instance" || lower == "resource_ref" || strings.HasSuffix(lower, "_instance") || strings.HasSuffix(lower, "_service") || strings.HasSuffix(lower, "_pod"):
		return "resource_ref"
	case lower == "target_host" || strings.HasSuffix(lower, "_host"):
		return "host_ref"
	case lower == "execution_surface":
		return "execution_surface"
	case lower == "time_range":
		return "time_range"
	case strings.Contains(lower, "backup_path") || strings.HasSuffix(lower, "_path"):
		return "path"
	case strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "token"):
		return "secret_ref"
	case lower == "symptom_or_metric" || strings.Contains(lower, "symptom") || strings.Contains(lower, "metric"):
		return "text"
	case declared != "":
		return declared
	default:
		return "text"
	}
}

func DefaultParamLabel(id string) string {
	switch strings.TrimSpace(id) {
	case "target_host":
		return "目标位置"
	case "target_instance":
		return "实例/服务"
	case "redis_instance":
		return "Redis 实例"
	case "pg_instance":
		return "PostgreSQL 实例"
	case "mysql_instance":
		return "MySQL 实例"
	case "execution_surface":
		return "访问/执行入口"
	case "backup_path":
		return "备份路径"
	case "symptom_or_metric":
		return "现象/证据"
	default:
		return strings.ReplaceAll(strings.TrimSpace(id), "_", " ")
	}
}

func DefaultParamUIControl(req ParamRequirement) string {
	switch req.Type {
	case "resource_ref", "host_ref", "execution_surface":
		return "select"
	case "secret_ref":
		return "secret_ref"
	default:
		return "text"
	}
}

func mergeParamRequirement(base, override ParamRequirement) ParamRequirement {
	out := base
	if override.Label != "" {
		out.Label = override.Label
	}
	if override.Type != "" {
		out.Type = override.Type
	}
	if override.Required {
		out.Required = true
	}
	if override.Sensitive {
		out.Sensitive = true
	}
	if override.DefaultSource != "" {
		out.DefaultSource = override.DefaultSource
	}
	if override.DefaultValue != nil {
		out.DefaultValue = override.DefaultValue
	}
	if len(override.DependsOn) > 0 {
		out.DependsOn = cloneStrings(override.DependsOn)
	}
	if len(override.ResolverHints) > 0 {
		out.ResolverHints = cloneStrings(override.ResolverHints)
	}
	if len(override.AskUserWhen) > 0 {
		out.AskUserWhen = cloneStrings(override.AskUserWhen)
	}
	if override.UIControl != "" {
		out.UIControl = override.UIControl
	}
	return out
}

func hintsForParameterRule(rule ParameterRule) []string {
	switch strings.ToLower(strings.TrimSpace(rule.Source)) {
	case "user":
		return []string{"conversation", "manual_default"}
	case "selected_host":
		return []string{"selected_host"}
	case "run_record":
		return []string{"run_record"}
	default:
		return nil
	}
}

func paramRequirementsFromMetadata(raw any) []ParamRequirement {
	switch typed := raw.(type) {
	case []ParamRequirement:
		return cloneParamRequirements(typed)
	case []any:
		out := []ParamRequirement{}
		for _, item := range typed {
			if req, ok := paramRequirementFromMap(item); ok {
				out = append(out, req)
			}
		}
		return out
	case map[string]any:
		out := []ParamRequirement{}
		for key, item := range typed {
			if req, ok := paramRequirementFromMap(item); ok {
				if req.ID == "" {
					req.ID = key
				}
				out = append(out, req)
			}
		}
		return out
	default:
		return nil
	}
}

func paramRequirementFromMap(raw any) (ParamRequirement, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return ParamRequirement{}, false
	}
	req := ParamRequirement{
		ID:            strings.TrimSpace(fmt.Sprint(firstAny(m["id"], m["name"]))),
		Label:         strings.TrimSpace(fmt.Sprint(m["label"])),
		Type:          strings.TrimSpace(fmt.Sprint(m["type"])),
		Required:      metadataBool(m, "required"),
		Sensitive:     metadataBool(m, "sensitive"),
		DefaultSource: strings.TrimSpace(fmt.Sprint(firstAny(m["default_source"], m["defaultSource"]))),
		DefaultValue:  firstAny(m["default_value"], m["defaultValue"]),
		DependsOn:     metadataStringSliceFromAny(firstAny(m["depends_on"], m["dependsOn"])),
		ResolverHints: metadataStringSliceFromAny(firstAny(m["resolver_hints"], m["resolverHints"])),
		AskUserWhen:   metadataStringSliceFromAny(firstAny(m["ask_user_when"], m["askUserWhen"])),
		UIControl:     strings.TrimSpace(fmt.Sprint(firstAny(m["ui_control"], m["uiControl"]))),
	}
	if req.DefaultValue == "" {
		req.DefaultValue = nil
	}
	return req, req.ID != ""
}
