package runtimecontract

import "strings"

const (
	MetadataProfile           = "profile"
	MetadataToolProfile       = "toolProfile"
	MetadataAgentProfile      = "agentProfile"
	MetadataAgentKind         = "agentKind"
	MetadataPermissionProfile = "permissionProfile"
	MetadataUserConstraints   = "aiops.user.constraints"
	MetadataRuntimeRoute      = "runtimeRoute"
	MetadataApprovalPolicy    = "approvalPolicy"
	MetadataPermissionHash    = "permissionHash"
	MetadataTargetHostID      = "aiops.target.hostId"
	MetadataTargetRefs        = "aiops.target.refs"
	MetadataTargetBinding     = "aiops.target.binding"
	MetadataRouteMode         = "aiops.route.mode"
)

type admissionMetadataUse uint8

const (
	admissionMetadataControl admissionMetadataUse = iota + 1
	admissionMetadataCompatibility
)

var admissionMetadataRegistry = map[string]admissionMetadataUse{
	MetadataIntentFrame:       admissionMetadataControl,
	MetadataIntentKind:        admissionMetadataControl,
	MetadataIntentDataScopes:  admissionMetadataControl,
	MetadataIntentRiskBudget:  admissionMetadataControl,
	MetadataIntentConfidence:  admissionMetadataControl,
	MetadataEvidenceKinds:     admissionMetadataControl,
	MetadataWeakSignals:       admissionMetadataControl,
	MetadataProfile:           admissionMetadataControl,
	MetadataToolProfile:       admissionMetadataControl,
	MetadataAgentProfile:      admissionMetadataControl,
	MetadataAgentKind:         admissionMetadataControl,
	MetadataPermissionProfile: admissionMetadataControl,
	MetadataUserConstraints:   admissionMetadataControl,
	MetadataRuntimeRoute:      admissionMetadataCompatibility,
	MetadataApprovalPolicy:    admissionMetadataCompatibility,
	MetadataPermissionHash:    admissionMetadataCompatibility,
	MetadataLegacyRoute:       admissionMetadataCompatibility,
	MetadataIntentRoute:       admissionMetadataCompatibility,
	MetadataRouteDiff:         admissionMetadataCompatibility,
	MetadataTargetHostID:      admissionMetadataCompatibility,
	MetadataTargetRefs:        admissionMetadataCompatibility,
	MetadataTargetBinding:     admissionMetadataCompatibility,
	MetadataRouteMode:         admissionMetadataCompatibility,
}

func admissionMetadataKeyUse(key string) (admissionMetadataUse, bool) {
	use, ok := admissionMetadataRegistry[strings.TrimSpace(key)]
	return use, ok
}

// IsAdmissionControlMetadataKey reports whether a metadata key participates in
// immutable admission control rather than compatibility-only projection.
func IsAdmissionControlMetadataKey(key string) bool {
	use, ok := admissionMetadataKeyUse(key)
	return ok && use == admissionMetadataControl
}
