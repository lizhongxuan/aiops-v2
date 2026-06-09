package runtimekernel

import (
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

type ContextUsage = promptinput.ContextUsage
type ContextUsageCategory = promptinput.ContextUsageCategory
type ContextContributor = promptinput.ContextContributor

type ContextUsageInput struct {
	Messages   []*schema.Message
	Compiled   promptcompiler.CompiledPrompt
	Governance []ContextGovernanceEvent
}

func AnalyzeContextUsage(input ContextUsageInput) ContextUsage {
	acc := newContextUsageAccumulator()
	acc.addText("system", input.Compiled.Stable.System.Content, "compiled.system")
	acc.addText("developer", input.Compiled.Stable.Developer.Content, "compiled.developer")
	acc.addText("tools", input.Compiled.Stable.Tools.Content, "compiled.tools")
	for i, asset := range input.Compiled.Dynamic.SkillPromptAssets {
		acc.addText("skills", asset, "skill")
		_ = i
	}
	for _, section := range input.Compiled.Dynamic.ExtraSections {
		category := "messages"
		title := strings.ToLower(section.Title)
		switch {
		case strings.Contains(title, "mcp") || strings.Contains(title, "resource"):
			category = "mcp"
		case strings.Contains(title, "artifact") || strings.Contains(title, "reference"):
			category = "artifacts"
		}
		acc.addText(category, section.Content, section.Title)
	}
	for _, event := range input.Governance {
		if event.Budget.MaxContextTokens > 0 || event.Budget.ReservedOutputTokens > 0 || event.Budget.EffectiveContextWindow > 0 {
			acc.maxContextTokens = firstPositiveInt(acc.maxContextTokens, event.Budget.MaxContextTokens)
			acc.reservedOutputTokens = firstPositiveInt(acc.reservedOutputTokens, event.Budget.ReservedOutputTokens)
			acc.addText("buffers", "context budget thresholds", event.Kind)
		}
	}
	for _, msg := range input.Messages {
		if msg == nil {
			continue
		}
		category := "messages"
		id := string(msg.Role)
		if msg.Role == schema.Tool {
			category = "tool_results"
			id = strings.TrimSpace(msg.ToolCallID)
			if id == "" {
				id = "tool_result"
			}
		}
		acc.addText(category, msg.Content, id)
	}
	return acc.result()
}

type contextUsageAccumulator struct {
	categories           map[string]*ContextUsageCategory
	contributors         []ContextContributor
	maxContextTokens     int
	reservedOutputTokens int
}

func newContextUsageAccumulator() *contextUsageAccumulator {
	acc := &contextUsageAccumulator{categories: map[string]*ContextUsageCategory{}}
	for _, name := range []string{"system", "developer", "tools", "skills", "mcp", "messages", "tool_results", "artifacts", "buffers"} {
		acc.categories[name] = &ContextUsageCategory{Name: name}
	}
	return acc
}

func (a *contextUsageAccumulator) addText(category, content, id string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	a.addCategory(category, len([]byte(content)), estimateTextTokens(content), 1)
	a.contributors = append(a.contributors, ContextContributor{
		Kind:           category,
		ID:             sanitizeContributorID(id, category),
		Bytes:          len([]byte(content)),
		TokensEstimate: estimateTextTokens(content),
		Action:         contextUsageContributorAction(category),
	})
}

func (a *contextUsageAccumulator) addCategory(category string, bytes, tokens, items int) {
	if category == "" {
		category = "messages"
	}
	entry, ok := a.categories[category]
	if !ok {
		entry = &ContextUsageCategory{Name: category}
		a.categories[category] = entry
	}
	entry.Bytes += bytes
	entry.TokensEstimate += tokens
	entry.Items += items
}

func (a *contextUsageAccumulator) result() ContextUsage {
	names := make([]string, 0, len(a.categories))
	for name := range a.categories {
		names = append(names, name)
	}
	sort.Strings(names)
	categories := make([]ContextUsageCategory, 0, len(names))
	total := 0
	for _, name := range names {
		category := *a.categories[name]
		total += category.TokensEstimate
		categories = append(categories, category)
	}
	sort.SliceStable(a.contributors, func(i, j int) bool {
		return a.contributors[i].TokensEstimate > a.contributors[j].TokensEstimate
	})
	top := a.contributors
	if len(top) > 8 {
		top = top[:8]
	}
	return ContextUsage{
		MaxContextTokens:     a.maxContextTokens,
		ReservedOutputTokens: a.reservedOutputTokens,
		EstimatedInputTokens: total,
		Categories:           categories,
		TopContributors:      append([]ContextContributor(nil), top...),
	}
}

func estimateTextTokens(content string) int {
	return EstimateTokens(Message{Role: "system", Content: content})
}

func sanitizeContributorID(id, fallback string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return fallback
	}
	if len(id) > 80 {
		id = id[:80]
	}
	id = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.' || r == ':':
			return r
		default:
			return '-'
		}
	}, id)
	id = strings.Trim(id, "-")
	if id == "" {
		return fallback
	}
	return id
}

func contextUsageContributorAction(category string) string {
	switch category {
	case "tool_results":
		return "inspect_budget"
	case "tools":
		return "consider_deferred_loading"
	case "artifacts":
		return "read_by_reference"
	default:
		return "keep_inline"
	}
}

func firstPositiveInt(current, next int) int {
	if current > 0 {
		return current
	}
	if next > 0 {
		return next
	}
	return 0
}
