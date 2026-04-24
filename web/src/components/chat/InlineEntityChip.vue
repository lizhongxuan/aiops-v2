<script setup>
defineProps({
  kind: {
    type: String,
    default: "value",
  },
  text: {
    type: String,
    default: "",
  },
});
</script>

<template>
  <span class="inline-entity-chip" :data-entity-kind="kind">{{ text }}</span>
</template>

<style scoped>
.inline-entity-chip {
  display: inline-flex;
  align-items: center;
  margin: 0 1px;
  padding: 0 7px;
  min-height: 1.45em;
  border-radius: 999px;
  background: rgba(241, 245, 249, 0.9);
  color: #334155;
  font-size: 0.95em;
  line-height: 1.35;
  vertical-align: baseline;
}

.inline-entity-chip[data-entity-kind="percent"] {
  background: rgba(226, 232, 240, 0.95);
}

.inline-entity-chip[data-entity-kind="symbol"] {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  letter-spacing: 0.01em;
}
</style>
