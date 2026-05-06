<script setup>
import { computed } from "vue";
import { variablePath } from "./ioTypes";

const props = defineProps({
  modelValue: {
    type: Object,
    default: null,
  },
  variables: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:modelValue"]);
const serverVariables = computed(() => props.variables.filter((variable) => variablePath(variable)));

function choose(variable) {
  emit("update:modelValue", { ...variable, path: variablePath(variable) });
}
</script>

<template>
  <section class="variable-reference-picker">
    <button
      v-for="variable in serverVariables"
      :key="variablePath(variable)"
      type="button"
      :class="{ active: variablePath(modelValue || {}) === variablePath(variable) }"
      :data-testid="`variable-option-${variablePath(variable)}`"
      @click="choose(variable)"
    >
      <strong>{{ variablePath(variable) }}</strong>
      <small>{{ variable.scope }}</small>
    </button>
  </section>
</template>
