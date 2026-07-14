package runtimekernel

import "testing"

func TestBuildFinalContractStatusVocabulary(t *testing.T) {
	tests := []struct {
		name         string
		answer       string
		verification FinalEvidenceVerification
		wantStatus   FinalContractStatus
		wantChecked  []string
	}{
		{
			name:   "verified checked evidence no blockers",
			answer: "已检查 uptime，主机负载正常。",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionAllow,
				Confidence: FinalEvidenceConfidenceHigh,
				State: FinalEvidenceState{
					Checked: []CheckedEvidence{{ToolCallID: "call-uptime", ToolName: "exec_command", Summary: "uptime load normal"}},
				},
			},
			wantStatus:  FinalContractStatusVerified,
			wantChecked: []string{"call-uptime"},
		},
		{
			name:   "needs evidence while declared post checks remain outstanding",
			answer: "变更已经执行，服务当前可访问。",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionAllow,
				Confidence: FinalEvidenceConfidenceHigh,
				State: FinalEvidenceState{
					Checked:            []CheckedEvidence{{ToolCallID: "call-precheck", ToolName: "exec_command", Summary: "pre-change service state"}},
					PerformedActions:   []string{"restart_service#call-restart"},
					RequiredPostChecks: []string{"service_health"},
				},
			},
			wantStatus:  FinalContractStatusNeedsEvidence,
			wantChecked: []string{"call-precheck"},
		},
		{
			name:   "verified after every declared post check is completed",
			answer: "变更和后置校验均已完成。",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionAllow,
				Confidence: FinalEvidenceConfidenceHigh,
				State: FinalEvidenceState{
					Checked:            []CheckedEvidence{{ToolCallID: "call-health", ToolName: "exec_command", Summary: "service healthy"}},
					PerformedActions:   []string{"restart_service#call-restart"},
					PostChecks:         []string{"service_health"},
					RequiredPostChecks: []string{"service_health"},
				},
			},
			wantStatus:  FinalContractStatusVerified,
			wantChecked: []string{"call-health"},
		},
		{
			name:   "partial with useful evidence and generic failed tool",
			answer: "已检查部分证据，但指标工具超时，结论需要降级。",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionDowngrade,
				Confidence: FinalEvidenceConfidenceMedium,
				State: FinalEvidenceState{
					Checked:     []CheckedEvidence{{ToolCallID: "call-proc", ToolName: "exec_command", Summary: "proc data readable"}},
					FailedTools: []FailedToolImpact{{ToolCallID: "call-metrics", ToolName: "metrics", FailureClass: "timeout", Impact: "metrics missing"}},
				},
			},
			wantStatus:  FinalContractStatusPartial,
			wantChecked: []string{"call-proc"},
		},
		{
			name:   "needs evidence when verified claim has no checked evidence",
			answer: "已确认全部检查完成。",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionDowngrade,
				Confidence: FinalEvidenceConfidenceLow,
				Reasons:    []string{"checked_claim_without_checked_evidence"},
				State: FinalEvidenceState{
					NotChecked: []NotCheckedItem{{ToolName: "exec_command", Reason: "approval_required", RequiredAction: "ask_user"}},
				},
			},
			wantStatus: FinalContractStatusNeedsEvidence,
		},
		{
			name:   "approval denied terminal reason",
			answer: `{"status":"approval_denied","reason":"operator denied"}`,
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionBlock,
				Confidence: FinalEvidenceConfidenceLow,
				Reasons:    []string{"approval_denied"},
			},
			wantStatus: FinalContractStatusApprovalDenied,
		},
		{
			name:   "host agent unavailable",
			answer: "host agent unavailable, cannot execute evidence collection.",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionDowngrade,
				Confidence: FinalEvidenceConfidenceLow,
				State: FinalEvidenceState{
					FailedTools: []FailedToolImpact{{ToolCallID: "call-exec", ToolName: "exec_command", FailureClass: "needs_host_agent", Impact: "host agent 7072 refused"}},
				},
			},
			wantStatus: FinalContractStatusToolUnavailable,
		},
		{
			name:   "unsafe mutation without target",
			answer: "不能继续执行或提供变更命令。",
			verification: FinalEvidenceVerification{
				Action:     FinalEvidenceActionBlock,
				Confidence: FinalEvidenceConfidenceLow,
				Reasons:    []string{"mutation_intent_requires_explicit_target_binding", "no_explicit_target_binding"},
				State: FinalEvidenceState{
					MutationIntentWithoutTarget: true,
				},
			},
			wantStatus: FinalContractStatusBlocked,
		},
		{
			name:       "unknown default",
			answer:     "普通回答。",
			wantStatus: FinalContractStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := BuildFinalContract(tt.answer, finalRuntimeFactsForContractTest(tt.verification))
			if contract.SchemaVersion != FinalContractSchemaVersion {
				t.Fatalf("schemaVersion = %q, want %q", contract.SchemaVersion, FinalContractSchemaVersion)
			}
			if contract.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q: %#v", contract.Status, tt.wantStatus, contract)
			}
			if contract.AnswerText != tt.answer {
				t.Fatalf("answerText = %q, want original answer", contract.AnswerText)
			}
			if tt.name == "needs evidence while declared post checks remain outstanding" && contract.Confidence != FinalEvidenceConfidenceMedium {
				t.Fatalf("confidence = %q, want capped %q while post checks remain", contract.Confidence, FinalEvidenceConfidenceMedium)
			}
			for _, want := range tt.wantChecked {
				if !containsString(contract.CheckedEvidenceRefs, want) {
					t.Fatalf("checkedEvidenceRefs = %#v, want %q", contract.CheckedEvidenceRefs, want)
				}
			}
		})
	}
}

func finalRuntimeFactsForContractTest(verification FinalEvidenceVerification) FinalRuntimeFacts {
	state := verification.State
	facts := FinalRuntimeFacts{
		CompletionStatus: FinalCompletionStatusUnknown,
		EvidenceRefs:     checkedEvidenceRefs(state.Checked),
		PostcheckStatus:  finalPostcheckStatus(state.RequiredPostChecks, state.PostChecks, nil),
		RollbackStatus:   FinalRollbackStatusNotRequired,
		FailureCodes:     append([]string(nil), verification.Reasons...),
		EvidenceState:    state,
		EvidenceDecision: verification,
	}
	for _, failed := range state.FailedTools {
		facts.FailureCodes = append(facts.FailureCodes, failed.FailureClass)
	}
	for _, missing := range state.NotChecked {
		facts.FailureCodes = append(facts.FailureCodes, missing.Reason, missing.RequiredAction)
	}
	facts.FailureCodes = uniqueSortedHarnessStrings(facts.FailureCodes)
	switch verification.Action {
	case FinalEvidenceActionBlock:
		facts.CompletionStatus = FinalCompletionStatusBlocked
	case FinalEvidenceActionDowngrade:
		facts.CompletionStatus = FinalCompletionStatusPartial
	case FinalEvidenceActionAllow:
		if len(outstandingRequiredPostChecks(state)) > 0 {
			facts.CompletionStatus = FinalCompletionStatusPartial
		} else if len(state.Checked) > 0 && len(state.NotChecked) == 0 && len(state.FailedTools) == 0 {
			facts.CompletionStatus = FinalCompletionStatusSucceeded
		}
	}
	return facts
}

func TestBuildTerminalFinalContractStatuses(t *testing.T) {
	tests := []struct {
		status FinalContractStatus
		want   FinalContractStatus
	}{
		{status: FinalContractStatusCancelled, want: FinalContractStatusCancelled},
		{status: FinalContractStatusFailed, want: FinalContractStatusFailed},
		{status: FinalContractStatusApprovalDenied, want: FinalContractStatusApprovalDenied},
		{status: "unexpected", want: FinalContractStatusUnknown},
	}
	for _, tt := range tests {
		contract := BuildTerminalFinalContract("terminal final", tt.status, []string{"reason-1"})
		if contract.Status != tt.want {
			t.Fatalf("terminal status = %q, want %q: %#v", contract.Status, tt.want, contract)
		}
		if !containsString(contract.Limitations, "reason-1") {
			t.Fatalf("limitations = %#v, want reason-1", contract.Limitations)
		}
	}
}

func TestBuildFinalContractSucceededFactsRequireCompleteTypedVerification(t *testing.T) {
	tests := []struct {
		name  string
		facts FinalRuntimeFacts
		want  FinalContractStatus
	}{
		{
			name: "missing evidence cannot be verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				PostcheckStatus:  FinalPostcheckStatusNotRequired,
			},
			want: FinalContractStatusNeedsEvidence,
		},
		{
			name: "pending postcheck cannot be verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://health"},
				PostcheckStatus:  FinalPostcheckStatusPending,
			},
			want: FinalContractStatusNeedsEvidence,
		},
		{
			name: "failed postcheck is failed",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://health"},
				PostcheckStatus:  FinalPostcheckStatusFailed,
			},
			want: FinalContractStatusFailed,
		},
		{
			name: "unknown postcheck cannot be verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://health"},
				PostcheckStatus:  "unknown",
			},
			want: FinalContractStatusNeedsEvidence,
		},
		{
			name: "outstanding required postcheck cannot be verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://precheck"},
				PostcheckStatus:  FinalPostcheckStatusPassed,
				EvidenceState: FinalEvidenceState{
					RequiredPostChecks: []string{"service_health"},
				},
			},
			want: FinalContractStatusNeedsEvidence,
		},
		{
			name: "not checked evidence cannot be verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://partial"},
				PostcheckStatus:  FinalPostcheckStatusNotRequired,
				EvidenceState: FinalEvidenceState{
					NotChecked: []NotCheckedItem{{ToolName: "metrics", Reason: "unavailable"}},
				},
			},
			want: FinalContractStatusNeedsEvidence,
		},
		{
			name: "passed postcheck with evidence is verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://health"},
				PostcheckStatus:  FinalPostcheckStatusPassed,
				EvidenceState: FinalEvidenceState{
					PostChecks:         []string{"service_health"},
					RequiredPostChecks: []string{"service_health"},
				},
			},
			want: FinalContractStatusVerified,
		},
		{
			name: "not required postcheck with evidence is verified",
			facts: FinalRuntimeFacts{
				CompletionStatus: FinalCompletionStatusSucceeded,
				EvidenceRefs:     []string{"evidence://read-only"},
				PostcheckStatus:  FinalPostcheckStatusNotRequired,
			},
			want: FinalContractStatusVerified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := BuildFinalContract("same display text", tt.facts)
			if contract.Status != tt.want {
				t.Fatalf("status = %q, want %q for facts %#v", contract.Status, tt.want, tt.facts)
			}
		})
	}
}

func TestFinalContractValidateAndNormalizeVerifiedInvariant(t *testing.T) {
	tests := []struct {
		name       string
		contract   FinalContract
		wantValid  bool
		wantStatus FinalContractStatus
	}{
		{
			name: "verified without evidence is downgraded",
			contract: FinalContract{
				SchemaVersion: FinalContractSchemaVersion,
				Status:        FinalContractStatusVerified,
			},
			wantStatus: FinalContractStatusNeedsEvidence,
		},
		{
			name: "verified with unchecked requirement is downgraded",
			contract: FinalContract{
				SchemaVersion:         FinalContractSchemaVersion,
				Status:                FinalContractStatusVerified,
				CheckedEvidenceRefs:   []string{"evidence://health"},
				UncheckedRequirements: []string{"metrics:unavailable"},
			},
			wantStatus: FinalContractStatusNeedsEvidence,
		},
		{
			name: "verified with outstanding postcheck is downgraded",
			contract: FinalContract{
				SchemaVersion:       FinalContractSchemaVersion,
				Status:              FinalContractStatusVerified,
				CheckedEvidenceRefs: []string{"evidence://precheck"},
				RequiredPostChecks:  []string{"service_health"},
			},
			wantStatus: FinalContractStatusNeedsEvidence,
		},
		{
			name: "verified with complete typed facts stays verified",
			contract: FinalContract{
				SchemaVersion:       FinalContractSchemaVersion,
				Status:              FinalContractStatusVerified,
				CheckedEvidenceRefs: []string{"evidence://health"},
				PostChecks:          []string{"service_health"},
				RequiredPostChecks:  []string{"service_health"},
			},
			wantValid:  true,
			wantStatus: FinalContractStatusVerified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.contract.Validate()
			if (err == nil) != tt.wantValid {
				t.Fatalf("Validate() error = %v, wantValid = %t", err, tt.wantValid)
			}
			normalized := tt.contract.NormalizeForProjection()
			if normalized.Status != tt.wantStatus {
				t.Fatalf("normalized status = %q, want %q: %#v", normalized.Status, tt.wantStatus, normalized)
			}
			if tt.wantValid {
				if len(normalized.Limitations) != 0 {
					t.Fatalf("valid contract limitations = %#v, want none", normalized.Limitations)
				}
				return
			}
			if !containsString(normalized.Limitations, FinalContractLimitationInvalidVerifiedFacts) {
				t.Fatalf("normalized limitations = %#v, want %q", normalized.Limitations, FinalContractLimitationInvalidVerifiedFacts)
			}
			if tt.contract.Status != FinalContractStatusVerified || len(tt.contract.Limitations) != 0 {
				t.Fatalf("NormalizeForProjection mutated source contract: %#v", tt.contract)
			}
		})
	}
}
