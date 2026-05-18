package opsmanual

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type hybridRetrievalGoldenCase struct {
	Name                  string          `json:"name"`
	Text                  string          `json:"text"`
	Metadata              map[string]any  `json:"metadata,omitempty"`
	ExpectedDecision      DecisionState   `json:"expected_decision,omitempty"`
	ExpectedTopManualID   string          `json:"expected_top_manual_id,omitempty"`
	ExpectedTopAction     string          `json:"expected_top_action,omitempty"`
	ExpectedTopPreflight  PreflightStatus `json:"expected_top_preflight,omitempty"`
	ForbiddenDecisions    []DecisionState `json:"forbidden_decisions,omitempty"`
	ForbiddenManualIDs    []string        `json:"forbidden_manual_ids,omitempty"`
	RequiredMissingFields []string        `json:"required_missing_fields,omitempty"`
	ForbiddenMissingFields []string       `json:"forbidden_missing_fields,omitempty"`
}

func TestHybridRetrievalGoldenCases(t *testing.T) {
	store, err := NewFileStore(filepath.Join("testdata", "search_ops_manuals_library.json"))
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "hybrid_retrieval_golden_cases.json"))
	if err != nil {
		t.Fatalf("ReadFile(golden cases) error = %v", err)
	}
	var cases []hybridRetrievalGoldenCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("Unmarshal(golden cases) error = %v", err)
	}
	if len(cases) < 20 {
		t.Fatalf("golden cases = %d, want at least 20", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			result, err := SearchOpsManuals(store, SearchOpsManualsRequest{Text: tc.Text, Metadata: tc.Metadata})
			if err != nil {
				t.Fatalf("SearchOpsManuals() error = %v", err)
			}
			if tc.ExpectedDecision != "" && result.Decision != tc.ExpectedDecision {
				t.Fatalf("decision = %q, want %q; result=%#v", result.Decision, tc.ExpectedDecision, result)
			}
			for _, forbidden := range tc.ForbiddenDecisions {
				if result.Decision == forbidden {
					t.Fatalf("decision = %q, forbidden; result=%#v", result.Decision, result)
				}
			}
			if tc.ExpectedTopManualID != "" {
				if len(result.Manuals) == 0 {
					t.Fatalf("manuals empty, want top manual %q; result=%#v", tc.ExpectedTopManualID, result)
				}
				if result.Manuals[0].Manual.ID != tc.ExpectedTopManualID {
					t.Fatalf("top manual = %q, want %q; result=%#v", result.Manuals[0].Manual.ID, tc.ExpectedTopManualID, result)
				}
			}
			if tc.ExpectedTopAction != "" {
				if len(result.Manuals) == 0 {
					t.Fatalf("manuals empty, want top recommended action %q; result=%#v", tc.ExpectedTopAction, result)
				}
				if result.Manuals[0].RecommendedAction != tc.ExpectedTopAction {
					t.Fatalf("top recommended action = %q, want %q; result=%#v", result.Manuals[0].RecommendedAction, tc.ExpectedTopAction, result)
				}
			}
			if tc.ExpectedTopPreflight != "" {
				if len(result.Manuals) == 0 {
					t.Fatalf("manuals empty, want top preflight status %q; result=%#v", tc.ExpectedTopPreflight, result)
				}
				if result.Manuals[0].PreflightStatus != tc.ExpectedTopPreflight {
					t.Fatalf("top preflight status = %q, want %q; result=%#v", result.Manuals[0].PreflightStatus, tc.ExpectedTopPreflight, result)
				}
			}
			for _, forbiddenID := range tc.ForbiddenManualIDs {
				for _, hit := range result.Manuals {
					if hit.Manual.ID == forbiddenID && (hit.UsableMode == DecisionDirectExecute || hit.UsableMode == DecisionAdapt) {
						t.Fatalf("manual %q appeared as executable hit: %#v", forbiddenID, hit)
					}
				}
			}
			if len(tc.RequiredMissingFields) > 0 {
				if len(result.Manuals) == 0 {
					t.Fatalf("manuals empty, want missing fields %#v", tc.RequiredMissingFields)
				}
				for _, field := range tc.RequiredMissingFields {
					if !hasAny(result.Manuals[0].MissingFields, field) {
						t.Fatalf("missing fields = %#v, want %q; result=%#v", result.Manuals[0].MissingFields, field, result)
					}
				}
			}
			for _, field := range tc.ForbiddenMissingFields {
				if len(result.Manuals) > 0 && hasAny(result.Manuals[0].MissingFields, field) {
					t.Fatalf("missing fields = %#v, forbidden %q; result=%#v", result.Manuals[0].MissingFields, field, result)
				}
			}
		})
	}
}
