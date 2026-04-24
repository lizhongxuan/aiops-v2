<script setup>
import { computed, ref } from "vue";
import { NSpin, NCollapse, NCollapseItem } from "naive-ui";
import {
  BrainIcon,
  ListTodoIcon,
  ShieldCheckIcon,
  MessageSquareIcon,
  PlayIcon,
  GlobeIcon,
  SearchIcon,
  PencilIcon,
  FlaskConicalIcon,
  CheckCircle2Icon,
} from "lucide-vue-next";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const normalizedPhase = computed(() => {
  const phase = (props.card.phase || "").trim();
  const map = {
    thinking: "thinking",
    planning: "planning",
    waiting_approval: "waiting_approval",
    waiting_confirmation: "waiting_confirmation",
    waiting_input: "waiting_input",
    executing: "executing",
    browsing: "browsing",
    searching: "searching",
    editing: "editing",
    testing: "testing",
    finalizing: "finalizing",
  };
  return map[phase] || "thinking";
});

const phaseLabel = computed(() => {
  const map = {
    thinking: "正在思考",
    planning: "正在规划步骤",
    waiting_approval: "等待审批中",
    waiting_confirmation: "等待确认中",
    waiting_input: "等待输入中",
    executing: "执行中",
    browsing: "正在浏览文件",
    searching: "正在搜索内容",
    editing: "正在修改代码",
    testing: "正在验证与测试",
    finalizing: "正在整理结果",
  };
  return map[normalizedPhase.value];
});

const phaseIcon = computed(() => {
  const map = {
    thinking: BrainIcon,
    planning: ListTodoIcon,
    waiting_approval: ShieldCheckIcon,
    waiting_confirmation: ShieldCheckIcon,
    waiting_input: MessageSquareIcon,
    executing: PlayIcon,
    browsing: GlobeIcon,
    searching: SearchIcon,
    editing: PencilIcon,
    testing: FlaskConicalIcon,
    finalizing: CheckCircle2Icon,
  };
  return map[normalizedPhase.value] || BrainIcon;
});

const phaseTone = computed(() => normalizedPhase.value);
const phaseDetail = computed(() => (props.card.hint || props.card.text || "").trim());
const showSpin = computed(() => normalizedPhase.value !== "waiting_approval");
const detailExpanded = ref([]);
</script>

<template>
  <div class="thinking-wrapper" :class="phaseTone">
    <div class="thinking-indicator">
      <span class="thinking-state">
        <n-spin v-if="showSpin" :size="14" />
        <component :is="phaseIcon" v-else size="15" class="thinking-phase-icon" />
        <span class="thinking-text">{{ phaseLabel }}</span>
      </span>
      <n-collapse v-if="phaseDetail" v-model:expanded-names="detailExpanded" class="thinking-collapse">
        <n-collapse-item title="活动详情" name="detail">
          <span class="thinking-detail">{{ phaseDetail }}</span>
        </n-collapse-item>
      </n-collapse>
      <span v-if="phaseDetail && !detailExpanded.length" class="thinking-detail-preview">{{ phaseDetail }}</span>
    </div>
  </div>
</template>

<style scoped>
.thinking-wrapper {
  padding: 2px 0;
  margin-left: 36px;
  animation: fadeInUp 0.2s ease-out;
}

.thinking-indicator {
  display: inline-flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  padding: 7px 12px 8px;
  border-radius: 10px;
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
  border: 1px solid #e2e8f0;
  color: #475569;
  font-size: 12.5px;
  line-height: 1.4;
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.03);
  max-width: min(640px, calc(100vw - 80px));
}

.thinking-phase-icon {
  color: #64748b;
  flex-shrink: 0;
}

.thinking-text {
  font-weight: 500;
  color: #0f172a;
  font-size: 13px;
}

.thinking-state {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.thinking-collapse {
  width: 100%;
}

.thinking-detail {
  color: #64748b;
  font-size: 11.5px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-word;
}

.thinking-detail-preview {
  color: #94a3b8;
  font-size: 11px;
  line-height: 1.4;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}

@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(6px); }
  to   { opacity: 1; transform: translateY(0); }
}

@media (max-width: 640px) {
  .thinking-wrapper {
    margin-left: 0;
  }

  .thinking-indicator {
    max-width: none;
  }
}
</style>
