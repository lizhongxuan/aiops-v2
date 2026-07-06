package resourcebinding

import "testing"

func TestNormalizeRoleSupportsPGAliases(t *testing.T) {
	for _, alias := range []string{"主节点", "主库", "primary"} {
		if got := NormalizeRole(alias); got != RolePGPrimary {
			t.Fatalf("NormalizeRole(%q) = %q, want pg_primary", alias, got)
		}
	}
	for _, alias := range []string{"从节点", "备库", "standby"} {
		if got := NormalizeRole(alias); got != RolePGStandby {
			t.Fatalf("NormalizeRole(%q) = %q, want pg_standby", alias, got)
		}
	}
}

func TestRolesConflictAndUniqueRole(t *testing.T) {
	if !RolesConflict(RolePGPrimary, RolePGStandby) {
		t.Fatalf("pg primary and standby should conflict on same resource")
	}
	if !RoleUniqueInTargetSet(RolePGPrimary) {
		t.Fatalf("pg primary should be unique in target set")
	}
	if RoleUniqueInTargetSet(RolePGStandby) {
		t.Fatalf("pg standby should allow multiple resources")
	}
}
