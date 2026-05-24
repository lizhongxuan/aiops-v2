package runtimekernel

import (
	"testing"

	"aiops-v2/internal/modelrouter"
)

func TestContextBudgetPolicyThresholdsForLargeContext(t *testing.T) {
	policy := DefaultContextBudgetPolicy(200000, 32000)
	thresholds := policy.Thresholds()
	if thresholds.ReservedOutputTokens != 20000 {
		t.Fatalf("reserved = %d, want 20000", thresholds.ReservedOutputTokens)
	}
	if thresholds.EffectiveContextWindow != 180000 {
		t.Fatalf("effective = %d, want 180000", thresholds.EffectiveContextWindow)
	}
	if thresholds.WarningThreshold != 147000 {
		t.Fatalf("warning = %d, want 147000", thresholds.WarningThreshold)
	}
	if thresholds.AutoCompactThreshold != 167000 {
		t.Fatalf("auto = %d, want 167000", thresholds.AutoCompactThreshold)
	}
	if thresholds.BlockingLimit != 177000 {
		t.Fatalf("blocking = %d, want 177000", thresholds.BlockingLimit)
	}
	if thresholds.SmallContextMode {
		t.Fatal("large context should not use small-context mode")
	}
}

func TestContextBudgetPolicyThresholdsForDefaultContext(t *testing.T) {
	policy := DefaultContextBudgetPolicy(128000, 16000)
	thresholds := policy.Thresholds()
	if thresholds.ReservedOutputTokens != 16000 {
		t.Fatalf("reserved = %d, want 16000", thresholds.ReservedOutputTokens)
	}
	if thresholds.EffectiveContextWindow != 112000 {
		t.Fatalf("effective = %d, want 112000", thresholds.EffectiveContextWindow)
	}
	if thresholds.WarningThreshold != 79000 {
		t.Fatalf("warning = %d, want 79000", thresholds.WarningThreshold)
	}
	if thresholds.AutoCompactThreshold != 99000 {
		t.Fatalf("auto = %d, want 99000", thresholds.AutoCompactThreshold)
	}
	if thresholds.BlockingLimit != 109000 {
		t.Fatalf("blocking = %d, want 109000", thresholds.BlockingLimit)
	}
}

func TestContextBudgetPolicyThresholdsForSmallContext32K(t *testing.T) {
	policy := DefaultContextBudgetPolicy(32000, 8000)
	thresholds := policy.Thresholds()
	if !thresholds.SmallContextMode {
		t.Fatal("expected small context mode")
	}
	if thresholds.ReservedOutputTokens != 4800 {
		t.Fatalf("reserved = %d, want 4800", thresholds.ReservedOutputTokens)
	}
	if thresholds.EffectiveContextWindow != 27200 {
		t.Fatalf("effective = %d, want 27200", thresholds.EffectiveContextWindow)
	}
	if thresholds.WarningThreshold != 21216 {
		t.Fatalf("warning = %d, want 21216", thresholds.WarningThreshold)
	}
	if thresholds.AutoCompactThreshold != 23936 {
		t.Fatalf("auto = %d, want 23936", thresholds.AutoCompactThreshold)
	}
	if thresholds.BlockingLimit != 25568 {
		t.Fatalf("blocking = %d, want 25568", thresholds.BlockingLimit)
	}
}

func TestContextBudgetPolicyThresholdsForSmallContext20K(t *testing.T) {
	policy := DefaultContextBudgetPolicy(20000, 8000)
	thresholds := policy.Thresholds()
	if !thresholds.SmallContextMode {
		t.Fatal("expected small context mode")
	}
	if thresholds.ReservedOutputTokens != 3000 {
		t.Fatalf("reserved = %d, want 3000", thresholds.ReservedOutputTokens)
	}
	if thresholds.WarningThreshold != 13260 {
		t.Fatalf("warning = %d, want 13260", thresholds.WarningThreshold)
	}
	if thresholds.AutoCompactThreshold != 14960 {
		t.Fatalf("auto = %d, want 14960", thresholds.AutoCompactThreshold)
	}
	if thresholds.BlockingLimit != 15980 {
		t.Fatalf("blocking = %d, want 15980", thresholds.BlockingLimit)
	}
}

func TestContextBudgetPolicyInvalidConfigFallsBack(t *testing.T) {
	policy := DefaultContextBudgetPolicy(-1, -1)
	thresholds := policy.Thresholds()
	if thresholds.MaxContextTokens != DefaultMaxTokens {
		t.Fatalf("max context = %d, want %d", thresholds.MaxContextTokens, DefaultMaxTokens)
	}
	if thresholds.ReservedOutputTokens != 20000 {
		t.Fatalf("reserved = %d, want 20000", thresholds.ReservedOutputTokens)
	}
	if thresholds.AutoCompactThreshold <= thresholds.WarningThreshold {
		t.Fatalf("auto threshold should be greater than warning: %#v", thresholds)
	}
}

func TestContextBudgetPolicyAdoptsManualContextWindow(t *testing.T) {
	session := &SessionState{
		Context: ContextWindow{MaxTokens: 128000},
		Messages: []Message{{
			ID:      "msg-1",
			Role:    "user",
			Content: "hello",
		}},
	}
	kernel := &EinoKernel{
		modelRouter: modelrouterWithContextWindow(12000),
	}

	policy := kernel.contextBudgetPolicyForSession(session, agentKindForSession(session))
	thresholds := policy.Thresholds()

	if session.Context.MaxTokens != 12000 {
		t.Fatalf("session max tokens = %d, want 12000", session.Context.MaxTokens)
	}
	if thresholds.MaxContextTokens != 12000 || !thresholds.SmallContextMode {
		t.Fatalf("thresholds = %#v, want manual 12K small-context budget", thresholds)
	}
}

func modelrouterWithContextWindow(maxContextTokens int) *modelrouter.Router {
	router := modelrouter.NewRouter("mock", nil, nil)
	router.SetProviderConfigResolver(testProviderConfigResolver{config: modelrouter.ProviderConfig{
		Provider:         "mock",
		Model:            "mock",
		MaxContextTokens: maxContextTokens,
	}})
	return router
}
