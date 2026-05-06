<script setup>
import { computed, ref, watch } from "vue";
import { SparklesIcon, XIcon } from "lucide-vue-next";
import { validateRunnerStudioWorkflowGraph } from "../../../api/runnerStudioClient";
import AiDiffPreview from "./AiDiffPreview.vue";
import { draftRunnerWorkflowWithAI } from "./aiRunnerApi";
import "../runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  workflow: {
    type: Object,
    default: () => ({ name: "", status: "draft" }),
  },
  graph: {
    type: Object,
    default: () => ({ version: "v1", workflow: {}, nodes: [], edges: [] }),
  },
});

const emit = defineEmits(["close", "apply-patch"]);

const instruction = ref("");
const result = ref(null);
const error = ref("");
const loading = ref(false);
const validating = ref(false);

const workflowStatus = computed(() => String(props.workflow?.status || "draft").toLowerCase());
const isDraft = computed(() => workflowStatus.value === "draft");
const graphPatch = computed(() => result.value?.graph_patch || null);
const candidateGraph = computed(() => result.value?.candidate_graph || graphPatch.value?.graph || result.value?.graph || null);
const diffSummary = computed(() => result.value?.diff_summary || result.value?.diffSummary || {});
const canApply = computed(() => Boolean(graphPatch.value && candidateGraph.value && !loading.value && !validating.value));

watch(
  () => props.show,
  (show) => {
    if (!show) {
      instruction.value = "";
      result.value = null;
      error.value = "";
      loading.value = false;
      validating.value = false;
    }
  },
);

async function generateDraft() {
  error.value = "";
  result.value = null;
  if (!isDraft.value) {
    error.value = "只能在 draft 工作流中使用 AI patch。";
    return;
  }
  if (!instruction.value.trim()) {
    error.value = "请输入希望 AI 生成或修改的工作流目标。";
    return;
  }
  loading.value = true;
  try {
    const payload = await draftRunnerWorkflowWithAI({
      workflow_name: props.workflow?.name || props.graph?.workflow?.name || "",
      workflow_status: workflowStatus.value,
      instruction: instruction.value.trim(),
      graph: props.graph,
    });
    result.value = payload?.data || payload || {};
    if (result.value.error_explanation) {
      error.value = result.value.error_explanation;
    }
  } catch (cause) {
    error.value = cause?.message || "AI 生成失败";
  } finally {
    loading.value = false;
  }
}

async function applyPatch() {
  if (!canApply.value) return;
  validating.value = true;
  error.value = "";
  try {
    const validation = await validateRunnerStudioWorkflowGraph({ graph: candidateGraph.value });
    const validationResult = validation?.data || validation || {};
    if (validationResult.valid === false) {
      const firstError = Array.isArray(validationResult.errors) ? validationResult.errors[0] : null;
      error.value =
        (typeof firstError === "string" ? firstError : firstError?.message) || "AI patch 校验失败";
      return;
    }
    emit("apply-patch", {
      graph_patch: graphPatch.value,
      graph: candidateGraph.value,
      diff_summary: diffSummary.value,
      validation: validationResult,
    });
  } catch (cause) {
    error.value = cause?.message || "AI patch 校验失败";
  } finally {
    validating.value = false;
  }
}
</script>

<template>
  <section v-if="show" class="runner-ai-backdrop" data-testid="runner-ai-modal">
    <div class="runner-ai-modal" role="dialog" aria-modal="true" aria-label="Runner AI 助手">
      <header class="runner-ai-head">
        <div>
          <p>RUNNER AI</p>
          <h2>AI 生成工作流草稿</h2>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </header>

      <main class="runner-ai-body">
        <label class="runner-ai-instruction">
          <span>目标</span>
          <textarea
            v-model="instruction"
            data-testid="runner-ai-instruction"
            :disabled="loading || validating"
            placeholder="描述要生成或调整的 Runner 工作流"
          />
        </label>

        <p v-if="error" class="runner-ai-error" role="alert">{{ error }}</p>

        <AiDiffPreview v-if="graphPatch" :graph-patch="graphPatch" :diff-summary="diffSummary" />
      </main>

      <footer class="runner-ai-footer">
        <button type="button" :disabled="loading || validating" data-testid="runner-ai-generate" @click="generateDraft">
          <SparklesIcon :size="15" />
          生成 draft
        </button>
        <button type="button" class="primary" :disabled="!canApply" data-testid="runner-ai-apply" @click="applyPatch">
          应用并校验
        </button>
      </footer>
    </div>
  </section>
</template>
