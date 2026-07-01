package runtimekernel

import "strings"

const riskyAdviceCategoryUngatedMutationCommandAdvice = "ungated_mutation_command_advice"

type RiskyAdviceResult struct {
	RequiresEvidenceGate bool
	Category             string
	Reason               string
}

func EvaluateRiskyOperationalAdvice(answer string) RiskyAdviceResult {
	text := strings.ToLower(strings.TrimSpace(answer))
	if text == "" {
		return RiskyAdviceResult{}
	}
	destructive := hasAnyRiskMarker(text, []string{
		"rm -rf",
		"delete archive",
		"delete wal",
		"delete pgdata",
		"删除 archive",
		"删除 wal",
		"清理 archive",
		"清理 wal",
		"清空 pgdata",
		"清空数据目录",
		"删除数据目录",
	})
	dataOrArchive := hasAnyRiskMarker(text, []string{
		"archive",
		"wal",
		"pgdata",
		"data directory",
		"数据目录",
		"归档",
	})
	hasDestructiveDataGate := hasAnyRiskMarker(text, []string{"先确认", "先检查", "verify", "confirm", "确认"}) &&
		hasAnyRiskMarker(text, []string{"备份", "backup", "snapshot", "快照"}) &&
		hasAnyRiskMarker(text, []string{"审批", "批准", "approval", "approved"}) &&
		hasAnyRiskMarker(text, []string{"维护窗口", "停服窗口", "停止服务窗口", "变更窗口", "maintenance window", "change window"}) &&
		hasAnyRiskMarker(text, []string{"回滚", "rollback", "验收", "post-check", "验证"}) &&
		hasAnyRiskMarker(text, []string{"timeline", "control data", "pg_controldata", "元数据", "校验", "角色", "目标"})
	hasSaferIsolation := hasAnyRiskMarker(text, []string{"move", "mv ", "隔离目录", "quarantine"}) &&
		!hasAnyRiskMarker(text, []string{"rm -rf", "删除", "delete"})
	if destructive && dataOrArchive && !hasDestructiveDataGate && !hasSaferIsolation {
		return RiskyAdviceResult{
			RequiresEvidenceGate: true,
			Category:             "destructive_archive_or_data_deletion",
			Reason:               "destructive data/archive advice lacks evidence, backup, approval, maintenance-window, rollback, or scope gate",
		}
	}
	highRiskReplicationRepair := hasAnyRiskMarker(text, []string{
		"pg_rewind",
		" pg rewind",
		"promote",
		"manual failover",
		"perform failover",
		"trigger failover",
		"execute failover",
		"fail over",
		"执行 failover",
		"触发 failover",
		"发起 failover",
		"switchover",
		"提升 tl",
		"提升 timeline",
		"提升为 primary",
		"提升为主",
		"升主",
		"主备切换",
		"权威切换",
		"authority switch",
		"以 tl",
		"为主重建",
		"全量重建",
		"pg_basebackup",
	})
	hasApprovalGate := hasAnyRiskMarker(text, []string{"审批", "批准", "approval", "approved"}) &&
		hasAnyRiskMarker(text, []string{"备份", "backup", "snapshot", "快照"}) &&
		hasAnyRiskMarker(text, []string{"确认", "先确认", "verify", "权威", "authoritative"}) &&
		hasAnyRiskMarker(text, []string{"回滚", "rollback", "验收", "post-check", "维护窗口"})
	if highRiskReplicationRepair && !hasApprovalGate {
		return RiskyAdviceResult{
			RequiresEvidenceGate: true,
			Category:             "high_risk_replication_repair",
			Reason:               "replication repair advice lacks approval, backup, authority, rollback, or validation gate",
		}
	}
	if unsupportedLineageAuthorityInference(text) {
		return RiskyAdviceResult{
			RequiresEvidenceGate: true,
			Category:             "unsupported_lineage_authority_inference",
			Reason:               "lineage authority advice infers stale primary or authoritative data from branch id without lineage and data-authority evidence",
		}
	}
	if containsMutationCommandAdvice(text) {
		return RiskyAdviceResult{
			RequiresEvidenceGate: true,
			Category:             riskyAdviceCategoryUngatedMutationCommandAdvice,
			Reason:               "mutation command advice must be tied to explicit target binding, execution policy, approval, and validation evidence",
		}
	}
	return RiskyAdviceResult{}
}

func hasAnyRiskMarker(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func unsupportedLineageAuthorityInference(text string) bool {
	if !hasAnyRiskMarker(text, []string{"lineage", "branch", "history", "timeline", "tl "}) {
		return false
	}
	if !hasAnyRiskMarker(text, []string{"更高", "数值更高", "branch 9", "timeline 9", "tl 9", "高 2", "selected new branch", "selected new timeline"}) {
		return false
	}
	if !hasAnyRiskMarker(text, []string{
		"拥有更权威",
		"拥有权威",
		"更权威的数据",
		"权威源重建",
		"为权威源",
		"是权威",
		"是过期主库",
		"过期主库",
		"stale primary",
	}) {
		return false
	}
	if hasAnyRiskMarker(text, []string{
		"无法确认权威",
		"不能确认权威",
		"不可断言权威",
		"不能仅凭 timeline",
		"不能仅凭更高",
		"timeline id 数值大小本身不代表数据新旧",
		"timeline 数字不代表",
	}) && !hasAnyRiskMarker(text, []string{"拥有更权威", "权威源重建", "为权威源", "是过期主库", "stale primary"}) {
		return false
	}
	return true
}

func containsMutationCommandAdvice(text string) bool {
	normalized := strings.NewReplacer(
		"\n", " ",
		"\r", " ",
		"\t", " ",
		"`", " ",
		"；", ";",
		"。", ".",
	).Replace(strings.ToLower(text))
	if hasAnyRiskMarker(normalized, []string{
		"systemctl restart",
		"systemctl stop",
		"systemctl start",
		"systemctl reload",
		"systemctl kill",
		"kubectl rollout restart",
		"kubectl delete",
		"docker restart",
		"docker stop",
		"docker rm",
		"docker kill",
		"nginx -s reload",
		"nginx -s stop",
		"nginx -s quit",
		"kill -9",
		"pkill ",
	}) {
		return true
	}
	fields := commandAdviceFields(normalized)
	for i, token := range fields {
		if token == "sudo" && i+1 < len(fields) {
			token = fields[i+1]
			i++
		}
		switch token {
		case "systemctl":
			if i+1 < len(fields) && isServiceMutationVerb(fields[i+1]) {
				return true
			}
		case "service":
			if i+2 < len(fields) && isServiceMutationVerb(fields[i+2]) {
				return true
			}
		case "kubectl":
			if i+1 < len(fields) && (fields[i+1] == "delete" || fields[i+1] == "apply" || fields[i+1] == "patch" || fields[i+1] == "replace" || fields[i+1] == "scale") {
				return true
			}
		case "docker":
			if i+1 < len(fields) && (fields[i+1] == "restart" || fields[i+1] == "stop" || fields[i+1] == "rm" || fields[i+1] == "kill") {
				return true
			}
		case "nginx":
			if i+2 < len(fields) && fields[i+1] == "-s" && (fields[i+2] == "reload" || fields[i+2] == "stop" || fields[i+2] == "quit") {
				return true
			}
		case "kill":
			if standaloneKillLooksCommandLike(fields, i) {
				return true
			}
		case "pkill", "iptables", "firewall-cmd":
			return true
		}
	}
	return false
}

func standaloneKillLooksCommandLike(fields []string, index int) bool {
	if index < 0 || index >= len(fields) || index+1 >= len(fields) {
		return false
	}
	prev := ""
	if index > 0 {
		prev = strings.TrimSpace(fields[index-1])
	}
	if prev != "" && !isCommandLeadInToken(prev) {
		return false
	}
	next := strings.TrimSpace(fields[index+1])
	return strings.HasPrefix(next, "-") || isAllDigits(next)
}

func isCommandLeadInToken(token string) bool {
	token = strings.TrimSpace(token)
	switch token {
	case "", "sudo", "run", "execute", "exec", "command", "cmd", "执行", "运行", "命令":
		return true
	default:
		return strings.HasSuffix(token, "执行") || strings.HasSuffix(token, "运行")
	}
}

func isAllDigits(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func commandAdviceFields(text string) []string {
	raw := strings.Fields(text)
	fields := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.Trim(token, "`'\"“”‘’。，,;:()[]{}<>")
		if token != "" {
			fields = append(fields, token)
		}
	}
	return fields
}

func isServiceMutationVerb(token string) bool {
	switch strings.TrimSpace(token) {
	case "restart", "stop", "start", "reload", "kill", "disable", "enable":
		return true
	default:
		return false
	}
}
