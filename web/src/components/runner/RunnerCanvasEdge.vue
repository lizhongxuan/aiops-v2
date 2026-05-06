<script setup>
import { computed } from "vue";
import { BaseEdge, getBezierPath } from "@vue-flow/core";

defineOptions({ inheritAttrs: false });

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  sourceX: {
    type: Number,
    required: true,
  },
  sourceY: {
    type: Number,
    required: true,
  },
  targetX: {
    type: Number,
    required: true,
  },
  targetY: {
    type: Number,
    required: true,
  },
  sourcePosition: {
    type: String,
    default: "right",
  },
  targetPosition: {
    type: String,
    default: "left",
  },
  markerEnd: {
    type: String,
    default: "",
  },
  selected: {
    type: Boolean,
    default: false,
  },
  data: {
    type: Object,
    default: () => ({}),
  },
});

const emit = defineEmits(["open-menu", "edge-hover"]);

const pathResult = computed(() =>
  getBezierPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    sourcePosition: props.sourcePosition,
    targetX: props.targetX,
    targetY: props.targetY,
    targetPosition: props.targetPosition,
  }),
);
const edgePath = computed(() => pathResult.value[0]);
const edge = computed(() => props.data?.edge || { id: props.id });

function openMenu(event) {
  event?.preventDefault?.();
  event?.stopPropagation?.();
  emit("open-menu", {
    edge: edge.value,
    x: event.clientX || 0,
    y: event.clientY || 0,
  });
}

function setHover(hovering) {
  emit("edge-hover", { id: props.id, hovering });
}
</script>

<template>
  <g
    :data-testid="`runner-edge-${id}`"
    class="runner-canvas-edge"
    :class="{ selected }"
    @pointerenter="setHover(true)"
    @pointerleave="setHover(false)"
    @contextmenu.prevent="openMenu"
  >
    <BaseEdge :id="id" class="runner-edge-path" :path="edgePath" :marker-end="markerEnd" />
    <path
      class="runner-edge-hit-path"
      :d="edgePath"
      fill="none"
      :data-testid="`runner-edge-hit-${id}`"
      @click.stop="openMenu"
      @contextmenu.prevent="openMenu"
    />
  </g>
</template>
