package resourcebinding

import "testing"

func TestSessionTargetFromVerifiedBindingsBuildsMultiHostTargetSet(t *testing.T) {
	bindings := []ResourceBindingSnapshot{
		NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-b"}, BindingOptions{Source: BindingSourceMention, VerifiedBy: HostVerifierHostopsResolver, TrustLevel: TrustLevelVerified}),
		NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-a"}, BindingOptions{Source: BindingSourceMention, VerifiedBy: HostVerifierHostopsResolver, TrustLevel: TrustLevelVerified}),
		NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-x"}, BindingOptions{Source: BindingSourceMention, TrustLevel: TrustLevelRejected}),
	}

	snapshot := SessionTargetFromVerifiedBindings(bindings, "turn-1", []string{"m2", "m1"})
	if snapshot.BindingMode != BindingModeMultiHost {
		t.Fatalf("BindingMode = %q, want multi_host", snapshot.BindingMode)
	}
	if len(snapshot.HostIDs) != 2 || snapshot.HostIDs[0] != "host-a" || snapshot.HostIDs[1] != "host-b" {
		t.Fatalf("HostIDs = %#v, want sorted verified hosts", snapshot.HostIDs)
	}
	if snapshot.ActiveTargetSetID == "" || snapshot.TraceHash == "" {
		t.Fatalf("snapshot hashes missing: %#v", snapshot)
	}
}

func TestSessionTargetFromNewExplicitBindingsReplacesPreviousTargetSet(t *testing.T) {
	previous := NewSessionTargetSnapshot(SessionTargetInput{
		HostIDs:          []string{"host-a", "host-b"},
		SourceTurnID:     "turn-1",
		SourceMentionIDs: []string{"m-host-a", "m-host-b"},
	})
	next := SessionTargetFromVerifiedBindings([]ResourceBindingSnapshot{
		NewBindingSnapshot(ResourceRef{Type: ResourceTypeHost, ID: "host-c"}, BindingOptions{
			Source:     BindingSourceMention,
			VerifiedBy: HostVerifierHostopsResolver,
			TrustLevel: TrustLevelVerified,
		}),
	}, "turn-2", []string{"m-host-c"})

	if next.ActiveTargetSetID == previous.ActiveTargetSetID {
		t.Fatalf("new explicit target reused old target set id %q", next.ActiveTargetSetID)
	}
	if next.BindingMode != BindingModeSingleHost {
		t.Fatalf("BindingMode = %q, want single_host", next.BindingMode)
	}
	if len(next.HostIDs) != 1 || next.HostIDs[0] != "host-c" {
		t.Fatalf("HostIDs = %#v, want only new explicit host-c", next.HostIDs)
	}
	if len(next.SourceMentionIDs) != 1 || next.SourceMentionIDs[0] != "m-host-c" {
		t.Fatalf("SourceMentionIDs = %#v, want only new mention", next.SourceMentionIDs)
	}
}

func TestSessionTargetNextTurnExpiresWithoutChangingTargetID(t *testing.T) {
	snapshot := NewSessionTargetSnapshot(SessionTargetInput{HostIDs: []string{"host-a"}, SourceTurnID: "turn-1", ExpiresAfterTurns: 2})
	next := snapshot.NextTurn()

	if next.ActiveTargetSetID != snapshot.ActiveTargetSetID {
		t.Fatalf("target set id changed: %q != %q", next.ActiveTargetSetID, snapshot.ActiveTargetSetID)
	}
	if next.ExpiresAfterTurns != 1 {
		t.Fatalf("ExpiresAfterTurns = %d, want 1", next.ExpiresAfterTurns)
	}
}

func TestSessionTargetClearedRequiresConfirmation(t *testing.T) {
	snapshot := SessionTargetCleared("turn-clear")
	if snapshot.BindingMode != BindingModeNone || !snapshot.RequiresConfirmation {
		t.Fatalf("cleared snapshot = %#v, want none and requires confirmation", snapshot)
	}
}
