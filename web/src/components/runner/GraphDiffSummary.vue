<script setup>
import { computed } from "vue";

const props = defineProps({
  diff: {
    type: Object,
    default: () => ({}),
  },
  semanticChanges: {
    type: Array,
    default: () => [],
  },
  layoutChanges: {
    type: Array,
    default: () => [],
  },
  conflicts: {
    type: Array,
    default: () => [],
  },
});

function asArray(value) {
  if (!value) return [];
  return Array.isArray(value) ? value : [value];
}

function normalizeChange(change) {
  if (typeof change === "string") {
    return { title: change, detail: "" };
  }
  return {
    title: change?.title || change?.label || change?.path || change?.key || change?.type || "变更",
    detail: change?.detail || change?.message || change?.description || change?.summary || "",
  };
}

const semanticItems = computed(() =>
  [
    ...props.semanticChanges,
    ...asArray(props.diff.semantic_changes),
    ...asArray(props.diff.semanticChanges),
    ...asArray(props.diff.semantic),
    ...asArray(props.diff.execution_semantic_changes),
  ].map(normalizeChange),
);

const layoutItems = computed(() =>
  [
    ...props.layoutChanges,
    ...asArray(props.diff.layout_changes),
    ...asArray(props.diff.layoutChanges),
    ...asArray(props.diff.layout),
    ...asArray(props.diff.ui_layout_changes),
  ].map(normalizeChange),
);

const conflictItems = computed(() =>
  [
    ...props.conflicts,
    ...asArray(props.diff.semantic_conflicts),
    ...asArray(props.diff.semanticConflicts),
    ...asArray(props.diff.conflicts),
  ].map(normalizeChange),
);

const hasLinearizeConflict = computed(() => props.diff.linearizable === false || props.diff.can_linearize === false);
</script>

<template>
  <section class="graph-diff-summary" data-testid="graph-diff-summary">
    <section v-if="hasLinearizeConflict || conflictItems.length" class="graph-diff-conflict">
      <h3>语义冲突</h3>
      <p>当前 graph 不可线性化，不允许生成顺序假象。</p>
      <ul v-if="conflictItems.length">
        <li v-for="(item, index) in conflictItems" :key="`conflict-${index}`">
          <strong>{{ item.title }}</strong>
          <span v-if="item.detail">{{ item.detail }}</span>
        </li>
      </ul>
    </section>

    <section>
      <h3>执行语义 diff</h3>
      <ul v-if="semanticItems.length">
        <li v-for="(item, index) in semanticItems" :key="`semantic-${index}`">
          <strong>{{ item.title }}</strong>
          <span v-if="item.detail">{{ item.detail }}</span>
        </li>
      </ul>
      <p v-else>执行语义没有变化。</p>
    </section>

    <section>
      <h3>UI layout diff</h3>
      <ul v-if="layoutItems.length">
        <li v-for="(item, index) in layoutItems" :key="`layout-${index}`">
          <strong>{{ item.title }}</strong>
          <span v-if="item.detail">{{ item.detail }}</span>
        </li>
      </ul>
      <p v-else>画布布局没有变化。</p>
    </section>
  </section>
</template>
