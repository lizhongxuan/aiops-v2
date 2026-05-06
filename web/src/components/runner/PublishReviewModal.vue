<script setup>
import { computed, ref, watch } from "vue";
import { RocketIcon, XIcon } from "lucide-vue-next";
import { publishRunnerStudioWorkflow } from "../../api/runnerStudioClient";
import GraphDiffSummary from "./GraphDiffSummary.vue";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  workflow: {
    type: Object,
    default: () => ({ name: "", status: "draft" }),
  },
  diffSummary: {
    type: Object,
    default: () => ({}),
  },
  riskSummary: {
    type: Object,
    default: () => ({ level: "low", items: [] }),
  },
  validationResult: {
    type: Object,
    default: () => ({ valid: false, errors: [], warnings: [] }),
  },
});

const emit = defineEmits(["close", "published"]);

const publishNote = ref("");
const aiDraftConfirmed = ref(false);
const riskAcknowledged = ref(false);
const warningAcknowledged = ref(false);
const loading = ref(false);
const error = ref("");

const workflowName = computed(() => props.workflow?.name || props.workflow?.id || "");
const validatedGraphHash = computed(() => props.workflow?.validated_graph_hash || props.workflow?.validatedGraphHash || "");
const dryRunGraphHash = computed(() => props.workflow?.dry_run_graph_hash || props.workflow?.dryRunGraphHash || "");
const isAiDraft = computed(() => Boolean(props.workflow?.ai_generated_draft || props.workflow?.aiGeneratedDraft));
const riskItems = computed(() => props.riskSummary?.items || props.riskSummary?.risks || []);
const validationErrors = computed(() => props.validationResult?.errors || []);
const validationWarnings = computed(() => props.validationResult?.warnings || []);
const hasPublishNote = computed(() => publishNote.value.trim().length > 0);
const validationPassed = computed(() => Boolean(props.validationResult?.valid) && validationErrors.value.length === 0);
const dryRunCurrent = computed(() => Boolean(dryRunGraphHash.value && dryRunGraphHash.value === validatedGraphHash.value));
const riskLevel = computed(() => String(props.riskSummary?.level || "low").toLowerCase());
const requiresRiskAcknowledgement = computed(() => ["high", "critical"].includes(riskLevel.value));
const requiresWarningAcknowledgement = computed(() => validationWarnings.value.length > 0);
const canPublish = computed(
  () =>
    Boolean(
      workflowName.value &&
        validatedGraphHash.value &&
        dryRunCurrent.value &&
        validationPassed.value &&
        hasPublishNote.value &&
        (!requiresRiskAcknowledgement.value || riskAcknowledged.value) &&
        (!requiresWarningAcknowledgement.value || warningAcknowledged.value) &&
        (!isAiDraft.value || aiDraftConfirmed.value) &&
        !loading.value,
    ),
);
const disabledReason = computed(() => {
  if (!validatedGraphHash.value) return "缺少当前 validated_graph_hash，发布前必须先校验当前 graph。";
  if (!dryRunCurrent.value) return "Dry Run 未通过或已过期，发布前必须重新 Dry Run 当前 graph。";
  if (!validationPassed.value) return "校验未通过，修复错误后才能发布。";
  if (!hasPublishNote.value) return "发布说明不能为空。";
  if (requiresRiskAcknowledgement.value && !riskAcknowledged.value) return "高风险发布必须确认影响范围、审批策略和回滚方案。";
  if (requiresWarningAcknowledgement.value && !warningAcknowledged.value) return "校验警告必须确认后才能发布。";
  if (isAiDraft.value && !aiDraftConfirmed.value) return "AI draft 必须人工确认后才能发布。";
  return "";
});

watch(
  () => props.show,
  (show) => {
    if (!show) {
      publishNote.value = "";
      aiDraftConfirmed.value = false;
      riskAcknowledged.value = false;
      warningAcknowledged.value = false;
      error.value = "";
      loading.value = false;
    }
  },
);

async function publishWorkflow() {
  if (!canPublish.value) return;
  loading.value = true;
  error.value = "";
  try {
    const payload = {
      save_note: publishNote.value.trim(),
      validated_graph_hash: validatedGraphHash.value,
      dry_run_graph_hash: dryRunGraphHash.value,
      diff: props.diffSummary,
      risk_summary: props.riskSummary,
      validation_result: props.validationResult,
      ai_draft_confirmed: aiDraftConfirmed.value,
      risk_acknowledged: requiresRiskAcknowledgement.value ? riskAcknowledged.value : false,
      warning_acknowledged: requiresWarningAcknowledgement.value ? warningAcknowledged.value : false,
    };
    const response = await publishRunnerStudioWorkflow(workflowName.value, payload);
    emit("published", response?.data || response || {});
  } catch (cause) {
    error.value = cause?.message || "发布失败";
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <section v-if="show" class="publish-review-backdrop" data-testid="publish-review-modal">
    <div class="publish-review-modal" role="dialog" aria-modal="true" aria-label="发布审阅">
      <header class="publish-review-head">
        <div>
          <p>PUBLISH REVIEW</p>
          <h2>发布审阅</h2>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </header>

      <main class="publish-review-body">
        <GraphDiffSummary :diff="diffSummary" />

        <section class="publish-review-card">
          <h3>风险摘要</h3>
          <strong>{{ riskSummary.level || "low" }}</strong>
          <p v-if="!riskItems.length">暂无高风险项。</p>
          <ul v-else>
            <li v-for="item in riskItems" :key="item">{{ item }}</li>
          </ul>
        </section>

        <section class="publish-review-card">
          <h3>校验结果</h3>
          <p>{{ validationResult.valid ? "校验通过" : "校验未通过或未提供结果" }}</p>
          <ul v-if="validationErrors.length">
            <li v-for="item in validationErrors" :key="item">{{ item }}</li>
          </ul>
          <ul v-if="validationWarnings.length">
            <li v-for="item in validationWarnings" :key="item">{{ item }}</li>
          </ul>
        </section>

        <label class="publish-note-field">
          <span>发布说明</span>
          <textarea v-model="publishNote" data-testid="publish-note" placeholder="记录变更窗口、审批单或发布原因" />
        </label>

        <label v-if="isAiDraft" class="publish-ai-confirm">
          <input v-model="aiDraftConfirmed" type="checkbox" data-testid="ai-draft-confirmed" />
          <span>我已人工审阅 AI draft、diff 和风险摘要</span>
        </label>

        <label v-if="requiresRiskAcknowledgement" class="publish-ai-confirm">
          <input v-model="riskAcknowledged" type="checkbox" data-testid="publish-risk-acknowledged" />
          <span>我已确认影响范围、目标主机、审批策略和回滚方案</span>
        </label>

        <label v-if="requiresWarningAcknowledgement" class="publish-ai-confirm">
          <input v-model="warningAcknowledged" type="checkbox" data-testid="publish-warning-acknowledged" />
          <span>我已确认校验警告和 Dry Run 风险提示</span>
        </label>

        <p v-if="disabledReason" class="publish-review-warning">{{ disabledReason }}</p>
        <p v-if="error" class="publish-review-error" role="alert">{{ error }}</p>
      </main>

      <footer class="publish-review-footer">
        <button type="button" @click="emit('close')">取消</button>
        <button type="button" class="primary" :disabled="!canPublish" data-testid="publish-confirm" @click="publishWorkflow">
          <RocketIcon :size="15" />
          确认发布
        </button>
      </footer>
    </div>
  </section>
</template>
