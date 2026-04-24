package modules

import "strings"

const ExportPrefix = "BOPS_EXPORT:"

func ExportVarsEnabled(req Request) bool {
	if req.Step.Args == nil {
		return false
	}
	raw, ok := req.Step.Args["export_vars"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(v))
		switch trimmed {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off", "":
			return false
		default:
			return false
		}
	default:
		return false
	}
}

func ParseExportVars(output string) map[string]any {
	if strings.TrimSpace(output) == "" {
		return nil
	}
	exports := map[string]any{}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasPrefix(trimmed, ExportPrefix) {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, ExportPrefix))
		if payload == "" {
			continue
		}
		if strings.HasPrefix(payload, "{") && strings.HasSuffix(payload, "}") {
			payload = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(payload, "{"), "}"))
		}
		key, val, ok := splitKeyValue(payload)
		if !ok {
			continue
		}
		exports[key] = val
	}
	if len(exports) == 0 {
		return nil
	}
	return exports
}

func splitKeyValue(line string) (string, string, bool) {
	if strings.Contains(line, "=") {
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			return "", "", false
		}
		return key, val, true
	}
	if strings.Contains(line, ":") {
		parts := strings.SplitN(line, ":", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			return "", "", false
		}
		return key, val, true
	}
	return "", "", false
}
