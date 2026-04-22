package settings

import "testing"

func TestPrecedenceIncludesClaudeLikeSettingSources(t *testing.T) {
	got := Precedence()
	want := []Source{
		SourceUserSettings,
		SourceProjectSettings,
		SourceLocalSettings,
		SourceFlagSettings,
		SourcePolicySettings,
	}

	if len(got) != len(want) {
		t.Fatalf("Precedence() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Precedence()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestPolicyPrecedenceIncludesClaudeLikeManagedLayers(t *testing.T) {
	got := PolicyPrecedence()
	want := []PolicySource{
		PolicySourceRemote,
		PolicySourceMachine,
		PolicySourceManaged,
		PolicySourceUser,
	}

	if len(got) != len(want) {
		t.Fatalf("PolicyPrecedence() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PolicyPrecedence()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
