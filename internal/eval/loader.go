package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadCases reads all *.json files in dir and returns them sorted by id.
func LoadCases(dir string) ([]Case, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read cases dir: %w", err)
	}
	var cases []Case
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read case %s: %w", path, err)
		}
		var c Case
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("parse case %s: %w", path, err)
		}
		if err := validateCase(c, path); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].ID < cases[j].ID
	})
	return cases, nil
}

func validateCase(c Case, path string) error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("case %s: id is required", path)
	}
	if strings.TrimSpace(c.Category) == "" {
		return fmt.Errorf("case %s: category is required", path)
	}
	if strings.TrimSpace(c.Input) == "" {
		return fmt.Errorf("case %s: input is required", path)
	}
	return nil
}

// LoadReport reads a JSON report from disk.
func LoadReport(path string) (Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Report{}, fmt.Errorf("read report: %w", err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, fmt.Errorf("parse report: %w", err)
	}
	return report, nil
}

// SaveReport writes a JSON report to disk.
func SaveReport(path string, report Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
