package incidents

type Hypothesis struct {
	Hypothesis         string   `json:"hypothesis"`
	Rank               int      `json:"rank,omitempty"`
	Confidence         float64  `json:"confidence,omitempty"`
	SupportingEvidence []string `json:"supportingEvidence,omitempty"`
}
