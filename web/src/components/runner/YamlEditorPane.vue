<script setup>
defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  disabled: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["update:modelValue"]);
</script>

<template>
  <label class="yaml-editor-pane">
    <span>YAML</span>
    <textarea
      :value="modelValue"
      :disabled="disabled"
      data-testid="yaml-editor"
      spellcheck="false"
      @input="emit('update:modelValue', $event.target.value)"
    />
  </label>
</template>
