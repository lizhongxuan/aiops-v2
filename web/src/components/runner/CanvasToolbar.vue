<script setup>
import { ref } from "vue";
import { HandIcon, MaximizeIcon, MoreHorizontalIcon, PlusIcon } from "lucide-vue-next";
import NodePicker from "./NodePicker.vue";

const props = defineProps({
  actions: {
    type: Array,
    default: () => [],
  },
  fullscreen: {
    type: Boolean,
    default: false,
  },
  recentActionKeys: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["add-action", "toggle-fullscreen"]);
const pickerOpen = ref(false);

function addAction(action) {
  emit("add-action", action);
  pickerOpen.value = false;
}
</script>

<template>
  <aside class="runner-canvas-toolbar" aria-label="节点动作库">
    <div class="runner-canvas-tool-rail">
      <button
        type="button"
        class="runner-canvas-tool-button primary"
        :class="{ active: pickerOpen }"
        data-testid="runner-node-picker-toggle"
        title="添加节点"
        @click="pickerOpen = !pickerOpen"
      >
        <PlusIcon :size="17" />
        <span>添加节点</span>
      </button>
      <button type="button" class="runner-canvas-tool-button" title="选择/拖动画布">
        <HandIcon :size="17" />
      </button>
      <button
        type="button"
        class="runner-canvas-tool-button"
        :class="{ active: fullscreen }"
        data-testid="runner-canvas-fullscreen-toggle"
        :title="fullscreen ? '退出全屏' : '全屏编排'"
        @click="emit('toggle-fullscreen')"
      >
        <MaximizeIcon :size="17" />
      </button>
      <button type="button" class="runner-canvas-tool-button" title="更多">
        <MoreHorizontalIcon :size="17" />
      </button>
    </div>
    <NodePicker
      v-if="pickerOpen"
      :actions="actions"
      :recent-action-keys="recentActionKeys"
      @select="addAction"
      @close="pickerOpen = false"
    />
  </aside>
</template>
