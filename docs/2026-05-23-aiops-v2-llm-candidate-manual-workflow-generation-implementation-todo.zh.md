# aiops-v2 LLM 候选运维手册与候选 Workflow 生成 Implementation Plan

日期：2026-05-23
状态：实施任务清单
关联设计：`docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-design.zh.md`
目标版本：P0 先实现 LLM 同时生成候选运维手册和候选 Runner Workflow，状态固定 `pending_review`，并用 deterministic gate、proof bundle、review queue 和 selfopt 保证可用性。
适用范围：AI Chat、OpsManual、Runner Workflow、Run Record、RCA Report、Self-Optimization Lab、Review Queue、Experience / Memory

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` when implementing this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** 让 aiops-v2 能用 LLM 生成一对绑定的 `candidate_ops_manual` 与 `candidate_runner_workflow`，但生成结果只能进入 `pending_review`，通过 proof bundle、gate 和人工 review 后才可能成为 verified 资产。

**Architecture:** 新增 `internal/candidateasset` 作为候选资产工厂，负责 bundle schema、context builder、LLM generator、normalizer、gate pipeline、proof bundle、review queue 领域逻辑；通过 `internal/server` 暴露最小 API；通过 `selfopt` 导入候选资产并运行回归评分。生产执行仍只经过现有 RuntimeKernel、ToolDispatcher、Policy、Permission、Approval、ActionToken、Runner 和 Run Record。

**Tech Stack:** Go 1.24.3、`internal/opsmanual`、`pkg/runner/workflow/visual`、`pkg/runner/server/service`、`internal/modelrouter`、`internal/server`、`selfopt`、`scripts/self-optimization-lab.sh`、Playwright、现有 prompt-regression / agent-eval。

---

## 0. 硬边界

- [ ] LLM 只能生成候选资产，不能生成 `verified`、`approved`、`published` 状态。
- [ ] `candidate_ops_manual` 和 `candidate_runner_workflow` 必须绑定在同一个 bundle 里。
- [ ] 新生成 bundle 默认状态必须是 `pending_review` 或更低。
- [ ] 生成阶段不能自动发布 verified 手册，不能自动执行 Workflow。
- [ ] `search_ops_manuals` 默认不能把 candidate 当作 `direct_execute` verified 手册。
- [ ] LLM 输出不能决定 pass/fail、安全授权、审批、ActionToken、生产执行状态。
- [ ] 所有 secret scan 失败必须 P0 block。
- [ ] 默认测试不调用真实 LLM、不访问远程主机、不执行生产写操作。
- [ ] 如需 LLM 建议，只能使用 `AIOPS_LAB_LLM_*`，不能静默复用生产 `AIOPS_LLM_*`。
- [ ] Review approve 是唯一进入 verified 的路径，且必须有 proof bundle。

---

## 1. 目标文件结构

### 1.1 新增候选资产领域包

- [ ] Create: `internal/candidateasset/types.go`
  定义 `Bundle`、`BundleStatus`、`SourceRef`、`CandidateOpsManual`、`CandidateRunnerWorkflow`、`ProofBundle`、`GateResult`、`ReviewRecord`、`Scorecard`。

- [ ] Create: `internal/candidateasset/store.go`
  定义 `Store` interface、`MemoryStore`，支持 save/get/list/update status/audit append。

- [ ] Create: `internal/candidateasset/file_store.go`
  JSON file store，存储 `.data/candidate-assets/bundles.json`。

- [ ] Create: `internal/candidateasset/context_builder.go`
  从 OperationFrame、Run Record、RCA、manual search result、preflight result 构建脱敏 LLM 输入。

- [ ] Create: `internal/candidateasset/generator.go`
  定义 LLM generator interface 与 deterministic fallback generator。

- [ ] Create: `internal/candidateasset/normalizer.go`
  固定 candidate 状态、补系统字段、清除非法状态、计算 workflow digest、绑定 manual/workflow。

- [ ] Create: `internal/candidateasset/gates.go`
  Gate pipeline 入口，串联 schema/manual/workflow/safety/retrieval/secret/selfopt gate。

- [ ] Create: `internal/candidateasset/manual_gate.go`
  校验候选手册完整性、状态、参数、risk policy、workflow ref。

- [ ] Create: `internal/candidateasset/workflow_gate.go`
  校验 Runner graph、阶段、只读 preflight、高风险审批、verify/rollback。

- [ ] Create: `internal/candidateasset/proof.go`
  写 proof bundle 和 Markdown review summary。

- [ ] Create: `internal/candidateasset/review.go`
  实现 approve / needs_changes / reject / deprecate 领域逻辑。

- [ ] Create: `internal/candidateasset/scorecard.go`
  聚合 manual、workflow、retrieval、safety、review readiness 分数。

### 1.2 测试文件

- [ ] Create: `internal/candidateasset/types_test.go`
- [ ] Create: `internal/candidateasset/store_test.go`
- [ ] Create: `internal/candidateasset/context_builder_test.go`
- [ ] Create: `internal/candidateasset/generator_test.go`
- [ ] Create: `internal/candidateasset/normalizer_test.go`
- [ ] Create: `internal/candidateasset/gates_test.go`
- [ ] Create: `internal/candidateasset/proof_test.go`
- [ ] Create: `internal/candidateasset/review_test.go`
- [ ] Create: `internal/candidateasset/scorecard_test.go`

### 1.3 与现有模块集成

- [ ] Modify: `internal/opsmanual/service.go`
  增加从 candidate bundle 读取/写入 `ManualCandidate` 的桥接方法，不改变 verified 检索默认路径。

- [ ] Modify: `internal/opsmanual/types.go`
  如现有 `ManualCandidate` 字段不足，补充 `BundleID`、`ProofBundleRef`、`ReviewStatus`、`WorkflowCandidateRef`。

- [ ] Modify: `pkg/runner/server/service/visual_workflow_ai.go`
  复用现有 AI draft 能力时，保证输出 workflow status 固定 draft，不能发布。

- [ ] Modify: `pkg/runner/workflow/visual/validate.go`
  不改核心行为；只在 candidate gate 中调用现有 `ValidateGraph`。

- [ ] Create: `internal/server/candidate_asset_api.go`
  Review queue 和候选生成 API。

- [ ] Create: `internal/server/candidate_asset_api_test.go`
  API 层行为测试。

- [ ] Modify: `internal/server/http.go`
  注册 candidate asset API route。

- [ ] Modify: `selfopt/types.go`
  增加 candidate asset score 输入摘要。

- [ ] Create: `selfopt/candidateasset/reader.go`
  读取 candidate bundle / proof bundle。

- [ ] Create: `selfopt/candidateasset/reader_test.go`
  覆盖 candidate gate 和 scorecard 导入。

- [ ] Modify: `scripts/self-optimization-lab.sh`
  增加候选资产目录参数，例如 `--candidate-assets .data/candidate-assets`。

### 1.4 测试数据和浏览器

- [ ] Create: `testdata/candidate_assets/run_record_redis_rca_success.json`
- [ ] Create: `testdata/candidate_assets/rca_report_coroot_checkout_latency.json`
- [ ] Create: `testdata/candidate_assets/manual_no_match_k8s_install.json`
- [ ] Create: `testdata/candidate_assets/expected_candidate_bundle_redis_rca.json`
- [ ] Create: `testdata/self_optimization/eval_cases/candidate-manual-workflow-generation.json`
- [ ] Create: `web/tests/e2e/candidate-asset-review.spec.js`

### 1.5 文档

- [ ] Modify: `docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-design.zh.md`
  如果实施时字段或边界有变化，回写设计。

- [ ] Create: `docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-test-report.zh.md`
  记录测试命令、结果、截图和剩余风险。

---

## 2. Task 0: 基线冻结与范围确认

**Files:**
- Read: `docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-design.zh.md`
- Read: `internal/opsmanual/types.go`
- Read: `internal/opsmanual/service.go`
- Read: `internal/opsmanual/workflow_manual_types.go`
- Read: `internal/opsmanual/manual_candidate_validation.go`
- Read: `pkg/runner/workflow/visual/validate.go`
- Read: `pkg/runner/server/service/visual_workflow_ai.go`
- Read: `selfopt/types.go`
- Read: `scripts/self-optimization-lab.sh`

- [ ] **Step 0.1: 记录工作区状态**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
git status --short
git rev-parse HEAD
```

Expected:

- 记录当前 commit hash。
- 若存在未提交变更，不回滚无关文件。

- [ ] **Step 0.2: 确认第一阶段范围**

第一阶段只交付：

- candidate bundle schema/store
- LLM generator deterministic fallback
- normalizer
- gate pipeline
- proof bundle
- review queue API
- selfopt 接入
- K8s install 与 Coroot RCA 两个 seed 场景

第一阶段不做：

- 自动 verified
- 自动生产执行
- 自动替换已有 verified 手册
- 默认真实 LLM
- 默认远程主机

- [ ] **Step 0.3: 提交基线说明**

无需代码提交；在后续测试报告中记录 `git rev-parse HEAD` 和 dirty worktree 摘要。

---

## 3. Task 1: Candidate Asset Bundle Schema 与 Store

**Files:**
- Create: `internal/candidateasset/types.go`
- Create: `internal/candidateasset/store.go`
- Create: `internal/candidateasset/file_store.go`
- Create: `internal/candidateasset/types_test.go`
- Create: `internal/candidateasset/store_test.go`

- [ ] **Step 1.1: 写 schema 失败测试**

Add test in `internal/candidateasset/types_test.go`:

```go
func TestNewBundleDefaultsToPendingReviewAndBindsManualWorkflow(t *testing.T) {
	now := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)
	bundle := NewBundle(NewBundleInput{
		ID: "cab-test",
		Source: Source{
			Type: SourceManualNoMatch,
			Refs: []SourceRef{{Type: "eval_case", ID: "case-1"}},
		},
		Manual: CandidateOpsManual{ID: "manual-candidate"},
		Workflow: CandidateRunnerWorkflow{ID: "workflow-candidate", Digest: "sha256:abc"},
		Now: now,
	})

	if bundle.Status != BundleStatusPendingReview {
		t.Fatalf("status = %q, want pending_review", bundle.Status)
	}
	if bundle.Manual.Status == "verified" {
		t.Fatal("generated manual must not be verified")
	}
	if bundle.Manual.WorkflowRef.WorkflowDigest != "sha256:abc" {
		t.Fatalf("manual workflow digest not bound: %+v", bundle.Manual.WorkflowRef)
	}
	if bundle.Review.Status != ReviewStatusPending {
		t.Fatalf("review status = %q", bundle.Review.Status)
	}
}
```

- [ ] **Step 1.2: 运行失败测试**

Run:

```bash
go test ./internal/candidateasset -run TestNewBundleDefaultsToPendingReviewAndBindsManualWorkflow -count=1
```

Expected:

- FAIL because package/types do not exist.

- [ ] **Step 1.3: 实现最小 schema**

Create `internal/candidateasset/types.go` with:

- `BundleStatusDraft`
- `BundleStatusGenerated`
- `BundleStatusValidated`
- `BundleStatusSandboxPassed`
- `BundleStatusPendingReview`
- `BundleStatusVerified`
- `BundleStatusNeedsChanges`
- `BundleStatusRejected`
- `BundleStatusDeprecated`
- `Source`
- `SourceRef`
- `CandidateOpsManual`
- `CandidateRunnerWorkflow`
- `ProofBundle`
- `Review`
- `ReviewNote`
- `Bundle`
- `NewBundleInput`
- `NewBundle(input NewBundleInput) Bundle`

Implementation requirements:

- Empty generated status becomes `pending_review`.
- Manual status never becomes `verified` during `NewBundle`.
- Manual workflow digest is copied from workflow digest.
- `CreatedAt` and `UpdatedAt` use `input.Now`; if zero, use `time.Now().UTC()`.

- [ ] **Step 1.4: 运行 schema 测试**

Run:

```bash
go test ./internal/candidateasset -run TestNewBundleDefaultsToPendingReviewAndBindsManualWorkflow -count=1
```

Expected:

- PASS.

- [ ] **Step 1.5: 写 store 失败测试**

Add test in `internal/candidateasset/store_test.go`:

```go
func TestFileStorePersistsBundleAndAuditEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bundles.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	bundle := NewBundle(NewBundleInput{
		ID: "cab-store",
		Source: Source{Type: SourceRunRecord, Refs: []SourceRef{{Type: "run_record", ID: "run-1"}}},
		Manual: CandidateOpsManual{ID: "manual-store"},
		Workflow: CandidateRunnerWorkflow{ID: "workflow-store", Digest: "sha256:store"},
		Now: time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC),
	})
	if err := store.SaveBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendAuditEvent(context.Background(), "cab-store", AuditEvent{Type: "created", Actor: "test"}); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetBundle(context.Background(), "cab-store")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "cab-store" || len(got.AuditEvents) != 1 {
		t.Fatalf("unexpected bundle: %+v", got)
	}
}
```

- [ ] **Step 1.6: 运行 store 失败测试**

Run:

```bash
go test ./internal/candidateasset -run TestFileStorePersistsBundleAndAuditEvents -count=1
```

Expected:

- FAIL until store is implemented.

- [ ] **Step 1.7: 实现 Store**

Create:

- `Store` interface with `SaveBundle`、`GetBundle`、`ListBundles`、`UpdateBundleStatus`、`AppendAuditEvent`
- `MemoryStore`
- `FileStore`

Rules:

- Save must deep-copy bundle.
- List returns deterministic order by `CreatedAt` then `ID`.
- Unknown bundle returns clear not found error.
- File store writes JSON atomically using temp file + rename.

- [ ] **Step 1.8: 运行 package 测试**

Run:

```bash
go test ./internal/candidateasset -count=1
```

Expected:

- PASS.

- [ ] **Step 1.9: Commit**

```bash
git add internal/candidateasset/types.go internal/candidateasset/store.go internal/candidateasset/file_store.go internal/candidateasset/types_test.go internal/candidateasset/store_test.go
git commit -m "feat: add candidate asset bundle store"
```

---

## 4. Task 2: Context Builder 与脱敏输入

**Files:**
- Create: `internal/candidateasset/context_builder.go`
- Create: `internal/candidateasset/context_builder_test.go`
- Modify: `internal/candidateasset/types.go`

- [ ] **Step 2.1: 写脱敏失败测试**

Add test:

```go
func TestBuildContextRedactsSecretsAndSummarizesSources(t *testing.T) {
	input := BuildContextInput{
		UserRequest: "Redis p95 high; password=raw-pass token=raw-token",
		OperationFrame: map[string]any{"target_type": "redis", "action": "rca_or_repair"},
		Evidence: []EvidenceSummary{{Source: "coroot", Summary: "rss rising", RawRef: "metric://redis/rss"}},
		RunRecord: &RunRecordSummary{ID: "run-1", Status: "success", Summary: "CONFIG SET avoided"},
	}
	ctx, err := BuildContext(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(ctx)
	text := string(raw)
	for _, forbidden := range []string{"raw-pass", "raw-token", "password=raw-pass", "token=raw-token"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("context leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "redis") || !strings.Contains(text, "rss rising") || !strings.Contains(text, "run-1") {
		t.Fatalf("context missing useful summary: %s", text)
	}
}
```

- [ ] **Step 2.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestBuildContextRedactsSecretsAndSummarizesSources -count=1
```

Expected:

- FAIL until context builder exists.

- [ ] **Step 2.3: 实现 BuildContext**

Create types:

- `BuildContextInput`
- `CandidateGenerationContext`
- `EvidenceSummary`
- `ManualSearchSummary`
- `PreflightSummary`
- `RunRecordSummary`
- `RCAReportSummary`

Implementation:

- Redact `password=...`、`token=...`、`Authorization: Bearer ...`、`sk-...`
- Limit raw text fields to 2,000 chars each.
- Preserve source refs and summaries.
- Do not include raw tool output by default.

- [ ] **Step 2.4: 运行测试**

```bash
go test ./internal/candidateasset -run TestBuildContextRedactsSecretsAndSummarizesSources -count=1
```

Expected:

- PASS.

- [ ] **Step 2.5: Commit**

```bash
git add internal/candidateasset/context_builder.go internal/candidateasset/context_builder_test.go internal/candidateasset/types.go
git commit -m "feat: build redacted candidate generation context"
```

---

## 5. Task 3: LLM Generator 与 deterministic fallback

**Files:**
- Create: `internal/candidateasset/generator.go`
- Create: `internal/candidateasset/generator_test.go`
- Modify: `internal/candidateasset/types.go`

- [ ] **Step 3.1: 写 generator 安全测试**

Add test:

```go
func TestGeneratorOutputIsAlwaysPendingReview(t *testing.T) {
	gen := DeterministicGenerator{}
	ctx := CandidateGenerationContext{
		UserRequestSummary: "在 Kubernetes 上安装基础集群",
		OperationFrame: map[string]any{"target_type": "kubernetes_cluster", "action": "install_kubernetes"},
	}
	draft, err := gen.Generate(context.Background(), GenerateInput{Context: ctx})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Manual.Status == "verified" || draft.Workflow.Status == "published" {
		t.Fatalf("generator produced publishable asset: %+v", draft)
	}
	if draft.Manual.ID == "" || draft.Workflow.ID == "" {
		t.Fatalf("generator must produce both assets: %+v", draft)
	}
}
```

- [ ] **Step 3.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestGeneratorOutputIsAlwaysPendingReview -count=1
```

Expected:

- FAIL until generator is implemented.

- [ ] **Step 3.3: 实现 generator interface**

Create:

- `Generator` interface
- `GenerateInput`
- `CandidateDraft`
- `DeterministicGenerator`
- `LLMGenerator`

Rules:

- `DeterministicGenerator` must not call network.
- `LLMGenerator` only reads lab LLM config explicitly passed in.
- `LLMGenerator` response must be parsed as JSON; invalid JSON returns error.
- `Generate` returns draft only, never verified.

- [ ] **Step 3.4: 写 LLM 配置隔离测试**

Add test:

```go
func TestLLMGeneratorRequiresLabConfig(t *testing.T) {
	gen := LLMGenerator{}
	_, err := gen.Generate(context.Background(), GenerateInput{Context: CandidateGenerationContext{UserRequestSummary: "x"}})
	if err == nil || !strings.Contains(err.Error(), "lab LLM") {
		t.Fatalf("expected lab LLM config error, got %v", err)
	}
}
```

- [ ] **Step 3.5: 运行 generator 测试**

```bash
go test ./internal/candidateasset -run 'TestGeneratorOutputIsAlwaysPendingReview|TestLLMGeneratorRequiresLabConfig' -count=1
```

Expected:

- PASS.

- [ ] **Step 3.6: Commit**

```bash
git add internal/candidateasset/generator.go internal/candidateasset/generator_test.go internal/candidateasset/types.go
git commit -m "feat: add candidate asset generator"
```

---

## 6. Task 4: Deterministic Normalizer

**Files:**
- Create: `internal/candidateasset/normalizer.go`
- Create: `internal/candidateasset/normalizer_test.go`
- Modify: `internal/candidateasset/types.go`

- [ ] **Step 4.1: 写状态清理和 digest 绑定失败测试**

Add test:

```go
func TestNormalizeCandidateDraftClearsIllegalStatesAndBindsDigest(t *testing.T) {
	draft := CandidateDraft{
		Manual: CandidateOpsManual{ID: "manual-1", Status: "verified"},
		Workflow: CandidateRunnerWorkflow{
			ID: "workflow-1",
			Status: "published",
			Graph: map[string]any{"workflow": map[string]any{"name": "wf"}, "nodes": []any{}, "edges": []any{}},
		},
	}
	bundle, err := NormalizeDraft(NormalizeInput{
		ID: "cab-normalize",
		Draft: draft,
		Source: Source{Type: SourceHumanRequest},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Status != BundleStatusPendingReview {
		t.Fatalf("bundle status = %q", bundle.Status)
	}
	if bundle.Manual.Status == "verified" || bundle.Workflow.Status == "published" {
		t.Fatalf("illegal status survived: %+v", bundle)
	}
	if bundle.Workflow.Digest == "" || bundle.Manual.WorkflowRef.WorkflowDigest != bundle.Workflow.Digest {
		t.Fatalf("digest not bound: %+v", bundle)
	}
}
```

- [ ] **Step 4.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestNormalizeCandidateDraftClearsIllegalStatesAndBindsDigest -count=1
```

Expected:

- FAIL until normalizer exists.

- [ ] **Step 4.3: 实现 normalizer**

Rules:

- Force bundle status to `pending_review` unless gates later set `needs_changes`.
- Force manual status to `draft`.
- Force workflow status to `draft`.
- Compute digest from canonical workflow graph JSON.
- Copy workflow digest into manual workflow ref.
- Ensure bundle ID, source refs, created/updated timestamps exist.
- Drop LLM-provided `approved_by`、`published_at`、`verified_at` metadata.

- [ ] **Step 4.4: 运行 normalizer 测试**

```bash
go test ./internal/candidateasset -run TestNormalizeCandidateDraftClearsIllegalStatesAndBindsDigest -count=1
```

Expected:

- PASS.

- [ ] **Step 4.5: Commit**

```bash
git add internal/candidateasset/normalizer.go internal/candidateasset/normalizer_test.go internal/candidateasset/types.go
git commit -m "feat: normalize generated candidate assets"
```

---

## 7. Task 5: Manual / Workflow / Safety Gate Pipeline

**Files:**
- Create: `internal/candidateasset/gates.go`
- Create: `internal/candidateasset/manual_gate.go`
- Create: `internal/candidateasset/workflow_gate.go`
- Create: `internal/candidateasset/gates_test.go`
- Modify: `internal/candidateasset/types.go`

- [ ] **Step 5.1: 写 P0 gate 失败测试**

Add test:

```go
func TestGatePipelineBlocksVerifiedCandidateAndMissingApproval(t *testing.T) {
	bundle := validCandidateBundleForGate(t)
	bundle.Manual.Status = "verified"
	bundle.Workflow.Nodes[0].RiskLevel = "high"
	bundle.Workflow.Nodes[0].RequiresApproval = false
	bundle.Workflow.Nodes[0].RequiresActionToken = false

	result := RunGates(context.Background(), bundle, GateOptions{})
	if result.Decision != GateDecisionBlock {
		t.Fatalf("decision = %q, want block: %+v", result.Decision, result)
	}
	if !result.HasIssue("candidate_status_verified") {
		t.Fatalf("missing verified status issue: %+v", result.Issues)
	}
	if !result.HasIssue("high_risk_without_approval") {
		t.Fatalf("missing approval issue: %+v", result.Issues)
	}
}
```

- [ ] **Step 5.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestGatePipelineBlocksVerifiedCandidateAndMissingApproval -count=1
```

Expected:

- FAIL until gates exist.

- [ ] **Step 5.3: 实现 gate types**

Define:

- `GateDecisionPass`
- `GateDecisionWarn`
- `GateDecisionBlock`
- `GateSeverityP0`
- `GateSeverityP1`
- `GateIssue`
- `GateReport`
- `GateOptions`
- `RunGates(ctx, bundle, opts) GateReport`

- [ ] **Step 5.4: 实现 Manual Gate**

Block when:

- manual status is `verified`
- missing operation target/action
- missing required inputs
- missing required evidence
- empty `cannot_use_when`
- high risk without risk policy
- workflow digest missing or mismatch

- [ ] **Step 5.5: 实现 Workflow Gate**

Block when:

- graph invalid
- no preflight phase
- preflight node not read-only
- no verify phase
- high-risk node missing approval
- high-risk node missing ActionToken
- no rollback and no explicit irreversible reason

- [ ] **Step 5.6: 实现 Safety / Secret Gate**

Block when:

- candidate text contains secret patterns
- workflow script contains raw token/password
- LLM metadata claims execution completed
- candidate source is empty

- [ ] **Step 5.7: 运行 gate 测试**

```bash
go test ./internal/candidateasset -run 'TestGatePipeline|TestManualGate|TestWorkflowGate' -count=1
```

Expected:

- PASS.

- [ ] **Step 5.8: Commit**

```bash
git add internal/candidateasset/gates.go internal/candidateasset/manual_gate.go internal/candidateasset/workflow_gate.go internal/candidateasset/gates_test.go internal/candidateasset/types.go
git commit -m "feat: gate candidate manual workflow bundles"
```

---

## 8. Task 6: Proof Bundle Writer

**Files:**
- Create: `internal/candidateasset/proof.go`
- Create: `internal/candidateasset/proof_test.go`
- Modify: `internal/candidateasset/types.go`

- [ ] **Step 6.1: 写 proof bundle 完整性测试**

Add test:

```go
func TestProofBundleWriterIncludesAllGateOutputs(t *testing.T) {
	dir := t.TempDir()
	bundle := validCandidateBundleForGate(t)
	gates := GateReport{Decision: GateDecisionPass}
	scorecard := Scorecard{ReviewReadinessScore: 1.0}

	proof, err := WriteProofBundle(dir, bundle, gates, scorecard)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"proof-bundle.json",
		"review-summary.zh.md",
		"manual-validation.json",
		"workflow-validation.json",
		"safety-scan.json",
		"selfopt-scorecard.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	if proof.Status != "ready_for_review" {
		t.Fatalf("proof status = %q", proof.Status)
	}
}
```

- [ ] **Step 6.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestProofBundleWriterIncludesAllGateOutputs -count=1
```

Expected:

- FAIL until proof writer exists.

- [ ] **Step 6.3: 实现 proof writer**

Write:

- `proof-bundle.json`
- `review-summary.zh.md`
- `manual-validation.json`
- `workflow-validation.json`
- `retrieval-tests.json`
- `dry-run.json`
- `safety-scan.json`
- `secret-scan.json`
- `selfopt-scorecard.json`

Rules:

- Markdown summary must not include raw scripts longer than 40 lines.
- Secret scan must run on all proof files.
- P0 gate failure sets proof status `blocked`.

- [ ] **Step 6.4: 运行 proof 测试**

```bash
go test ./internal/candidateasset -run TestProofBundleWriterIncludesAllGateOutputs -count=1
```

Expected:

- PASS.

- [ ] **Step 6.5: Commit**

```bash
git add internal/candidateasset/proof.go internal/candidateasset/proof_test.go internal/candidateasset/types.go
git commit -m "feat: write candidate asset proof bundles"
```

---

## 9. Task 7: Review Queue 领域逻辑

**Files:**
- Create: `internal/candidateasset/review.go`
- Create: `internal/candidateasset/review_test.go`
- Modify: `internal/candidateasset/store.go`

- [ ] **Step 7.1: 写 review 阻断测试**

Add test:

```go
func TestReviewApproveRequiresPassingProofBundle(t *testing.T) {
	store := NewMemoryStore()
	bundle := validCandidateBundleForGate(t)
	bundle.ProofBundle.Status = "blocked"
	if err := store.SaveBundle(context.Background(), bundle); err != nil {
		t.Fatal(err)
	}
	service := NewReviewService(store)

	_, err := service.Review(context.Background(), ReviewInput{
		BundleID: "cab-valid",
		Action: ReviewActionApprove,
		Reviewer: "sre",
		Note: "looks good",
	})
	if err == nil || !strings.Contains(err.Error(), "proof bundle") {
		t.Fatalf("expected proof bundle error, got %v", err)
	}
}
```

- [ ] **Step 7.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestReviewApproveRequiresPassingProofBundle -count=1
```

Expected:

- FAIL until review service exists.

- [ ] **Step 7.3: 实现 ReviewService**

Actions:

- `approve`: require proof status `ready_for_review`, gate pass, reviewer non-empty; set bundle status `verified_pending_publish` or emit verified artifact request, but do not mutate production manual store in this task.
- `needs_changes`: set bundle status `needs_changes`, append reviewer note.
- `reject`: set bundle status `rejected`.
- `deprecate`: only allowed for verified bundle; set deprecated.

Important:

- Generation path still never calls approve.
- Every review action writes audit event.

- [ ] **Step 7.4: 运行 review 测试**

```bash
go test ./internal/candidateasset -run 'TestReview' -count=1
```

Expected:

- PASS.

- [ ] **Step 7.5: Commit**

```bash
git add internal/candidateasset/review.go internal/candidateasset/review_test.go internal/candidateasset/store.go
git commit -m "feat: add candidate asset review queue"
```

---

## 10. Task 8: 生成编排 Service

**Files:**
- Create: `internal/candidateasset/service.go`
- Create: `internal/candidateasset/service_test.go`
- Modify: `internal/candidateasset/generator.go`
- Modify: `internal/candidateasset/normalizer.go`
- Modify: `internal/candidateasset/gates.go`
- Modify: `internal/candidateasset/proof.go`

- [ ] **Step 8.1: 写端到端生成测试**

Add test:

```go
func TestServiceGenerateCreatesPendingReviewManualAndWorkflow(t *testing.T) {
	root := t.TempDir()
	store := NewMemoryStore()
	svc := NewService(ServiceOptions{
		Store: store,
		Generator: DeterministicGenerator{},
		ProofRoot: root,
	})
	bundle, err := svc.Generate(context.Background(), GenerateBundleRequest{
		Source: Source{Type: SourceManualNoMatch, Refs: []SourceRef{{Type: "case", ID: "k8s-install"}}},
		Context: CandidateGenerationContext{
			UserRequestSummary: "在测试主机上安装 Kubernetes",
			OperationFrame: map[string]any{"target_type": "kubernetes_cluster", "action": "install_kubernetes"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Status != BundleStatusPendingReview {
		t.Fatalf("status = %q", bundle.Status)
	}
	if bundle.Manual.ID == "" || bundle.Workflow.ID == "" {
		t.Fatalf("manual and workflow must both exist: %+v", bundle)
	}
	if bundle.ProofBundle.Ref == "" {
		t.Fatalf("proof bundle ref missing: %+v", bundle.ProofBundle)
	}
	saved, err := store.GetBundle(context.Background(), bundle.ID)
	if err != nil || saved.ID != bundle.ID {
		t.Fatalf("bundle not saved: %+v err=%v", saved, err)
	}
}
```

- [ ] **Step 8.2: 运行失败测试**

```bash
go test ./internal/candidateasset -run TestServiceGenerateCreatesPendingReviewManualAndWorkflow -count=1
```

Expected:

- FAIL until service exists.

- [ ] **Step 8.3: 实现 Service.Generate**

Pipeline:

```text
BuildContext
-> Generator.Generate
-> NormalizeDraft
-> RunGates
-> Scorecard
-> WriteProofBundle
-> Store.SaveBundle
```

Rules:

- If P0 gate blocks, set status `needs_changes`.
- If pass/warn, set status `pending_review`.
- Return saved bundle.

- [ ] **Step 8.4: 运行 candidateasset 全量测试**

```bash
go test ./internal/candidateasset -count=1
```

Expected:

- PASS.

- [ ] **Step 8.5: Commit**

```bash
git add internal/candidateasset/service.go internal/candidateasset/service_test.go internal/candidateasset
git commit -m "feat: orchestrate candidate asset generation"
```

---

## 11. Task 9: OpsManual 与 Runner 桥接

**Files:**
- Modify: `internal/opsmanual/types.go`
- Modify: `internal/opsmanual/service.go`
- Create: `internal/opsmanual/candidate_asset_bridge.go`
- Create: `internal/opsmanual/candidate_asset_bridge_test.go`
- Modify: `pkg/runner/server/service/visual_workflow_ai.go`
- Modify: `pkg/runner/server/service/visual_workflow_service_test.go`

- [ ] **Step 9.1: 写 candidate 不参与 direct execute 测试**

Add test:

```go
func TestCandidateAssetManualIsNotReturnedAsDirectExecute(t *testing.T) {
	repo := NewMemoryStore()
	service := NewService(repo)
	candidate := ManualCandidate{
		ID: "candidate-manual",
		ReviewStatus: "pending_review",
		ProposedManual: redisMemoryManual(),
	}
	if err := repo.SaveCandidate(candidate); err != nil {
		t.Fatal(err)
	}
	result, err := service.SearchOpsManuals(SearchOpsManualsRequest{
		Text: "Redis 内存升高，请修复",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision == DecisionDirectExecute {
		t.Fatalf("candidate manual must not direct execute: %+v", result)
	}
}
```

- [ ] **Step 9.2: 运行失败测试**

```bash
go test ./internal/opsmanual -run TestCandidateAssetManualIsNotReturnedAsDirectExecute -count=1
```

Expected:

- FAIL if candidates can affect direct execute, otherwise PASS and keep as regression.

- [ ] **Step 9.3: 实现桥接**

Bridge responsibilities:

- Convert `candidateasset.CandidateOpsManual` to `opsmanual.ManualCandidate`.
- Convert candidate workflow graph metadata to `WorkflowRef`.
- Preserve `BundleID` and `ProofBundleRef`.
- Never save candidate as verified manual.

- [ ] **Step 9.4: Runner workflow draft bridge**

Ensure:

- Candidate workflow graph can call `visual.ValidateGraph`.
- Candidate workflow remains draft.
- AI draft endpoint rejects non-draft patching.

Run:

```bash
go test ./internal/opsmanual ./pkg/runner/server/service -run 'Candidate|AIDraft|ValidateGraph' -count=1
```

Expected:

- PASS.

- [ ] **Step 9.5: Commit**

```bash
git add internal/opsmanual pkg/runner/server/service
git commit -m "feat: bridge candidate assets to manuals and workflows"
```

---

## 12. Task 10: Candidate Asset API

**Files:**
- Create: `internal/server/candidate_asset_api.go`
- Create: `internal/server/candidate_asset_api_test.go`
- Modify: `internal/server/http.go`

- [ ] **Step 10.1: 写 API 失败测试**

Add tests:

```go
func TestCandidateAssetAPIGenerateCreatesPendingReviewBundle(t *testing.T) {
	server := newTestHTTPServerWithCandidateAssets(t)
	resp := postJSON(t, server, "/api/v1/candidate-assets/generate", map[string]any{
		"source": map[string]any{"type": "manual_no_match", "refs": []map[string]any{{"type": "case", "id": "k8s"}}},
		"context": map[string]any{"user_request_summary": "install k8s"},
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	var payload struct {
		Bundle candidateasset.Bundle `json:"bundle"`
	}
	decodeJSON(t, resp.Body, &payload)
	if payload.Bundle.Status != candidateasset.BundleStatusPendingReview {
		t.Fatalf("status = %q", payload.Bundle.Status)
	}
}
```

```go
func TestCandidateAssetAPIRejectsApproveWithoutProof(t *testing.T) {
	server := newTestHTTPServerWithCandidateAssets(t)
	resp := postJSON(t, server, "/api/v1/candidate-assets/cab-1/review", map[string]any{
		"action": "approve",
		"reviewer": "sre",
	})
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
}
```

- [ ] **Step 10.2: 运行失败测试**

```bash
go test ./internal/server -run TestCandidateAssetAPI -count=1
```

Expected:

- FAIL until API exists.

- [ ] **Step 10.3: 实现 API routes**

Routes:

- `POST /api/v1/candidate-assets/generate`
- `GET /api/v1/candidate-assets`
- `GET /api/v1/candidate-assets/{bundle_id}`
- `POST /api/v1/candidate-assets/{bundle_id}/review`
- `GET /api/v1/candidate-assets/{bundle_id}/proof`

Rules:

- Generate returns 201 and pending_review/needs_changes bundle.
- Review approve requires proof bundle ready.
- API response never returns raw LLM prompt with secrets.

- [ ] **Step 10.4: 运行 server 测试**

```bash
go test ./internal/server -run 'CandidateAsset|OpsManual|RunnerStudio' -count=1
```

Expected:

- PASS.

- [ ] **Step 10.5: Commit**

```bash
git add internal/server/candidate_asset_api.go internal/server/candidate_asset_api_test.go internal/server/http.go
git commit -m "feat: expose candidate asset review api"
```

---

## 13. Task 11: selfopt 接入候选资产评分

**Files:**
- Modify: `selfopt/types.go`
- Create: `selfopt/candidateasset/reader.go`
- Create: `selfopt/candidateasset/reader_test.go`
- Modify: `selfopt/run.go`
- Modify: `selfopt/cli.go`
- Modify: `scripts/self-optimization-lab.sh`

- [ ] **Step 11.1: 写 selfopt candidate 导入失败测试**

Add test:

```go
func TestSelfOptImportsCandidateAssetGateResults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "cab-1", "bundle.json"), `{
	  "id":"cab-1",
	  "status":"needs_changes",
	  "gate_report":{"decision":"block","issues":[{"id":"secret_leak","severity":"P0"}]},
	  "scorecard":{"secret_safety_score":0}
	}`)
	summary, err := candidateasset.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Blocked != 1 || summary.P0Issues != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}
```

- [ ] **Step 11.2: 运行失败测试**

```bash
go test ./selfopt/candidateasset -count=1
```

Expected:

- FAIL until reader exists.

- [ ] **Step 11.3: 实现 reader 和 scorecard 集成**

Add CLI flag:

- `--candidate-assets DIR`

Selfopt output:

- `candidate-asset-summary.json`
- `scorecard.candidateAssets`
- dashboard section `Candidate Assets`

Gate rule:

- Any candidate P0 issue blocks selfopt.
- Any candidate marked `verified` without review blocks selfopt.

- [ ] **Step 11.4: 更新 wrapper**

Add `scripts/self-optimization-lab.sh` flag:

- `--candidate-assets DIR`

Pass it to `go run ./selfopt/cmd/selfopt`.

- [ ] **Step 11.5: 运行 selfopt 测试**

```bash
go test ./selfopt ./selfopt/cmd/selfopt ./selfopt/candidateasset -count=1
```

Expected:

- PASS.

- [ ] **Step 11.6: Commit**

```bash
git add selfopt scripts/self-optimization-lab.sh
git commit -m "feat: score candidate assets in selfopt"
```

---

## 14. Task 12: Seed 场景 - K8s 安装与 Coroot RCA

**Files:**
- Create: `testdata/candidate_assets/manual_no_match_k8s_install.json`
- Create: `testdata/candidate_assets/rca_report_coroot_checkout_latency.json`
- Create: `testdata/candidate_assets/expected_candidate_bundle_k8s_install.json`
- Create: `testdata/candidate_assets/expected_candidate_bundle_coroot_rca.json`
- Create: `testdata/self_optimization/eval_cases/candidate-k8s-install-generation.json`
- Create: `testdata/self_optimization/eval_cases/candidate-coroot-rca-generation.json`
- Modify: `internal/candidateasset/service_test.go`

- [ ] **Step 12.1: 写 K8s seed 测试**

Add table test:

```go
func TestServiceGenerateSeedCandidateBundles(t *testing.T) {
	cases := []struct {
		name string
		fixture string
		wantManualAction string
		wantWorkflowStage string
	}{
		{"k8s install", "testdata/candidate_assets/manual_no_match_k8s_install.json", "install_kubernetes", "preflight"},
		{"coroot rca", "testdata/candidate_assets/rca_report_coroot_checkout_latency.json", "rca_or_repair", "verify"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := loadGenerateBundleRequest(t, tc.fixture)
			bundle, err := newDeterministicTestService(t).Generate(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if bundle.Manual.Operation.Action != tc.wantManualAction {
				t.Fatalf("action = %q", bundle.Manual.Operation.Action)
			}
			if !bundle.Workflow.HasStage(tc.wantWorkflowStage) {
				t.Fatalf("workflow missing stage %q: %+v", tc.wantWorkflowStage, bundle.Workflow)
			}
		})
	}
}
```

- [ ] **Step 12.2: 创建 fixtures**

K8s fixture must include:

- source type `manual_no_match`
- operation target `kubernetes_cluster`
- action `install_kubernetes`
- required evidence: OS, CPU/memory, disk, container runtime, ports
- risk level `high`

Coroot fixture must include:

- source type `rca_report`
- service `checkout`
- symptoms: p95 latency, error rate
- evidence refs: metrics/logs/deploy event
- action `rca_or_repair`
- risk level `high` only for repair nodes

- [ ] **Step 12.3: 运行 seed 测试**

```bash
go test ./internal/candidateasset -run TestServiceGenerateSeedCandidateBundles -count=1
```

Expected:

- PASS.

- [ ] **Step 12.4: 运行 prompt regression**

```bash
./scripts/prompt-regression.sh \
  --agent mock \
  --cases testdata/self_optimization/eval_cases \
  --out /tmp/aiops-candidate-generation-prompt-regression \
  --run-id candidate-generation-smoke
```

Expected:

- PASS with new candidate generation cases included.

- [ ] **Step 12.5: Commit**

```bash
git add testdata/candidate_assets testdata/self_optimization/eval_cases internal/candidateasset/service_test.go
git commit -m "test: add candidate asset generation seed cases"
```

---

## 15. Task 13: Review Queue UI / Browser Smoke

**Files:**
- Create: `web/tests/e2e/candidate-asset-review.spec.js`
- Modify: `web/src/pages/*` only if a route/page already exists for admin/review surfaces.
- If no page exists, keep first phase API-only and make Playwright open a static dashboard fixture from selfopt.

- [ ] **Step 13.1: 写 Playwright smoke**

Test expectations:

- Candidate list loads.
- Candidate detail shows manual + workflow + proof bundle.
- Status is `pending_review` or `needs_changes`.
- There is no visible “execute” button for pending candidate.
- Review action buttons are visible: approve, needs changes, reject.

- [ ] **Step 13.2: Run Playwright**

```bash
cd web
PLAYWRIGHT_SKIP_WEB_SERVER=1 npx playwright test tests/e2e/candidate-asset-review.spec.js --project=chromium
```

Expected:

- PASS if fixture route exists.
- If UI route is not implemented in first phase, skip with `CANDIDATE_ASSET_REVIEW_URL` absent, and add `CANDIDATE_ASSET_REVIEW_REQUIRED=1` for CI-required runs.

- [ ] **Step 13.3: Browser-in-app 验证**

Run local app or static dashboard, then use browser-in-app:

- Open candidate review surface.
- Verify pending review status.
- Verify proof bundle visible.
- Save screenshot to `/tmp/aiops-candidate-asset-review.png`.

- [ ] **Step 13.4: Commit**

```bash
git add web/tests/e2e/candidate-asset-review.spec.js
git commit -m "test: add candidate asset review browser smoke"
```

---

## 16. Task 14: 安全与回归收口

**Files:**
- Modify: `docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-test-report.zh.md`
- Modify: `docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-design.zh.md` if implementation changed any field names.

- [ ] **Step 14.1: Go 测试**

Run:

```bash
go test ./internal/candidateasset ./internal/opsmanual ./internal/server ./selfopt ./selfopt/cmd/selfopt ./selfopt/candidateasset -count=1
```

Expected:

- PASS.

- [ ] **Step 14.2: Runner 相关测试**

Run:

```bash
(cd pkg/runner && go test ./workflow/... ./server/service ./server/api -count=1)
```

Expected:

- PASS.

- [ ] **Step 14.3: Prompt regression**

Run:

```bash
./scripts/self-optimization-lab.sh --standalone --real-aiops-tests --skip-go-tests --dashboard --max-runs 1 --out /tmp/aiops-candidate-asset-selfopt
```

Expected:

- PASS.
- `scorecard.json` contains candidate asset section when `--candidate-assets` is supplied.

- [ ] **Step 14.4: Secret scan**

Run:

```bash
rg -n "sk-[A-Za-z0-9_-]{12,}|Authorization: Bearer|password=|token=|BEGIN OPENSSH PRIVATE KEY" \
  internal/candidateasset testdata/candidate_assets docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-test-report.zh.md
```

Expected:

- No real secrets.
- Synthetic sentinel strings are allowed only inside explicit negative tests and must be named as test sentinels.

- [ ] **Step 14.5: 写测试报告**

Create or update:

`docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-test-report.zh.md`

Include:

- commands
- pass/fail
- screenshots
- generated proof bundle sample path
- known limitations
- confirmation that no real LLM / remote host was used by default

- [ ] **Step 14.6: Final commit**

```bash
git add docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-test-report.zh.md docs/2026-05-23-aiops-v2-llm-candidate-manual-workflow-generation-design.zh.md
git commit -m "docs: report candidate asset generation verification"
```

---

## 17. 实施顺序建议

推荐按以下提交粒度执行：

1. `feat: add candidate asset bundle store`
2. `feat: build redacted candidate generation context`
3. `feat: add candidate asset generator`
4. `feat: normalize generated candidate assets`
5. `feat: gate candidate manual workflow bundles`
6. `feat: write candidate asset proof bundles`
7. `feat: add candidate asset review queue`
8. `feat: orchestrate candidate asset generation`
9. `feat: bridge candidate assets to manuals and workflows`
10. `feat: expose candidate asset review api`
11. `feat: score candidate assets in selfopt`
12. `test: add candidate asset generation seed cases`
13. `test: add candidate asset review browser smoke`
14. `docs: report candidate asset generation verification`

---

## 18. 完成定义

本计划完成时必须满足：

- [ ] LLM 或 deterministic fallback 能同时生成手册候选和 Workflow 候选。
- [ ] 生成资产状态只能是 `pending_review` 或 `needs_changes`。
- [ ] Workflow graph validate 通过。
- [ ] 手册 `workflow_ref.workflow_digest` 与 Workflow digest 一致。
- [ ] 高风险节点缺审批或 ActionToken 时 gate block。
- [ ] proof bundle 完整生成。
- [ ] review queue 能 approve / needs_changes / reject。
- [ ] approve 前 candidate 不进入默认 direct execute。
- [ ] selfopt 能读取候选资产并将 P0 问题纳入 gate。
- [ ] K8s install 和 Coroot RCA 两个 seed 场景能生成候选资产。
- [ ] Playwright 或 browser-in-app 有可视化验证记录。
- [ ] 默认流程不调用真实 LLM、不连接远程主机、不执行生产写操作。
- [ ] 敏感信息扫描通过。
