package selfopt

import (
	"sort"
	"strings"
)

func BuildImpactMatrix(changed []string, cases []Case) ImpactMatrix {
	tagSet := map[string]bool{}
	for _, path := range changed {
		for _, tag := range tagsForPath(path) {
			tagSet[tag] = true
		}
	}
	fullSuite := len(changed) == 0 || len(tagSet) == 0
	if fullSuite {
		for _, c := range cases {
			for _, tag := range c.Metadata.AreaTags {
				tagSet[tag] = true
			}
		}
	}
	matched := sortedKeys(tagSet)
	var selected, skipped []string
	for _, c := range cases {
		if fullSuite || intersects(c.Metadata.AreaTags, matched) {
			selected = append(selected, c.ID)
		} else {
			skipped = append(skipped, c.ID)
		}
	}
	sort.Strings(selected)
	sort.Strings(skipped)
	return ImpactMatrix{
		ChangedFiles:    append([]string(nil), changed...),
		MatchedAreaTags: matched,
		SelectedCaseIDs: selected,
		SkippedCaseIDs:  skipped,
		FullSuite:       fullSuite,
	}
}

func tagsForPath(path string) []string {
	switch {
	case strings.HasPrefix(path, "internal/runtimekernel/"),
		strings.HasPrefix(path, "internal/tooling/"),
		strings.HasPrefix(path, "internal/policyengine/"),
		strings.HasPrefix(path, "internal/permissions/"):
		return []string{"approval", "runner", "tool-lifecycle"}
	case strings.HasPrefix(path, "internal/promptcompiler/"),
		strings.Contains(path, "developer_rules"),
		strings.Contains(path, "SKILL.md"):
		return []string{"prompt"}
	case strings.HasPrefix(path, "internal/opsmanual/"),
		strings.HasPrefix(path, "internal/integrations/opsmanuals/"):
		return []string{"opsmanual"}
	case strings.HasPrefix(path, "pkg/runner/"):
		return []string{"runner"}
	case strings.HasPrefix(path, "internal/integrations/rca/"),
		strings.HasPrefix(path, "internal/integrations/coroot/"):
		return []string{"rca"}
	case strings.HasPrefix(path, "internal/memory/"):
		return []string{"memory", "learning"}
	case strings.HasPrefix(path, "internal/appui/"),
		strings.HasPrefix(path, "internal/server/"):
		return []string{"transport", "chat-ui"}
	case strings.HasPrefix(path, "web/src/chat/"):
		return []string{"chat-ui"}
	case strings.HasPrefix(path, "internal/modelrouter/"),
		strings.HasPrefix(path, "internal/modeltrace/"):
		return []string{"llm", "prompt"}
	default:
		return nil
	}
}

func intersects(left, right []string) bool {
	seen := map[string]bool{}
	for _, value := range left {
		seen[value] = true
	}
	for _, value := range right {
		if seen[value] {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
