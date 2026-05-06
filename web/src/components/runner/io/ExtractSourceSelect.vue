<script setup>
import { ALLOWED_EXTRACT_SOURCE_TYPES } from "./outputTypes";

defineProps({
  modelValue: {
    type: String,
    default: "stdout_text",
  },
  testId: {
    type: String,
    default: "output-source",
  },
});

const emit = defineEmits(["update:modelValue"]);
</script>

<template>
  <select :value="modelValue" :data-testid="testId" @change="emit('update:modelValue', $event.target.value)">
    <option v-for="type in ALLOWED_EXTRACT_SOURCE_TYPES" :key="type" :value="type">
      {{ type }}
    </option>
  </select>
</template>
