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
}

func admissionMetadataKeyUse(key string) (admissionMetadataUse, bool) {
	use, ok := admissionMetadataRegistry[strings.TrimSpace(key)]
	return use, ok
}
