// Package workflowtestmanifest owns validation and selection for the shared
// Workflow Canvas/MCP test-case manifest. It is test infrastructure, not a
// second workflow runtime.
package workflowtestmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	ManifestSchemaVersion = "aiops.workflow-canvas.case-manifest/v1"
	ActivationActive      = "active"
	ActivationPlanned     = "planned"
	OutcomePass           = "pass"
	OutcomeKnownFailure   = "known_failure"
	ResultPassed          = "passed"
	ResultFailed          = "failed"
)

type Manifest struct {
	SchemaVersion          string       `json:"schemaVersion"`
	CurrentActivationPhase int          `json:"currentActivationPhase"`
	ApprovedCapabilityTags []string     `json:"approvedCapabilityTags"`
	Fixtures               []FixtureRef `json:"fixtures"`
	Cases                  []Case       `json:"cases"`
}

type FixtureRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Case struct {
	ID                       string   `json:"id"`
	Priority                 string   `json:"priority"`
	ActivationPhase          int      `json:"activationPhase"`
	ActivationState          string   `json:"activationState"`
	Suite                    string   `json:"suite"`
	Selector                 string   `json:"selector"`
	Fixtures                 []string `json:"fixtures"`
	Browsers                 []string `json:"browsers"`
	CapabilityTags           []string `json:"capabilityTags"`
	Owner                    string   `json:"owner"`
	Evidence                 []string `json:"evidence"`
	ExpectedOutcome          string   `json:"expectedOutcome"`
	ExpectedFailureSignature string   `json:"expectedFailureSignature,omitempty"`
	GreenActivationPhase     *int     `json:"greenActivationPhase,omitempty"`
}

type SelectOptions struct {
	Phase             *int
	Priorities        []string
	KnownFailuresOnly bool
}

type CaseResult struct {
	ID               string `json:"id"`
	Outcome          string `json:"outcome"`
	FailureSignature string `json:"failureSignature,omitempty"`
}

func Load(path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer file.Close()
	var manifest Manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func LoadResults(path string) ([]CaseResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var results []CaseResult
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&results); err != nil {
		return nil, fmt.Errorf("decode results: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return results, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected JSON after document")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func Validate(manifest Manifest) error {
	var problems []string
	if manifest.SchemaVersion != ManifestSchemaVersion {
		problems = append(problems, fmt.Sprintf("schemaVersion must be %q", ManifestSchemaVersion))
	}
	if manifest.CurrentActivationPhase < 0 || manifest.CurrentActivationPhase > 5 {
		problems = append(problems, "currentActivationPhase must be between 0 and 5")
	}
	approved := stringSet(manifest.ApprovedCapabilityTags)
	fixtures := map[string]struct{}{}
	for index, fixture := range manifest.Fixtures {
		if fixture.ID == "" || fixture.Kind == "" || fixture.Path == "" {
			problems = append(problems, fmt.Sprintf("fixture[%d] requires id, kind, and path", index))
		}
		if _, exists := fixtures[fixture.ID]; exists {
			problems = append(problems, "duplicate fixture "+fixture.ID)
		}
		fixtures[fixture.ID] = struct{}{}
		if strings.HasPrefix(fixture.Path, "/") || strings.Contains(fixture.Path, "..") {
			problems = append(problems, fmt.Sprintf("fixture %s path must remain under manifest root", fixture.ID))
		}
	}
	seen := map[string]Case{}
	for index, testCase := range manifest.Cases {
		prefix := fmt.Sprintf("case[%d] %s", index, testCase.ID)
		if testCase.ID == "" {
			problems = append(problems, fmt.Sprintf("case[%d] requires id", index))
		}
		if _, exists := seen[testCase.ID]; exists {
			problems = append(problems, "duplicate case "+testCase.ID)
		}
		seen[testCase.ID] = testCase
		if !validPriority(testCase.Priority) {
			problems = append(problems, prefix+" requires priority P0, P1, or P2")
		}
		if testCase.ActivationPhase < 0 || testCase.ActivationPhase > 5 {
			problems = append(problems, prefix+" activationPhase must be between 0 and 5")
		}
		if testCase.ActivationState != ActivationActive && testCase.ActivationState != ActivationPlanned {
			problems = append(problems, prefix+" activationState must be active or planned")
		}
		if testCase.ActivationPhase > manifest.CurrentActivationPhase && testCase.ActivationState != ActivationPlanned {
			problems = append(problems, prefix+" is future work and must remain planned")
		}
		if testCase.Suite == "" || testCase.Selector == "" || testCase.Owner == "" || len(testCase.Fixtures) == 0 || len(testCase.Evidence) == 0 || len(testCase.CapabilityTags) == 0 {
			problems = append(problems, prefix+" requires suite, selector, owner, fixtures, evidence, and capabilityTags")
		}
		if testCase.ID != "" && !strings.Contains(testCase.Selector, testCase.ID) {
			problems = append(problems, prefix+" selector must contain the Case ID")
		}
		for _, fixture := range testCase.Fixtures {
			if _, exists := fixtures[fixture]; !exists {
				problems = append(problems, fmt.Sprintf("%s references unknown fixture %s", prefix, fixture))
			}
		}
		for _, browser := range testCase.Browsers {
			if !contains([]string{"chromium", "firefox", "webkit", "browser-in-app"}, browser) {
				problems = append(problems, fmt.Sprintf("%s has unknown browser %s", prefix, browser))
			}
		}
		switch testCase.ExpectedOutcome {
		case OutcomePass:
			if testCase.ExpectedFailureSignature != "" || testCase.GreenActivationPhase != nil {
				problems = append(problems, prefix+" pass outcome cannot declare known-failure fields")
			}
		case OutcomeKnownFailure:
			if testCase.ActivationState != ActivationActive || testCase.ExpectedFailureSignature == "" || testCase.GreenActivationPhase == nil {
				problems = append(problems, prefix+" known_failure requires active state, signature, and greenActivationPhase")
			} else if *testCase.GreenActivationPhase <= manifest.CurrentActivationPhase {
				problems = append(problems, prefix+" reached greenActivationPhase and must convert to pass")
			}
		default:
			problems = append(problems, prefix+" expectedOutcome must be pass or known_failure")
		}
		if testCase.ActivationPhase == 5 && testCase.ActivationState == ActivationActive && !intersects(approved, testCase.CapabilityTags) {
			problems = append(problems, prefix+" phase-5 activation requires an approved capabilityTag")
		}
	}
	for _, id := range ExpectedDesignCaseIDs() {
		if _, exists := seen[id]; !exists {
			problems = append(problems, "design case missing from manifest: "+id)
		}
	}
	for id := range seen {
		if !contains(ExpectedDesignCaseIDs(), id) {
			problems = append(problems, "unknown design case in manifest: "+id)
		}
	}
	return joinedErrors(problems)
}

func ResolvePhase(repositoryPhase int, override string) (int, error) {
	if strings.TrimSpace(override) == "" {
		return repositoryPhase, nil
	}
	phase, err := strconv.Atoi(override)
	if err != nil || phase < 0 || phase > 5 {
		return 0, fmt.Errorf("phase override %q must be an integer between 0 and 5", override)
	}
	if phase < repositoryPhase {
		return 0, fmt.Errorf("phase override %d cannot lower repository phase %d", phase, repositoryPhase)
	}
	return phase, nil
}

func Select(manifest Manifest, options SelectOptions) ([]Case, error) {
	if err := Validate(manifest); err != nil {
		return nil, err
	}
	phase := manifest.CurrentActivationPhase
	if options.Phase != nil {
		if *options.Phase < phase {
			return nil, fmt.Errorf("phase override %d cannot lower repository phase %d", *options.Phase, phase)
		}
		phase = *options.Phase
	}
	priorities := stringSet(options.Priorities)
	if len(priorities) == 0 {
		priorities = stringSet([]string{"P0", "P1"})
	}
	selected := make([]Case, 0)
	for _, testCase := range manifest.Cases {
		if testCase.ActivationState == ActivationActive && testCase.ActivationPhase <= phase {
			if options.KnownFailuresOnly && testCase.ExpectedOutcome != OutcomeKnownFailure {
				continue
			}
			if _, include := priorities[testCase.Priority]; include {
				selected = append(selected, testCase)
			}
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	return selected, nil
}

func Gate(manifest Manifest, results []CaseResult, options SelectOptions) error {
	selected, err := Select(manifest, options)
	if err != nil {
		return err
	}
	byID := map[string]CaseResult{}
	for _, result := range results {
		if _, exists := byID[result.ID]; exists {
			return fmt.Errorf("duplicate result %s", result.ID)
		}
		byID[result.ID] = result
	}
	var problems []string
	for _, testCase := range selected {
		result, exists := byID[testCase.ID]
		if !exists {
			problems = append(problems, testCase.ID+" has no result")
			continue
		}
		if testCase.ExpectedOutcome == OutcomeKnownFailure {
			if result.Outcome != ResultFailed || !strings.Contains(result.FailureSignature, testCase.ExpectedFailureSignature) {
				problems = append(problems, testCase.ID+" did not produce its expected failure signature")
			}
		} else if result.Outcome != ResultPassed {
			problems = append(problems, testCase.ID+" did not pass")
		}
	}
	return joinedErrors(problems)
}

func CompareProtectedBaseline(previous, current Manifest) error {
	var problems []string
	if current.CurrentActivationPhase < previous.CurrentActivationPhase {
		problems = append(problems, "currentActivationPhase cannot decrease")
	}
	currentByID := map[string]Case{}
	for _, testCase := range current.Cases {
		currentByID[testCase.ID] = testCase
	}
	for _, oldCase := range previous.Cases {
		newCase, exists := currentByID[oldCase.ID]
		if !exists {
			problems = append(problems, "case deleted: "+oldCase.ID)
			continue
		}
		if priorityRank(newCase.Priority) > priorityRank(oldCase.Priority) {
			problems = append(problems, "priority downgraded: "+oldCase.ID)
		}
		if oldCase.ActivationState == ActivationActive && newCase.ActivationState == ActivationPlanned {
			problems = append(problems, "active -> planned: "+oldCase.ID)
		}
		if newCase.ActivationPhase > oldCase.ActivationPhase {
			problems = append(problems, "activationPhase moved later: "+oldCase.ID)
		}
		if oldCase.ExpectedOutcome == OutcomePass && newCase.ExpectedOutcome == OutcomeKnownFailure {
			problems = append(problems, "pass -> known_failure: "+oldCase.ID)
		}
	}
	return joinedErrors(problems)
}

func ExpectedDesignCaseIDs() []string {
	groups := []struct {
		prefix string
		count  int
	}{{"DOM", 12}, {"MCP", 15}, {"TXN", 17}, {"UI", 27}, {"EVT", 10}, {"SEC", 15}, {"PW", 33}, {"RWD", 4}, {"VIS", 3}, {"A11Y", 6}, {"PERF", 5}, {"FI", 8}, {"GEN", 14}}
	ids := make([]string, 0, 169)
	for _, group := range groups {
		for index := 1; index <= group.count; index++ {
			ids = append(ids, fmt.Sprintf("%s-%03d", group.prefix, index))
		}
	}
	return ids
}

func validPriority(priority string) bool { return priorityRank(priority) < 3 }
func priorityRank(priority string) int {
	switch priority {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	default:
		return 3
	}
}
func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
func intersects(set map[string]struct{}, values []string) bool {
	for _, value := range values {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func joinedErrors(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return errors.New(strings.Join(problems, "; "))
}
