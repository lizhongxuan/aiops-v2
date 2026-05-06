<script setup>
import InputParamList from "../io/InputParamList.vue";

defineProps({
  node: {
    type: Object,
    required: true,
  },
  variables: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:node"]);

function updateInputs(node, inputs) {
  emit("update:node", {
    ...node,
    inputs,
  });
}
</script>

<template>
  <section data-testid="input-tab">
    <InputParamList
      :params="node.inputs || []"
      :variables="variables"
      @update:params="updateInputs(node, $event)"
    />
  </section>
</template>
