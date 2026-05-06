<script setup>
import { ALLOWED_VALUE_SOURCE_TYPES } from "./ioTypes";

defineProps({
  modelValue: {
    type: String,
    default: "constant",
  },
  testId: {
    type: String,
    default: "input-source",
  },
});

const emit = defineEmits(["update:modelValue"]);

const labels = {
  constant: "常量",
  variable_reference: "变量",
  expression: "表达式",
};
</script>

<template>
  <select :value="modelValue" :data-testid="testId" @change="emit('update:modelValue', $event.target.value)">
    <option v-for="type in ALLOWED_VALUE_SOURCE_TYPES" :key="type" :value="type">
      {{ labels[type] }}
    </option>
  </select>
</template>
