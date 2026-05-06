package incidents

import (
	"testing"
	"time"
)

func TestIncidentServiceCreateAddEvidenceRankAndClose(t *testing.T) {
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	service := NewService(NewInMemoryStore(), func() time.Time { return now })

	incident, err := service.Create(CreateRequest{
		Title:                "order-api latency spike",
		Severity:             "sev2",
		Source:               "coroot",
		Environment:          "prod",
		BusinessCapability:   "订单提交",
		AffectedServices:     []string{"order-api"},
		BusinessCapabilityID: "capability.order.submit",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if incident.Status != IncidentStatusOpen || incident.ID == "" {
		t.Fatalf("incident = %#v, want open incident with id", incident)
	}

	evidence, err := service.AddEvidence(incident.ID, EvidenceRef{
		Source:     "coroot",
		RawRef:     "coroot:webhook:abc",
		Summary:    "SLO burn rate increased",
		Confidence: "high",
		EntityID:   "service.order-api",
	})
	if err != nil {
		t.Fatalf("AddEvidence() error = %v", err)
	}
	if evidence.ID == "" || evidence.CreatedAt.IsZero() {
		t.Fatalf("evidence = %#v, want id and timestamp", evidence)
	}

	hypotheses, err := service.RankHypotheses(incident.ID, []Hypothesis{
		{Hypothesis: "DB connection pressure", Confidence: 0.82, SupportingEvidence: []string{evidence.ID}},
		{Hypothesis: "frontend regression", Confidence: 0.25},
	})
	if err != nil {
		t.Fatalf("RankHypotheses() error = %v", err)
	}
	if hypotheses[0].Rank != 1 || hypotheses[0].Hypothesis != "DB connection pressure" {
		t.Fatalf("hypotheses = %#v, want highest confidence first", hypotheses)
	}

	closed, err := service.Close(incident.ID, CloseRequest{
		RootCause:  "report worker exhausted DB connections",
		Mitigation: "restarted report worker",
		FollowUps:  []string{"add report worker connection limit"},
	})
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if closed.Status != IncidentStatusClosed || closed.Postmortem == nil {
		t.Fatalf("closed incident = %#v, want closed with postmortem", closed)
	}
	if closed.Postmortem.Impact == "" || closed.Postmortem.RootCause != "report worker exhausted DB connections" {
		t.Fatalf("postmortem = %#v", closed.Postmortem)
	}
}
