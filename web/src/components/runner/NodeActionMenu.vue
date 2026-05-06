<script setup>
import {
  BanIcon,
  BotIcon,
  ClockIcon,
  CopyIcon,
  PlayIcon,
  Trash2Icon,
} from "lucide-vue-next";

const props = defineProps({
  node: {
    type: Object,
    required: true,
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

const actions = [
  { key: "copy", label: "复制", icon: CopyIcon },
  { key: "delete", label: "删除", icon: Trash2Icon },
  { key: "disable", label: "禁用", icon: BanIcon },
  { key: "run-node", label: "单节点试跑", icon: PlayIcon },
  { key: "recent-runs", label: "最近运行", icon: ClockIcon },
  { key: "ai-fix", label: "AI 修复", icon: BotIcon },
];

function choose(action) {
  emit("action", action.key, props.node.id);
  emit("close");
}
</script>

<template>
  <section
    class="node-action-menu"
    data-testid="node-action-menu"
    :style="{ left: `${x}px`, top: `${y}px` }"
    role="menu"
  >
    <button
      v-for="action in actions"
      :key="action.key"
      type="button"
      :data-testid="`node-action-${action.key}`"
      @click="choose(action)"
    >
      <component :is="action.icon" :size="14" />
      <span>{{ action.label }}</span>
    </button>
  </section>
</template>
