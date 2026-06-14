package toolfailure

type ToolFailureKind string

const (
	KindNone                 ToolFailureKind = ""
	KindToolNotFound         ToolFailureKind = "tool_not_found"
	KindInvalidArguments     ToolFailureKind = "invalid_arguments"
	KindPermissionDenied     ToolFailureKind = "permission_denied"
	KindApprovalRequired     ToolFailureKind = "approval_required"
	KindEvidenceRequired     ToolFailureKind = "evidence_required"
	KindPolicyDenied         ToolFailureKind = "policy_denied"
	KindMCPServerUnavailable ToolFailureKind = "mcp_server_unavailable"
	KindMCPSessionExpired    ToolFailureKind = "mcp_session_expired"
	KindTimeout              ToolFailureKind = "timeout"
	KindRateLimited          ToolFailureKind = "rate_limited"
	KindTransientNetwork     ToolFailureKind = "transient_network"
	KindToolBusinessError    ToolFailureKind = "tool_business_error"
	KindOutputSchemaMismatch ToolFailureKind = "output_schema_mismatch"
	KindSideEffectUnknown    ToolFailureKind = "side_effect_unknown"
)

type HandlingAction string

const (
	ActionContinue         HandlingAction = "continue"
	ActionFeedErrorToModel HandlingAction = "feed_error_to_model"
	ActionAskUser          HandlingAction = "ask_user"
	ActionBlockApproval    HandlingAction = "block_for_approval"
	ActionBlockEvidence    HandlingAction = "block_for_evidence"
	ActionFailTurn         HandlingAction = "fail_turn"
)
