<script setup>
import { computed } from "vue";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["update:modelValue"]);

const highlightedSegments = computed(() => {
  const text = props.modelValue || "";
  const segments = [];
  const pattern = /\$\{([^}]+)\}/g;
  let lastIndex = 0;
  let match;
  while ((match = pattern.exec(text))) {
    if (match.index > lastIndex) {
      segments.push({ type: "text", value: text.slice(lastIndex, match.index) });
    }
    segments.push({ type: "variable", value: match[1] });
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < text.length) segments.push({ type: "text", value: text.slice(lastIndex) });
  return segments;
});
</script>

<template>
  <section class="mixed-variable-text">
    <textarea
      :value="modelValue"
      data-testid="mixed-variable-input"
      @input="emit('update:modelValue', $event.target.value)"
    />
    <p class="mixed-variable-preview" data-testid="mixed-variable-preview">
      <template v-for="(segment, index) in highlightedSegments" :key="index">
        <mark v-if="segment.type === 'variable'" class="mixed-variable-token">{{ segment.value }}</mark>
        <span v-else>{{ segment.value }}</span>
      </template>
    </p>
  </section>
</template>
