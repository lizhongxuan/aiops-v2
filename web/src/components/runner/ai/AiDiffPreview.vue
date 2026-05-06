<script setup>
import { computed } from "vue";
import GraphDiffSummary from "../GraphDiffSummary.vue";
import "../runnerStudio.css";

const props = defineProps({
  graphPatch: {
    type: Object,
    default: null,
  },
  diffSummary: {
    type: Object,
    default: () => ({}),
  },
});

const operations = computed(() => props.graphPatch?.operations || props.graphPatch?.ops || []);
</script>

<template>
  <section class="ai-diff-preview" data-testid="ai-diff-preview">
    <GraphDiffSummary :diff="diffSummary" />
    <section class="ai-patch-ops">
      <h3>AI graph patch</h3>
      <p v-if="!operations.length">暂无 patch 操作。</p>
      <pre v-for="(operation, index) in operations" :key="`op-${index}`">{{ JSON.stringify(operation, null, 2) }}</pre>
    </section>
  </section>
</template>
