package promptcompiler

import "testing"

func TestPromptSectionsChangedOnlyProtocolState(t *testing.T) {
	compiler := NewCompiler()
	baseCtx := CompileContext{
		Mode:          "inspect",
		RuntimePolicy: "runtime policy",
		ProtocolState: ProtocolPromptState{Items: []ProtocolPromptItem{{
			Kind:   "plan",
			ID:     "step-1",
			Status: "pending",
			Text:   "collect evidence",
		}}},
	}
	first, err := compiler.Compile(baseCtx)
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}

	secondCtx := baseCtx
	secondCtx.ProtocolState = ProtocolPromptState{Items: []ProtocolPromptItem{{
		Kind:   "plan",
		ID:     "step-1",
		Status: "completed",
		Text:   "collect evidence",
	}}}
	second, err := compiler.Compile(secondCtx)
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}

	changed := ChangedPromptSections(first.PromptSections, second.PromptSections)
	if len(changed) != 1 {
		t.Fatalf("changed sections = %#v, want only dynamic.context", changed)
	}
	if changed[0].ID != "dynamic.context" {
		t.Fatalf("changed section id = %q, want dynamic.context", changed[0].ID)
	}
	if changed[0].Reason != PromptSectionChangeDynamicAssetsChanged {
		t.Fatalf("changed reason = %q, want %q", changed[0].Reason, PromptSectionChangeDynamicAssetsChanged)
	}
	for _, section := range second.PromptSections {
		if section.ID == "" || section.Hash == "" {
			t.Fatalf("section missing stable metadata: %#v", section)
		}
		if section.ID == "dynamic.context" && section.TokensEstimate == 0 {
			t.Fatalf("dynamic.context should have a token estimate: %#v", section)
		}
	}
}

func TestPromptSectionsIncludeRequiredRedactionSafeIDs(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		Mode:              "plan",
		Profile:           PromptProfileEvidenceRCA,
		RuntimePolicy:     "runtime policy with token=secret-value",
		SkillPromptAssets: []string{"dynamic asset with password=secret"},
		ProtocolState: ProtocolPromptState{Items: []ProtocolPromptItem{{
			Kind:   "approval",
			ID:     "approval-1",
			Status: "pending",
			Text:   "requires review",
		}}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	byID := map[string]PromptSectionTrace{}
	for _, section := range compiled.PromptSections {
		byID[section.ID] = section
		if section.Source == "" || section.Kind == "" || section.Bytes < 0 || section.TokensEstimate < 0 {
			t.Fatalf("section has incomplete metadata: %#v", section)
		}
	}
	for _, want := range []string{
		"base.contract",
		"runtime.state",
		"profile.evidence_rca",
		"tool.surface",
		"dynamic.context",
	} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("missing prompt section %q in %#v", want, byID)
		}
	}
	for _, forbidden := range []string{
		"system.role",
		"developer.core_rules",
		"tools.index",
		"runtime.policy",
		"protocol.state",
		"context.dynamic_assets",
	} {
		if _, ok := byID[forbidden]; ok {
			t.Fatalf("legacy prompt section %q should not be traced: %#v", forbidden, byID)
		}
	}
}

func TestApplyPromptSectionCacheMarksHitMissAndInvalidated(t *testing.T) {
	previous := []PromptSectionTrace{
		{ID: "base.contract", Hash: "sha256:stable", Cache: PromptSectionCacheMiss},
		{ID: "dynamic.context", Hash: "sha256:old", Cache: PromptSectionCacheMiss},
	}
	current := []PromptSectionTrace{
		{ID: "base.contract", Hash: "sha256:stable", Cache: PromptSectionCacheMiss},
		{ID: "dynamic.context", Hash: "sha256:new", Cache: PromptSectionCacheMiss},
		{ID: "profile.advisor", Hash: "sha256:added", Cache: PromptSectionCacheMiss},
	}

	cached := ApplyPromptSectionCache(previous, current)
	byID := map[string]PromptSectionTrace{}
	for _, section := range cached {
		byID[section.ID] = section
	}

	if byID["base.contract"].Cache != PromptSectionCacheHit {
		t.Fatalf("base.contract cache = %q, want %q", byID["base.contract"].Cache, PromptSectionCacheHit)
	}
	if byID["base.contract"].CacheMissReason != "" {
		t.Fatalf("base.contract hit miss reason = %q, want empty", byID["base.contract"].CacheMissReason)
	}
	if byID["dynamic.context"].Cache != PromptSectionCacheInvalidated {
		t.Fatalf("dynamic.context cache = %q, want %q", byID["dynamic.context"].Cache, PromptSectionCacheInvalidated)
	}
	if byID["dynamic.context"].CacheMissReason != PromptSectionCacheMissReasonHashChanged {
		t.Fatalf("dynamic.context miss reason = %q, want %q", byID["dynamic.context"].CacheMissReason, PromptSectionCacheMissReasonHashChanged)
	}
	if byID["profile.advisor"].Cache != PromptSectionCacheMiss {
		t.Fatalf("profile.advisor cache = %q, want %q", byID["profile.advisor"].Cache, PromptSectionCacheMiss)
	}
	if byID["profile.advisor"].CacheMissReason != PromptSectionCacheMissReasonSectionAdded {
		t.Fatalf("profile.advisor miss reason = %q, want %q", byID["profile.advisor"].CacheMissReason, PromptSectionCacheMissReasonSectionAdded)
	}
}

func TestApplyPromptSectionCacheExplainsInitialMiss(t *testing.T) {
	cached := ApplyPromptSectionCache(nil, []PromptSectionTrace{{ID: "base.contract", Hash: "sha256:stable"}})
	if len(cached) != 1 || cached[0].Cache != PromptSectionCacheMiss || cached[0].CacheMissReason != PromptSectionCacheMissReasonNoPreviousTrace {
		t.Fatalf("initial cache trace = %#v", cached)
	}
}
