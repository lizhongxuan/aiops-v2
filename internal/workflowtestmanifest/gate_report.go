package workflowtestmanifest

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	ProfilePR          = "pr"
	ProfileNightly     = "nightly"
	ProfileRC          = "rc"
	ProfileRedEvidence = "red-evidence"

	ResultSkipped = "skipped"

	StatusPlanned     = "planned"
	StatusExpectedRed = "expected-red"
	StatusMissing     = "missing"
	StatusPassed      = "pass"
	StatusFailed      = "fail"
	StatusSkipped     = "skip"
)

type Profile struct {
	Name              string   `json:"name"`
	Priorities        []string `json:"priorities"`
	KnownFailuresOnly bool     `json:"knownFailuresOnly,omitempty"`
}

func ResolveProfile(name string) (Profile, error) {
	switch strings.TrimSpace(name) {
	case ProfilePR:
		return Profile{Name: ProfilePR, Priorities: []string{"P0"}}, nil
	case ProfileNightly:
		return Profile{Name: ProfileNightly, Priorities: []string{"P0", "P1"}}, nil
	case ProfileRC:
		return Profile{Name: ProfileRC, Priorities: []string{"P0", "P1"}}, nil
	case ProfileRedEvidence:
		return Profile{Name: ProfileRedEvidence, Priorities: []string{"P0", "P1", "P2"}, KnownFailuresOnly: true}, nil
	default:
		return Profile{}, fmt.Errorf("unknown workflow test profile %q", name)
	}
}

type GateCaseReport struct {
	ID               string `json:"id"`
	Priority         string `json:"priority"`
	ActivationPhase  int    `json:"activationPhase"`
	ExpectedOutcome  string `json:"expectedOutcome"`
	Status           string `json:"status"`
	Suite            string `json:"suite"`
	Selector         string `json:"selector"`
	FailureSignature string `json:"failureSignature,omitempty"`
}

type GateReport struct {
	SchemaVersion          string           `json:"schemaVersion"`
	CurrentActivationPhase int              `json:"currentActivationPhase"`
	Counts                 map[string]int   `json:"counts"`
	Cases                  []GateCaseReport `json:"cases"`
	Problems               []string         `json:"problems,omitempty"`
}

func BuildGateReport(manifest Manifest, results []CaseResult, options SelectOptions) (GateReport, error) {
	report := GateReport{
		SchemaVersion:          "aiops.workflow-canvas.gate-report/v1",
		CurrentActivationPhase: manifest.CurrentActivationPhase,
		Counts:                 map[string]int{},
	}
	selected, err := Select(manifest, options)
	if err != nil {
		return report, err
	}
	selectedIDs := make(map[string]Case, len(selected))
	manifestIDs := make(map[string]struct{}, len(manifest.Cases))
	for _, testCase := range manifest.Cases {
		manifestIDs[testCase.ID] = struct{}{}
	}
	for _, testCase := range selected {
		selectedIDs[testCase.ID] = testCase
	}
	byID := make(map[string]CaseResult, len(results))
	for _, result := range results {
		if _, known := manifestIDs[result.ID]; !known {
			report.Problems = append(report.Problems, "unknown result "+result.ID)
			continue
		}
		if _, exists := byID[result.ID]; exists {
			report.Problems = append(report.Problems, "duplicate result "+result.ID)
			continue
		}
		byID[result.ID] = result
	}

	for _, testCase := range manifest.Cases {
		item := GateCaseReport{
			ID: testCase.ID, Priority: testCase.Priority, ActivationPhase: testCase.ActivationPhase,
			ExpectedOutcome: testCase.ExpectedOutcome, Suite: testCase.Suite, Selector: testCase.Selector,
			Status: StatusPlanned,
		}
		if _, active := selectedIDs[testCase.ID]; active {
			result, exists := byID[testCase.ID]
			switch {
			case !exists:
				item.Status = StatusMissing
				report.Problems = append(report.Problems, testCase.ID+" has no result")
			case result.Outcome == ResultSkipped:
				item.Status = StatusSkipped
				report.Problems = append(report.Problems, testCase.ID+" was skipped")
			case testCase.ExpectedOutcome == OutcomeKnownFailure && result.Outcome == ResultFailed && strings.Contains(result.FailureSignature, testCase.ExpectedFailureSignature):
				item.Status = StatusExpectedRed
				item.FailureSignature = result.FailureSignature
			case testCase.ExpectedOutcome == OutcomePass && result.Outcome == ResultPassed:
				item.Status = StatusPassed
			case testCase.ExpectedOutcome == OutcomeKnownFailure:
				item.Status = StatusFailed
				item.FailureSignature = result.FailureSignature
				report.Problems = append(report.Problems, testCase.ID+" did not produce its expected failure signature")
			default:
				item.Status = StatusFailed
				item.FailureSignature = result.FailureSignature
				report.Problems = append(report.Problems, testCase.ID+" did not pass")
			}
		}
		report.Counts[item.Status]++
		report.Cases = append(report.Cases, item)
	}
	sort.Strings(report.Problems)
	if len(report.Problems) > 0 {
		return report, joinedErrors(report.Problems)
	}
	return report, nil
}

type junitTestSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name    string        `xml:"name,attr"`
	Class   string        `xml:"classname,attr"`
	Failure *junitFailure `xml:"failure,omitempty"`
	Skipped *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

func WriteJUnit(writer io.Writer, report GateReport) error {
	suite := junitSuite{Name: "workflow-canvas-case-gate", Tests: len(report.Cases)}
	for _, item := range report.Cases {
		testCase := junitTestCase{Name: item.ID, Class: item.Suite}
		switch item.Status {
		case StatusFailed, StatusMissing:
			suite.Failures++
			testCase.Failure = &junitFailure{Message: item.Status, Body: item.FailureSignature}
		case StatusPlanned, StatusExpectedRed, StatusSkipped:
			suite.Skipped++
			testCase.Skipped = &junitSkipped{Message: item.Status}
		}
		suite.Cases = append(suite.Cases, testCase)
	}
	_, err := writer.Write([]byte(xml.Header))
	if err != nil {
		return err
	}
	encoder := xml.NewEncoder(writer)
	encoder.Indent("", "  ")
	return encoder.Encode(junitTestSuites{Suites: []junitSuite{suite}})
}
