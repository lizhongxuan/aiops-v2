<script setup>
import { computed } from "vue";
import { FocusIcon, GitBranchIcon, Trash2Icon, XIcon } from "lucide-vue-next";

const props = defineProps({
  edge: {
    type: Object,
    default: () => ({}),
  },
  x: {
    type: Number,
    default: 0,
  },
  y: {
    type: Number,
    default: 0,
  },
});

const emit = defineEmits(["action", "close"]);
const style = computed(() => ({
  left: `${props.x}px`,
  top: `${props.y}px`,
}));

function choose(action) {
  emit("action", action, props.edge?.id);
  emit("close");
}
</script>

<template>
  <div class="runner-edge-menu" :style="style" data-testid="runner-edge-menu">
    <header>
      <strong>连线操作</strong>
      <div class="runner-edge-menu-head-actions">
        <span>{{ edge.kind || edge.source_port || "next" }}</span>
        <button
          type="button"
          class="runner-edge-menu-close"
          data-testid="runner-edge-menu-close"
          aria-label="关闭连线菜单"
          title="关闭"
          @click="emit('close')"
        >
          <XIcon :size="14" />
        </button>
      </div>
    </header>
    <button type="button" data-testid="edge-action-kind-next" @click="choose('set-kind:next')">
      <GitBranchIcon :size="14" />
      设为 NEXT
    </button>
    <button type="button" data-testid="edge-action-kind-failure" @click="choose('set-kind:failure')">
      <GitBranchIcon :size="14" />
      设为 FAILURE
    </button>
    <button type="button" data-testid="edge-action-focus-source" @click="choose('focus-source')">
      <FocusIcon :size="14" />
      定位上游
    </button>
    <button type="button" data-testid="edge-action-focus-target" @click="choose('focus-target')">
      <FocusIcon :size="14" />
      定位下游
    </button>
    <button type="button" data-testid="edge-action-delete" class="danger" @click="choose('delete')">
      <Trash2Icon :size="14" />
      删除连线
    </button>
  </div>
</template>
