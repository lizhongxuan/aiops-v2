package opsmanual

import "testing"

func TestOperationContextLedgerTracksFactsAndRedactsSensitiveValues(t *testing.T) {
	ledger := NewOperationContextLedger()
	ledger.AddFact(OperationContextFact{Key: "target_host", Value: "server-local", Source: "selected_host", Confidence: 0.9})
	ledger.AddFact(OperationContextFact{Key: "backup_path", Value: "/data/backups", Source: "conversation", Confidence: 0.72})
	ledger.AddFact(OperationContextFact{Key: "password", Value: "secret", Source: "conversation", Confidence: 0.3, Sensitive: true})
	ledger.AddFact(OperationContextFact{Key: "target_host", Value: "weak-host", Source: "conversation", Confidence: 0.2})
	ledger.AddFact(OperationContextFact{Key: "target_host", Value: "user-host", Source: "user", Confidence: 1, ConfirmedByUser: true})
	ledger.AddFact(OperationContextFact{Key: "target_host", Value: "later-weak", Source: "discovery", Confidence: 0.8})

	if fact, ok := ledger.Find("target_host"); !ok || fact.Value != "user-host" || !fact.ConfirmedByUser {
		t.Fatalf("target_host fact = %#v, %v; want confirmed user value", fact, ok)
	}
	if fact, ok := ledger.FindByType("backup_path"); !ok || fact.Value != "/data/backups" {
		t.Fatalf("backup_path fact = %#v, %v; want conversation path", fact, ok)
	}
	redacted := ledger.RedactedFacts()
	if fact, ok := redacted.Find("password"); !ok || fact.Value != "[REDACTED]" {
		t.Fatalf("redacted password = %#v, %v; want [REDACTED]", fact, ok)
	}
}

func TestOperationContextLedgerMergeKeepsHigherConfidence(t *testing.T) {
	left := NewOperationContextLedger()
	left.AddFact(OperationContextFact{Key: "target_host", Value: "server-local", Source: "selected_host", Confidence: 0.9})
	right := NewOperationContextLedger()
	right.AddFact(OperationContextFact{Key: "target_host", Value: "other", Source: "conversation", Confidence: 0.4})
	right.AddFact(OperationContextFact{Key: "redis_instance", Value: "docker:aiops-redis", Source: "docker", Confidence: 0.95})

	left.Merge(right)
	if fact, _ := left.Find("target_host"); fact.Value != "server-local" {
		t.Fatalf("target_host = %#v, want original high confidence", fact)
	}
	if fact, ok := left.Find("redis_instance"); !ok || fact.Value != "docker:aiops-redis" {
		t.Fatalf("redis_instance = %#v, %v; want merged value", fact, ok)
	}
}
