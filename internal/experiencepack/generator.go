package experiencepack

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CandidateInput struct {
	PackID        string                 `json:"packId,omitempty"`
	Name          string                 `json:"name,omitempty"`
	Summary       string                 `json:"summary,omitempty"`
	Category      string                 `json:"category,omitempty"`
	SourceType    string                 `json:"sourceType,omitempty"`
	Trajectory    Trajectory             `json:"trajectory"`
	Signals       []string               `json:"signals,omitempty"`
	Env           EnvironmentFingerprint `json:"env,omitempty"`
	MatchedGene   string                 `json:"matchedGene,omitempty"`
	MatchedPackID string                 `json:"matchedPackId,omitempty"`
}

type CandidateBundle struct {
	Manifest          ExperiencePackManifest   `json:"manifest"`
	Skill             SkillAsset               `json:"skill"`
	Gene              GEPGene                  `json:"gene"`
	Capsule           GEPCapsule               `json:"capsule"`
	EvolutionEvent    EvolutionEvent           `json:"evolutionEvent"`
	MemoryGraphEvents []MemoryGraphEvent       `json:"memoryGraphEvents"`
	RunnerCandidate   *RunnerWorkflowCandidate `json:"runnerCandidate,omitempty"`
}

func GenerateCandidate(input CandidateInput) (CandidateBundle, error) {
	signals := append([]string{}, input.Signals...)
	signals = append(signals, ExtractSignals(input.Trajectory.UserGoal)...)
	triggerSignals := defaultSignals(signals)
	category := input.Category
	if category == "" {
		category = inferCategory(input.Trajectory.UserGoal, input.Trajectory.Outcome)
	}
	packID := input.PackID
	if packID == "" {
		packID = "pack_" + stableSuffix(input.Trajectory.CaseID, input.Trajectory.UserGoal)
	}
	name := firstNonEmpty(input.Name, "运维经验包 "+packID)
	summary := firstNonEmpty(input.Summary, input.Trajectory.UserGoal)
	env := input.Env
	if env.OS == "" && input.Trajectory.Environment.OS != "" {
		env = input.Trajectory.Environment
	}
	if env.OS == "" && env.OSDistribution == "" {
		env = EnvironmentFingerprint{OS: "linux", OSDistribution: "unknown", HostCount: 1}
	}
	skill := SkillAsset{Type: "Skill", Path: "skills/SKILL.md", Title: name, Summary: summary, Content: skillMarkdown(name, summary, input.Trajectory)}
	skill.AssetID = HashSkillMarkdown(skill.Content)
	gene := GEPGene{
		Type: "Gene", SchemaVersion: SchemaVersionGEP, ID: "gene_" + stableSuffix(packID),
		Category: category, SignalsMatch: triggerSignals, Summary: summary,
		Preconditions:  []string{"必须确认目标对象、权限和环境指纹", "生产环境必须审批", "必须完成 Dry Run"},
		Postconditions: []string{"验证项全部通过", "Proof of Recovery 或 Completion 明确"},
		Strategy:       defaultStrategy(input.Trajectory),
		Constraints:    map[string]any{"max_files": 20, "forbidden_paths": []string{"/var/lib/pgsql/data when non-empty"}, "aiops": map[string]any{"requires_approval": true, "requires_host_lease": true, "dry_run_required": true, "max_blast_radius": map[string]any{"hosts": maxInt(1, env.HostCount)}}},
		Validation:     []string{"runner.readonly_probe:proof=" + firstNonEmpty(input.Trajectory.ProofID, "required")},
		Metadata:       Metadata{"author": "aiops-v2", "version": "0.1.0"}, Domain: "aiops",
	}
	gene.AssetID = MustHashCanonicalJSON(gene)
	if report := CheckValidationGate(gene); !report.Passed {
		return CandidateBundle{}, fmt.Errorf("%w: %s", ErrValidationFailed, strings.Join(report.BlockedReasons, "; "))
	}
	capsule := GEPCapsule{
		Type: "Capsule", SchemaVersion: SchemaVersionGEP, ID: "capsule_" + stableSuffix(packID, time.Now().UTC().Format("20060102150405")),
		Trigger: triggerSignals, Gene: gene.ID, GenesUsed: []string{gene.ID}, Summary: summary,
		Content:  "本次运维目标、执行策略、参数范围、验证方式和结果已被脱敏归纳为经验包候选。原始主机、凭证和敏感输出不进入 Capsule，只保存可复用结构和审计引用。",
		Strategy: gene.Strategy, Confidence: 0.85, BlastRadius: BlastRadius{Hosts: maxInt(1, env.HostCount)},
		Outcome: Outcome{Status: firstNonEmpty(input.Trajectory.Outcome, "success"), Score: 0.9}, SourceType: firstNonEmpty(input.SourceType, "generated"),
		EnvFingerprint: env, TriggerContext: TriggerContext{Prompt: input.Trajectory.UserGoal, SessionID: input.Trajectory.ChatSessionID, CaseID: input.Trajectory.CaseID, RunnerRunID: strings.Join(input.Trajectory.RunnerRunIDs, ","), ProofID: input.Trajectory.ProofID},
		Metadata: Metadata{"author": "aiops-v2", "version": "0.1.0"}, Domain: "aiops",
	}
	capsule.AssetID = MustHashCanonicalJSON(capsule)
	event := EvolutionEvent{Type: "EvolutionEvent", SchemaVersion: SchemaVersionGEP, ID: "evt_" + stableSuffix(packID, time.Now().UTC().Format("20060102150405")),
		Intent: category, Signals: triggerSignals, GenesUsed: []string{gene.ID}, MutationID: "mutation_" + stableSuffix(packID), BlastRadius: map[string]any{"expected": capsule.BlastRadius, "actual": capsule.BlastRadius},
		Outcome: capsule.Outcome, CapsuleID: capsule.ID, SourceType: capsule.SourceType, EnvFingerprint: env, ValidationReportID: input.Trajectory.ProofID, TriggerContext: capsule.TriggerContext,
	}
	event.AssetID = MustHashCanonicalJSON(event)
	memoryEvent := MemoryGraphEvent{Type: "MemoryGraphEvent", Kind: "outcome", ID: "mge_" + stableSuffix(packID, time.Now().UTC().Format("150405")), Timestamp: time.Now().UTC().Format(time.RFC3339),
		Signal: map[string]any{"items": triggerSignals}, Gene: map[string]any{"id": gene.ID, "asset_id": gene.AssetID}, Outcome: &capsule.Outcome,
	}
	memoryEvent.AssetID = MustHashCanonicalJSON(memoryEvent)
	required := requiredFiles()
	manifest := ExperiencePackManifest{Type: "AIOpsExperiencePack", SchemaVersion: SchemaVersionPack, ID: packID, Name: name, Title: name, Summary: summary, Domain: "aiops", Category: category, UsageMode: UsageGuided, Status: PackStatusCandidate, ReviewStatus: PackStatusReviewPending, Skill: skill,
		RequiredFiles: required, Genes: []AssetRef{{Path: "gep/genes/" + gene.ID + ".json", ID: gene.ID, AssetID: gene.AssetID}}, Capsules: []AssetRef{{Path: "gep/capsules/" + capsule.ID + ".json", ID: capsule.ID, AssetID: capsule.AssetID}},
		Events: AssetRef{Path: "gep/events.jsonl", AssetID: event.AssetID}, MemoryGraph: AssetRef{Path: "gep/memory_graph_events.jsonl", AssetID: memoryEvent.AssetID},
		Metadata: Metadata{"version": "0.1.0", "source_type": capsule.SourceType},
	}
	manifest.AssetID = MustHashCanonicalJSON(manifest)
	bundle := CandidateBundle{Manifest: manifest, Skill: skill, Gene: gene, Capsule: capsule, EvolutionEvent: event, MemoryGraphEvents: []MemoryGraphEvent{memoryEvent}}
	if len(input.Trajectory.Commands) >= 6 {
		runner := GenerateRunnerWorkflowCandidate(input.Trajectory)
		bundle.RunnerCandidate = &runner
	}
	return bundle, nil
}

func PersistCandidate(ctx context.Context, store Store, bundle CandidateBundle) error {
	if err := store.AppendSkill(ctx, bundle.Manifest.ID, bundle.Skill); err != nil {
		return err
	}
	if err := store.AppendGene(ctx, bundle.Manifest.ID, bundle.Gene); err != nil {
		return err
	}
	if err := store.AppendCapsule(ctx, bundle.Manifest.ID, bundle.Capsule); err != nil {
		return err
	}
	if err := store.AppendEvolutionEvent(ctx, bundle.Manifest.ID, bundle.EvolutionEvent); err != nil {
		return err
	}
	for _, event := range bundle.MemoryGraphEvents {
		if err := store.AppendMemoryGraphEvent(ctx, bundle.Manifest.ID, event); err != nil {
			return err
		}
	}
	if bundle.RunnerCandidate != nil {
		binding := RunnerBinding{Type: "RunnerBinding", SchemaVersion: "aiops-runner-binding-v1", ID: "binding_" + bundle.RunnerCandidate.ID, WorkflowID: bundle.RunnerCandidate.ID, WorkflowName: bundle.RunnerCandidate.WorkflowName, Status: "draft", DryRunRequired: true, ApprovalRequired: true, HostLeaseRequired: true, Published: false}
		binding.AssetID = MustHashCanonicalJSON(binding)
		if err := store.AppendRunnerBinding(ctx, bundle.Manifest.ID, binding); err != nil {
			return err
		}
		bundle.Manifest.RunnerBinding = &AssetRef{ID: binding.ID, Path: "runner/binding.json", AssetID: binding.AssetID}
	}
	bundle.Manifest.AssetID = MustHashCanonicalJSON(bundle.Manifest)
	return store.AppendManifest(ctx, bundle.Manifest)
}

func skillMarkdown(name, summary string, tr Trajectory) string {
	return fmt.Sprintf(`# %s

## 什么时候使用
%s

## 需要的输入
- 目标主机
- 操作系统和中间件版本
- 端口、数据目录和审批信息

## 前置检查
- 主机可连接
- 已获得 HostLease
- 数据目录和端口安全

## 执行策略
%s

## 安全限制
- 必须 Dry Run
- 生产环境必须审批
- 禁止覆盖已有数据目录

## 验证方式
- 使用只读验证器检查服务和指标
- 需要 Proof of Recovery 或 Proof of Completion

## 回滚方式
- 停止新增变更
- 保留快照和审计引用
- 按 Runner 回滚节点处理
`, name, summary, markdownSteps(defaultStrategy(tr)))
}

func defaultStrategy(tr Trajectory) []string {
	if strings.Contains(strings.ToLower(tr.UserGoal), "pg") || strings.Contains(tr.UserGoal, "主从") {
		return []string{"检查主机环境", "检查端口和数据目录", "部署 primary", "部署 standby", "配置复制关系", "部署 pg_mon", "执行只读验证"}
	}
	return []string{"收集证据", "生成 Dry Run", "执行受控操作", "运行验证", "生成恢复证明"}
}

func defaultSignals(signals []string) []string {
	result := []string{}
	for _, signal := range signals {
		if signal == "" {
			continue
		}
		result = append(result, signal)
	}
	for _, fallback := range []string{"aiops", "operation", "verification"} {
		if len(result) >= 3 {
			break
		}
		result = append(result, fallback)
	}
	return result
}

func inferCategory(goal, outcome string) string {
	lower := strings.ToLower(goal)
	if strings.Contains(lower, "optimize") || strings.Contains(goal, "优化") {
		return CategoryOptimize
	}
	if strings.Contains(lower, "repair") || strings.Contains(goal, "修复") || strings.Contains(goal, "故障") || strings.Contains(goal, "恢复") {
		return CategoryRepair
	}
	return CategoryInnovate
}

func markdownSteps(steps []string) string {
	lines := make([]string, 0, len(steps))
	for i, step := range steps {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, step))
	}
	return strings.Join(lines, "\n")
}

func requiredFiles() []RequiredFileAsset {
	files := []RequiredFileAsset{
		{Path: "files/input.schema.json", Kind: "input_schema", Content: `{"type":"object","additionalProperties":true}`},
		{Path: "files/evidence-requirements.json", Kind: "evidence", Content: `{"required":["host","permission","validation"]}`},
		{Path: "files/validation.md", Kind: "validation", Content: "使用白名单只读验证器证明恢复或完成。"},
		{Path: "files/rollback.md", Kind: "rollback", Content: "失败时按 Runner 回滚节点或人工 Runbook 回退，禁止删除未知数据目录。"},
		{Path: "files/safety.md", Kind: "safety", Content: "必须 Dry Run、审批、HostLease，并限制爆炸半径。"},
	}
	for i := range files {
		files[i].AssetID = HashSkillMarkdown(files[i].Content)
	}
	return files
}
