<script setup>
import { computed } from "vue";

const props = defineProps({
  sourceX: {
    type: Number,
    default: 0,
  },
  sourceY: {
    type: Number,
    default: 0,
  },
  targetX: {
    type: Number,
    default: 0,
  },
  targetY: {
    type: Number,
    default: 0,
  },
  connectionStatus: {
    type: String,
    default: "",
  },
});

const edgePath = computed(() => {
  const distance = Math.max(Math.abs(props.targetX - props.sourceX) * 0.5, 80);
  const sourceControlX = props.sourceX + distance;
  const targetControlX = props.targetX - distance;
  return `M${props.sourceX},${props.sourceY} C${sourceControlX},${props.sourceY} ${targetControlX},${props.targetY} ${props.targetX},${props.targetY}`;
});
</script>

<template>
  <path
    class="runner-connection-line"
    :class="connectionStatus"
    :d="edgePath"
    fill="none"
    data-testid="runner-connection-line"
  />
</template>
