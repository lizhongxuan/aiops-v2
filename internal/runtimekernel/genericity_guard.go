package runtimekernel

import (
	"path/filepath"
	"strings"
)

type GenericityFindingCategory string

const (
	GenericityBlockedCoreRule       GenericityFindingCategory = "blocked_core_rule"
	GenericityAllowedPluginMetadata GenericityFindingCategory = "allowed_plugin_metadata"
	GenericityAllowedTestFixture    GenericityFindingCategory = "allowed_test_fixture"
	GenericityAllowedUserFixture    GenericityFindingCategory = "allowed_user_fixture"
)

type GenericityFinding struct {
	Path     string                    `json:"path,omitempty"`
	Symbol   string                    `json:"symbol,omitempty"`
	Text     string                    `json:"text,omitempty"`
	Category GenericityFindingCategory `json:"category"`
	Reasons  []string                  `json:"reasons,omitempty"`
}

func ClassifyGenericityFinding(path, symbol, text string) GenericityFinding {
	normalizedPath := filepath.ToSlash(strings.TrimSpace(path))
	normalizedSymbol := strings.TrimSpace(symbol)
	finding := GenericityFinding{
		Path:   normalizedPath,
		Symbol: normalizedSymbol,
		Text:   strings.TrimSpace(text),
	}
	switch {
	case isUserFixtureGenericityPath(normalizedPath, normalizedSymbol):
		finding.Category = GenericityAllowedUserFixture
		finding.Reasons = []string{"candidate appears in user-provided fixture context"}
	case isTestFixtureGenericityPath(normalizedPath):
		finding.Category = GenericityAllowedTestFixture
		finding.Reasons = []string{"candidate appears in test or fixture context"}
	case isPluginMetadataGenericityPath(normalizedPath):
		finding.Category = GenericityAllowedPluginMetadata
		finding.Reasons = []string{"candidate appears in plugin, tool, or skill metadata"}
	default:
		finding.Category = GenericityBlockedCoreRule
		finding.Reasons = []string{"candidate appears in core rule context"}
	}
	return finding
}

func isUserFixtureGenericityPath(path, symbol string) bool {
	joined := strings.ToLower(path + "/" + symbol)
	return strings.Contains(joined, "user_fixture") ||
		strings.Contains(joined, "user_input") ||
		strings.Contains(joined, "user-provided")
}

func isTestFixtureGenericityPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "/testdata/") ||
		strings.HasSuffix(lower, "_test.go") ||
		strings.Contains(lower, "/fixtures/")
}

func isPluginMetadataGenericityPath(path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "/discovery_metadata") {
		return strings.Contains(lower, "internal/tooling/") ||
			strings.Contains(lower, "internal/skills/")
	}
	return strings.Contains(lower, "/plugins/") ||
		strings.Contains(lower, "/skills/") ||
		strings.Contains(lower, "toolmetadata") ||
		strings.Contains(lower, "skillmetadata")
}
