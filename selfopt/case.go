package selfopt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadCases(dir string) ([]Case, error) {
	var paths []string
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	cases := make([]Case, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var c Case
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		c.applyDefaults()
		cases = append(cases, c)
	}
	return cases, nil
}

func (c *Case) applyDefaults() {
	if c.Metadata.CaseType == "" {
		c.Metadata.CaseType = "eval"
	}
	if c.Metadata.RiskLevel == "" {
		if strings.EqualFold(c.Priority, "P0") {
			c.Metadata.RiskLevel = "high"
		} else {
			c.Metadata.RiskLevel = "medium"
		}
	}
	if c.Metadata.BaselinePolicy == "" {
		if strings.EqualFold(c.Priority, "P0") {
			c.Metadata.BaselinePolicy = BaselineBlockOnRegression
		} else {
			c.Metadata.BaselinePolicy = BaselineObserve
		}
	}
	if c.BaselinePolicy == "" {
		c.BaselinePolicy = c.Metadata.BaselinePolicy
	}
	if len(c.Metadata.AreaTags) == 0 {
		c.Metadata.AreaTags = inferAreaTags(*c)
	}
	if len(c.Metadata.ScoreWeights) == 0 && len(c.ScoreWeights) > 0 {
		c.Metadata.ScoreWeights = c.ScoreWeights
	}
}

func inferAreaTags(c Case) []string {
	tags := map[string]bool{}
	category := strings.ToLower(c.Category + " " + c.ID + " " + c.Input)
	if strings.Contains(category, "memory") || strings.Contains(category, "learning") {
		tags["memory"] = true
		tags["learning"] = true
	}
	if strings.Contains(category, "rca") || strings.Contains(category, "coroot") {
		tags["rca"] = true
	}
	if strings.Contains(category, "k8s") || strings.Contains(category, "approval") {
		tags["approval"] = true
		tags["runner"] = true
	}
	for _, call := range c.Expected.ExpectedToolCalls {
		if strings.Contains(call, "search_ops_manuals") {
			tags["opsmanual"] = true
		}
	}
	if len(tags) == 0 {
		tags["prompt"] = true
	}
	out := make([]string, 0, len(tags))
	for tag := range tags {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}
