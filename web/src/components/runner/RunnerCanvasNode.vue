<script setup>
import { computed, nextTick, onMounted, watch } from "vue";
import { Handle, Position, useVueFlow } from "@vue-flow/core";

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  data: {
    type: Object,
    default: () => ({}),
  },
  selected: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["open-node-config", "open-menu", "insert-after-port"]);
const { updateNodeInternals } = useVueFlow();

const node = computed(() => props.data?.node || { id: props.id });
const meta = computed(() => props.data?.meta || {});
const ports = computed(() => props.data?.ports || { inputs: [{ id: "in", label: "输入" }], outputs: [{ id: "next", label: "下一步" }] });
const label = computed(() => meta.value.displayLabel || props.data?.label || node.value.label || node.value.step?.name || props.id);
const action = computed(() => meta.value.action || node.value.step?.action || node.value.handler?.action || node.value.type || "node");
const inputPorts = computed(() => ports.value.inputs || []);
const outputPorts = computed(() => ports.value.outputs || []);
const portSignature = computed(() => {
  const inputs = inputPorts.value.map((port) => port.id).join(",");
  const outputs = outputPorts.value.map((port) => port.id).join(",");
  return `${inputs}|${outputs}`;
});

function refreshHandleBounds() {
  nextTick(() => updateNodeInternals?.([props.id]));
}

function portStyle(index, total) {
  const top = ((index + 1) * 100) / (total + 1);
  return { top: `${top}%` };
}

onMounted(refreshHandleBounds);
watch(portSignature, refreshHandleBounds);
</script>

<template>
  <article
    role="button"
    tabindex="0"
    class="runner-canvas-node"
    :class="{ selected }"
    :data-testid="`canvas-node-${id}`"
    @dblclick.stop="emit('open-node-config', id)"
    @keydown.enter.stop="emit('open-node-config', id)"
    @contextmenu.prevent.stop="emit('open-menu', { event: $event, node })"
  >
    <Handle
      v-for="(port, index) in inputPorts"
      :id="port.id"
      :key="`input-${port.id}`"
      type="target"
      :position="Position.Left"
      :connectable="true"
      :connectable-start="false"
      :connectable-end="true"
      class="runner-node-port input"
      :style="portStyle(index, inputPorts.length)"
      :title="port.label"
      :data-testid="`node-input-${id}-${port.id}`"
    />
    <div class="runner-node-card-head">
      <span class="runner-node-icon" :class="`tone-${meta.tone || 'slate'}`">{{ meta.iconText || "RUN" }}</span>
      <div>
        <strong>{{ label }}</strong>
        <small>{{ action }}</small>
      </div>
    </div>
    <p v-if="meta.description" class="runner-node-description">{{ meta.description }}</p>
    <div class="runner-node-meta">
      <span>{{ meta.category || "动作" }}</span>
      <span>{{ inputPorts.length }} in / {{ outputPorts.length }} out</span>
      <span v-if="meta.risk" :class="`risk-${meta.risk}`">{{ meta.risk }}</span>
    </div>
    <Handle
      v-for="(port, index) in outputPorts"
      :id="port.id"
      :key="`output-${port.id}`"
      type="source"
      :position="Position.Right"
      :connectable="true"
      :connectable-start="true"
      :connectable-end="false"
      class="runner-node-port output"
      :style="portStyle(index, outputPorts.length)"
      :title="port.label"
      :data-testid="`node-output-${id}-${port.id}`"
    />
    <button
      v-for="(port, index) in outputPorts"
      :key="`output-add-${port.id}`"
      type="button"
      class="runner-node-port-add"
      :style="portStyle(index, outputPorts.length)"
      :title="`在 ${port.label || port.id} 后添加节点`"
      :data-testid="`node-output-add-${id}-${port.id}`"
      @pointerdown.stop
      @mousedown.stop
      @click.stop="emit('insert-after-port', { nodeId: id, portId: port.id, event: $event })"
    >
      +
    </button>
  </article>
</template>
