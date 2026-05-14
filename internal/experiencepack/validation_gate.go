package experiencepack

import (
	"fmt"
	"strings"
)

type ValidationTask struct {
	Validator            string         `json:"validator"`
	Args                 map[string]any `json:"args,omitempty"`
	TimeoutSeconds       int            `json:"timeout_seconds"`
	Mode                 string         `json:"mode"`
	RequiredEvidenceRefs []string       `json:"required_evidence_refs,omitempty"`
}

var allowedValidators = map[string]bool{
	"coroot.metric_check":   true,
	"coroot.trace_check":    true,
	"coroot.topology_check": true,
	"runner.readonly_probe": true,
	"http.synthetic_check":  true,
}

var dangerousValidationTokens = []string{"`", "$(", ";", "&", "|", ">", "<"}

func CompileValidation(gene GEPGene) ([]ValidationTask, error) {
	if len(gene.Validation) == 0 {
		return nil, fmt.Errorf("%w: validation is required", ErrValidationFailed)
	}
	tasks := make([]ValidationTask, 0, len(gene.Validation))
	for _, raw := range gene.Validation {
		task, err := CompileValidationString(raw)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func CheckValidationGate(gene GEPGene) ValidationReport {
	tasks, err := CompileValidation(gene)
	if err == nil {
		return ValidationReport{Passed: true, CompiledTasks: redactValidationTasks(tasks), Redacted: true}
	}
	return ValidationReport{Passed: false, BlockedReasons: []string{err.Error()}, CompiledTasks: []ValidationTask{}, Redacted: true}
}

func CompileValidationString(raw string) (ValidationTask, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ValidationTask{}, fmt.Errorf("%w: empty validation", ErrValidationFailed)
	}
	for _, token := range dangerousValidationTokens {
		if strings.Contains(value, token) {
			return ValidationTask{}, fmt.Errorf("%w: validation contains forbidden token %q", ErrValidationFailed, token)
		}
	}

	validator := value
	args := map[string]any{"expression": value}
	if idx := strings.Index(value, ":"); idx > 0 {
		validator = strings.TrimSpace(value[:idx])
		args = parseArgs(strings.TrimSpace(value[idx+1:]))
	}
	if !allowedValidators[validator] {
		return ValidationTask{}, fmt.Errorf("%w: validator %q is not allowed", ErrValidationFailed, validator)
	}
	return ValidationTask{
		Validator:      validator,
		Args:           args,
		TimeoutSeconds: 180,
		Mode:           "read_only",
	}, nil
}

func parseArgs(raw string) map[string]any {
	args := map[string]any{}
	for _, part := range strings.Split(raw, ",") {
		piece := strings.TrimSpace(part)
		if piece == "" {
			continue
		}
		key, value, ok := strings.Cut(piece, "=")
		if !ok {
			args["expression"] = piece
			continue
		}
		args[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return args
}

func redactValidationTasks(tasks []ValidationTask) []ValidationTask {
	result := make([]ValidationTask, len(tasks))
	for idx, task := range tasks {
		result[idx] = task
		result[idx].Args = redactValidationArgs(task.Args)
	}
	return result
}

func redactValidationArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	result := make(map[string]any, len(args))
	for key, value := range args {
		if validationArgSensitive(key) {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = value
	}
	return result
}

func validationArgSensitive(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "private_key")
}
