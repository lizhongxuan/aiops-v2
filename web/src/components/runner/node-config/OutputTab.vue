<script setup>
import OutputParamList from "../io/OutputParamList.vue";

defineProps({
  node: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["update:node"]);

function updateOutputs(node, outputs) {
  emit("update:node", {
    ...node,
    outputs,
  });
}
</script>

<template>
  <section data-testid="output-tab">
    <OutputParamList :outputs="node.outputs || []" @update:outputs="updateOutputs(node, $event)" />
  </section>
</template>
