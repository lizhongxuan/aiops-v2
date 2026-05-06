<script setup>
import { computed, ref, watch } from "vue";
import { RefreshCwIcon, XIcon } from "lucide-vue-next";
import GraphDiffSummary from "./GraphDiffSummary.vue";
import YamlEditorPane from "./YamlEditorPane.vue";
import {
  compileRunnerStudioWorkflowGraph,
  parseRunnerStudioWorkflowYaml,
} from "../../api/runnerStudioClient";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  graph: {
    type: Object,
    default: () => ({ version: "v1", workflow: {}, nodes: [], edges: [] }),
  },
});

const emit = defineEmits(["close", "apply-graph"]);

const yaml = ref("");
const diff = ref({});
const error = ref("");
const loading = ref(false);

const canApply = computed(() => props.show && yaml.value.trim() && !loading.value);

function unwrapPayload(payload) {
  return payload?.data || payload || {};
}

async function refreshYaml() {
  if (!props.show) return;
  loading.value = true;
  error.value = "";
  try {
    const payload = unwrapPayload(await compileRunnerStudioWorkflowGraph({ graph: props.graph }));
    yaml.value = payload.yaml || payload.compiled_yaml || payload.workflow_yaml || "";
    diff.value = payload.diff || payload.graph_diff || {};
  } catch (cause) {
    error.value = cause?.message || "Graph 转 YAML 失败";
  } finally {
    loading.value = false;
  }
}

async function parseAndApply() {
  if (!canApply.value) return;
  loading.value = true;
  error.value = "";
  try {
    const payload = unwrapPayload(await parseRunnerStudioWorkflowYaml({ yaml: yaml.value }));
    diff.value = payload.diff || payload.graph_diff || {};
    emit("apply-graph", payload.graph || payload);
  } catch (cause) {
    error.value = cause?.message || "YAML 解析失败";
  } finally {
    loading.value = false;
  }
}

watch(
  () => [props.show, props.graph],
  ([show]) => {
    if (show) {
      refreshYaml();
    }
  },
  { immediate: true },
);
</script>

<template>
  <section v-if="show" class="yaml-diff-backdrop" data-testid="yaml-diff-modal">
    <div class="yaml-diff-modal" role="dialog" aria-modal="true" aria-label="YAML 与 Diff">
      <header class="yaml-diff-head">
        <div>
          <p>YAML COMPAT VIEW</p>
          <h2>YAML 与 Diff</h2>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </header>

      <main class="yaml-diff-body">
        <YamlEditorPane v-model="yaml" :disabled="loading" />
        <GraphDiffSummary :diff="diff" />
      </main>

      <p v-if="error" class="yaml-diff-error" role="alert">{{ error }}</p>

      <footer class="yaml-diff-footer">
        <button type="button" :disabled="loading" @click="refreshYaml">
          <RefreshCwIcon :size="15" />
          刷新 YAML
        </button>
        <button type="button" :disabled="!canApply" class="primary" data-testid="yaml-parse-apply" @click="parseAndApply">
          解析并应用
        </button>
      </footer>
    </div>
  </section>
</template>
