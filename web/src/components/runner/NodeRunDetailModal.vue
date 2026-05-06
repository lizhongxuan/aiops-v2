<script setup>
import { computed } from "vue";
import { XIcon } from "lucide-vue-next";
import "./runnerStudio.css";

const props = defineProps({
  show: {
    type: Boolean,
    default: false,
  },
  nodeId: {
    type: String,
    default: "",
  },
  state: {
    type: Object,
    default: () => ({ nodes: {}, logs: [], variables: { nodeResults: [] } }),
  },
});

const emit = defineEmits(["close"]);

const node = computed(() => props.state.nodes?.[props.nodeId] || null);
const logs = computed(() => (props.state.logs || []).filter((log) => log.nodeId === props.nodeId));
const result = computed(() => node.value?.result || props.state.variables?.nodeResults?.find((item) => item.nodeId === props.nodeId)?.result || null);
</script>

<template>
  <section v-if="show && node" class="node-run-detail-backdrop">
    <div class="node-run-detail-modal" role="dialog" aria-modal="true" data-testid="node-run-detail-modal">
      <header class="node-config-head">
        <div>
          <p>NODE RUN DETAIL</p>
          <h2>{{ node.nodeId }}</h2>
        </div>
        <button type="button" class="workflow-icon-button" aria-label="关闭" @click="emit('close')">
          <XIcon :size="16" />
        </button>
      </header>
      <main>
        <section>
          <h3>完整结果</h3>
          <pre>{{ JSON.stringify(result, null, 2) }}</pre>
        </section>
        <section>
          <h3>节点日志</h3>
          <pre v-for="(log, index) in logs" :key="`log-${index}`">{{ log.stream }} {{ log.hostId }} {{ log.message }}</pre>
        </section>
      </main>
    </div>
  </section>
</template>
