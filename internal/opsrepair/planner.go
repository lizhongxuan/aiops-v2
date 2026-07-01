package opsrepair

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"strings"

	"aiops-v2/internal/opsmanual"
)

func PlanStatefulRepair(_ context.Context, req PlanRequest) (*RepairPlan, error) {
	if strings.TrimSpace(req.Frame.Target.Type) == "" {
		return nil, errors.New("target type is required")
	}
	return &RepairPlan{
		ID:               "repair-" + stableFrameID(req.Frame),
		Capability:       "stateful_middleware_cluster_repair",
		DiagnosisSummary: "需要先收集只读证据，再选择受治理修复方案。",
		RequiresApproval: true,
		Options: []RepairOption{{
			ID:        "rebuild-from-healthy-member",
			Title:     "基于健康成员重建异常成员",
			RiskLevel: "high",
			DataLoss:  req.Frame.RiskPreference.DataLossAcceptable,
			Steps: []RepairStep{
				{ID: "collect-readonly-evidence", Phase: PhasePreflight, ReadOnly: true, ActionRef: "probe.collect_stateful_cluster_evidence"},
				{ID: "execute-selected-repair", Phase: PhaseExecute, ReadOnly: false, ActionRef: "runner.stateful_cluster_repair"},
				{ID: "verify-cluster-health", Phase: PhaseVerify, ReadOnly: true, ActionRef: "probe.verify_stateful_cluster_health"},
			},
			WhenToUse: []string{"只读证据显示至少一个成员可作为恢复参考", "用户已确认风险和审批"},
		}},
		Verification: RepairVerification{RequiredEvidence: cloneStrings(req.Frame.EvidenceRequirements), Independent: true},
	}, nil
}

func stableFrameID(frame opsmanual.OperationFrame) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		frame.Target.Type,
		frame.Target.Name,
		frame.Operation.Action,
		strings.Join(frame.TargetScope.Hosts, ","),
	}, "\x00")))
	return hex.EncodeToString(sum[:6])
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
