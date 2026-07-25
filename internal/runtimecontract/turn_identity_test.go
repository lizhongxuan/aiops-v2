package runtimecontract

import "testing"

func TestTurnIdentityFailsClosedForMissingOrUntrimmedFields(t *testing.T) {
	valid := TurnIdentity{TenantID: "tenant-a", UserID: "user-a", SessionID: "session-a"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []TurnIdentity{
		{UserID: "user-a", SessionID: "session-a"},
		{TenantID: " tenant-a", UserID: "user-a", SessionID: "session-a"},
		{TenantID: "tenant-a", UserID: "user-a", SessionID: ""},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid identity accepted: %#v", invalid)
		}
	}
}
