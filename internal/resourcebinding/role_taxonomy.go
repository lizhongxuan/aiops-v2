package resourcebinding

import "strings"

const (
	RolePGPrimary = "pg_primary"
	RolePGStandby = "pg_standby"
	RoleSource    = "source"
	RoleTarget    = "target"
	RoleLeader    = "leader"
	RoleFollower  = "follower"
)

var roleAliases = map[string]string{
	"pg主节点":    RolePGPrimary,
	"pg主库":     RolePGPrimary,
	"主节点":      RolePGPrimary,
	"主库":       RolePGPrimary,
	"primary":  RolePGPrimary,
	"master":   RolePGPrimary,
	"pg从节点":    RolePGStandby,
	"pg从库":     RolePGStandby,
	"从节点":      RolePGStandby,
	"备库":       RolePGStandby,
	"standby":  RolePGStandby,
	"replica":  RolePGStandby,
	"source":   RoleSource,
	"源":        RoleSource,
	"源端":       RoleSource,
	"target":   RoleTarget,
	"目标":       RoleTarget,
	"目标端":      RoleTarget,
	"leader":   RoleLeader,
	"follower": RoleFollower,
}

func NormalizeRole(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	if canonical, ok := roleAliases[value]; ok {
		return canonical
	}
	switch value {
	case RolePGPrimary, RolePGStandby, RoleSource, RoleTarget, RoleLeader, RoleFollower:
		return value
	default:
		return ""
	}
}

func RoleAliases(role string) []string {
	role = NormalizeRole(role)
	var out []string
	for alias, canonical := range roleAliases {
		if canonical == role {
			out = append(out, alias)
		}
	}
	out = append(out, role)
	return uniqueSorted(out)
}

func RolesConflict(a, b string) bool {
	a = NormalizeRole(a)
	b = NormalizeRole(b)
	if a == "" || b == "" || a == b {
		return false
	}
	return (a == RolePGPrimary && b == RolePGStandby) ||
		(a == RolePGStandby && b == RolePGPrimary) ||
		(a == RoleSource && b == RoleTarget) ||
		(a == RoleTarget && b == RoleSource) ||
		(a == RoleLeader && b == RoleFollower) ||
		(a == RoleFollower && b == RoleLeader)
}

func RoleUniqueInTargetSet(role string) bool {
	switch NormalizeRole(role) {
	case RolePGPrimary, RoleSource, RoleTarget, RoleLeader:
		return true
	default:
		return false
	}
}
