<script setup>
import { computed, ref, watch } from "vue";
import { validateJsonPath } from "./outputTypes";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  testId: {
    type: String,
    default: "jsonpath-editor",
  },
});

const emit = defineEmits(["update:modelValue"]);
const draft = ref("");
const error = computed(() => validateJsonPath(draft.value));

watch(
  () => props.modelValue,
  (value) => {
    draft.value = value || "";
  },
  { immediate: true },
);

function update(value) {
  draft.value = value;
  emit("update:modelValue", value);
}
</script>

<template>
  <label class="jsonpath-editor">
    <span>extract_rule</span>
    <input :value="draft" :data-testid="testId" @input="update($event.target.value)" />
    <small v-if="error">{{ error }}</small>
  </label>
</template>
