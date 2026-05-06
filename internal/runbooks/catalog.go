package runbooks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Catalog struct {
	runbooks map[string]Runbook
}

func NewCatalog(items []Runbook) *Catalog {
	catalog := &Catalog{runbooks: map[string]Runbook{}}
	for _, rb := range items {
		rb.ID = strings.TrimSpace(rb.ID)
		if rb.ID == "" {
			continue
		}
		catalog.runbooks[rb.ID] = rb
	}
	return catalog
}

func LoadCatalog(pattern string) (*Catalog, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("runbooks: no files match %s", pattern)
	}
	catalog := &Catalog{runbooks: map[string]Runbook{}}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rb, err := parseRunbook(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		catalog.runbooks[rb.ID] = rb
	}
	return catalog, nil
}

func LoadCatalogBytes(data []byte) (*Catalog, error) {
	rb, err := parseRunbook(data)
	if err != nil {
		return nil, err
	}
	return &Catalog{runbooks: map[string]Runbook{rb.ID: rb}}, nil
}

func (c *Catalog) Get(id string) (Runbook, bool) {
	if c == nil {
		return Runbook{}, false
	}
	rb, ok := c.runbooks[strings.TrimSpace(id)]
	return rb, ok
}

func (c *Catalog) List() []Runbook {
	if c == nil {
		return nil
	}
	out := make([]Runbook, 0, len(c.runbooks))
	for _, rb := range c.runbooks {
		out = append(out, rb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (c *Catalog) Match(req MatchRequest) []Candidate {
	if c == nil {
		return nil
	}
	type scored struct {
		candidate Candidate
	}
	var out []Candidate
	for _, rb := range c.runbooks {
		score, reasons := matchScore(rb, req)
		if score == 0 {
			continue
		}
		out = append(out, Candidate{Runbook: rb, Score: score, Reason: strings.Join(reasons, "; ")})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Runbook.ID < out[j].Runbook.ID
		}
		return out[i].Score > out[j].Score
	})
	if req.Limit > 0 && req.Limit < len(out) {
		out = out[:req.Limit]
	}
	return out
}

func parseRunbook(data []byte) (Runbook, error) {
	var rb Runbook
	if err := yaml.Unmarshal(data, &rb); err != nil {
		return Runbook{}, err
	}
	rb.ID = strings.TrimSpace(rb.ID)
	if rb.ID == "" {
		return Runbook{}, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(rb.Name) == "" {
		rb.Name = rb.ID
	}
	if rb.Risk == "" {
		rb.Risk = "medium"
	}
	if len(rb.Steps) == 0 {
		return Runbook{}, fmt.Errorf("steps are required")
	}
	for i := range rb.Steps {
		step := &rb.Steps[i]
		if strings.TrimSpace(step.ID) == "" {
			return Runbook{}, fmt.Errorf("steps[%d].id is required", i)
		}
		if strings.TrimSpace(step.Tool) == "" {
			return Runbook{}, fmt.Errorf("steps[%d].tool is required", i)
		}
		if step.Risk == "" {
			step.Risk = rb.Risk
		}
	}
	return rb, nil
}

func matchScore(rb Runbook, req MatchRequest) (int, []string) {
	var score int
	var reasons []string
	if matchAny(rb.Scope.Capabilities, req.Capability) {
		score += 50
		reasons = append(reasons, "capability matched")
	}
	if matchAny(rb.Scope.Services, req.Service) {
		score += 40
		reasons = append(reasons, "service matched")
	}
	if matchAny(rb.Scope.Environments, req.Environment) {
		score += 10
		reasons = append(reasons, "environment matched")
	}
	if req.Symptom != "" && textMatches(rb.Name+" "+rb.Description+" "+strings.Join(scopeText(rb.Scope), " "), req.Symptom) {
		score += 20
		reasons = append(reasons, "symptom matched")
	}
	return score, reasons
}

func matchAny(values []string, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return false
	}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == query || strings.Contains(value, query) || strings.Contains(query, value) {
			return true
		}
	}
	return false
}

func textMatches(text, query string) bool {
	text = strings.ToLower(text)
	query = strings.ToLower(strings.TrimSpace(query))
	return query != "" && strings.Contains(text, query)
}

func scopeText(scope Scope) []string {
	var out []string
	out = append(out, scope.Modules...)
	out = append(out, scope.Capabilities...)
	out = append(out, scope.Services...)
	out = append(out, scope.Environments...)
	return out
}
