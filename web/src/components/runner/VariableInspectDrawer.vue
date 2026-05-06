<script setup>
import { computed } from "vue";
import "./runnerStudio.css";

const props = defineProps({
  state: {
    type: Object,
    default: () => ({ variables: { inputs: [], outputs: [], exports: [], nodeResults: [] } }),
  },
  selectedNodeId: {
    type: String,
    default: "",
  },
});

const variables = computed(() => props.state.variables || {});
const inputVars = computed(() => variables.value.inputs || []);
const outputVars = computed(() => variables.value.outputs || []);
const exportVars = computed(() => variables.value.exports || []);
const nodeResults = computed(() =>
  (variables.value.nodeResults || []).filter((item) => !props.selectedNodeId || item.nodeId === props.selectedNodeId),
);

function displayValue(value) {
  if (typeof value === "string") return value;
  return JSON.stringify(value);
}
</script>

<template>
  <section class="variable-inspect-drawer" data-testid="variable-inspect-drawer">
    <header>
      <strong>变量检查</strong>
      <span>输入、输出、运行态导出变量和最近节点结果</span>
    </header>

    <div class="runner-drawer-grid">
      <section>
        <h3>输入变量</h3>
        <p v-if="!inputVars.length">暂无输入变量。</p>
        <pre v-for="item in inputVars" :key="`${item.nodeId}-${item.key}`">{{ item.key }}={{ displayValue(item.value) }}</pre>
      </section>
      <section>
        <h3>输出变量</h3>
        <p v-if="!outputVars.length">暂无输出变量。</p>
        <pre v-for="item in outputVars" :key="`${item.nodeId}-${item.key}`">{{ item.key }}={{ displayValue(item.value) }}</pre>
      </section>
      <section>
        <h3>运行态导出变量</h3>
        <p v-if="!exportVars.length">暂无导出变量。</p>
        <pre v-for="item in exportVars" :key="`${item.nodeId || 'run'}-${item.key}`">{{ item.key }}={{ displayValue(item.value) }}</pre>
      </section>
      <section>
        <h3>最近节点结果</h3>
        <p v-if="!nodeResults.length">暂无节点结果。</p>
        <pre v-for="item in nodeResults" :key="`${item.nodeId}-${JSON.stringify(item.result)}`">{{ item.nodeId }} {{ displayValue(item.result) }}</pre>
      </section>
    </div>
  </section>
</template>
