package resourcebinding

import (
	"testing"
)

func TestHostBindingFromVerifiedMention(t *testing.T) {
	binding := HostBindingFromMention(HostMentionProjection{
		Raw:         "@db-a",
		HostID:      "host-a",
		DisplayName: "db-a",
		Source:      "inventory",
		Resolved:    true,
		Confidence:  1,
	})

	if binding.Ref.Type != ResourceTypeHost || binding.Ref.ID != "host-a" {
		t.Fatalf("binding ref = %+v, want verified host-a", binding.Ref)
	}
	if binding.Source != BindingSourceMention {
		t.Fatalf("binding source = %q, want mention", binding.Source)
	}
	if binding.VerifiedBy != HostVerifierHostopsResolver || !binding.Verified() || binding.FailClosed {
		t.Fatalf("binding verification = %+v, want verified by hostops resolver and open", binding)
	}
}

func TestHostBindingFromUnresolvedMentionFailsClosed(t *testing.T) {
	binding := HostBindingFromMention(HostMentionProjection{
		Raw:         "@host-a",
		DisplayName: "host-a",
		Source:      "hostname_literal",
		Resolved:    false,
		Confidence:  0.75,
	})

	if binding.Ref.ID == "" {
		t.Fatalf("binding ref id is empty, want weak trace identity")
	}
	if binding.Verified() {
		t.Fatalf("unresolved host mention produced verified binding: %+v", binding)
	}
	if !binding.FailClosed {
		t.Fatalf("unresolved host mention FailClosed = false")
	}
}

func TestFakeRouteHostIDWithoutVerificationFailsClosed(t *testing.T) {
	binding := BuildHostBinding(HostBindingInput{
		HostID:      "host-a",
		DisplayName: "host-a",
		Source:      BindingSourceRouteMetadata,
	})

	if binding.TrustLevel != TrustLevelRejected {
		t.Fatalf("trust level = %q, want rejected", binding.TrustLevel)
	}
	if binding.Verified() || !binding.FailClosed {
		t.Fatalf("unverified route metadata binding = %+v, want fail-closed rejected", binding)
	}
}

func TestRawHostTextDoesNotProduceExecCapability(t *testing.T) {
	binding := HostBindingFromMention(HostMentionProjection{
		Raw:         "@host-a",
		DisplayName: "host-a",
		Source:      "hostname_literal",
		Resolved:    false,
	})
	capabilities := BuildCapabilities(binding, []ToolCapabilityInput{{
		ToolName:   "host.exec",
		Capability: CapabilityExec,
	}})

	if len(capabilities) != 0 {
		t.Fatalf("unverified host binding capabilities = %+v, want none", capabilities)
	}
}
