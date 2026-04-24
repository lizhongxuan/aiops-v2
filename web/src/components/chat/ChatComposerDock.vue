<script setup>
import { ref } from "vue";
import PlanCard from "../PlanCard.vue";
import Omnibar from "../Omnibar.vue";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  placeholder: {
    type: String,
    default: "",
  },
  disabled: {
    type: Boolean,
    default: false,
  },
  allowFollowUp: {
    type: Boolean,
    default: false,
  },
  planCard: {
    type: Object,
    default: null,
  },
  sessionKind: {
    type: String,
    default: "",
  },
  statusHint: {
    type: String,
    default: "",
  },
  showComposer: {
    type: Boolean,
    default: true,
  },
  isDockedBottom: {
    type: Boolean,
    default: false,
  },
  busy: {
    type: Boolean,
    default: false,
  },
  primaryActionOverride: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["update:modelValue", "send", "stop"]);
const omnibarRef = ref(null);

defineExpose({
  focus() {
    omnibarRef.value?.focus?.();
  },
});
</script>

<template>
  <footer class="chat-composer-dock">
    <div class="chat-composer-stack">
      <slot name="terminal" />

      <div v-if="planCard" class="chat-composer-plan">
        <PlanCard :card="planCard" :session-kind="sessionKind" compact />
      </div>

      <div v-if="statusHint" class="chat-composer-hint">
        {{ statusHint }}
      </div>

      <slot name="approval" />

      <Omnibar
        ref="omnibarRef"
        v-if="showComposer"
        :model-value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :allow-follow-up="allowFollowUp"
        :busy="busy"
        :primary-action-override="primaryActionOverride"
        :is-docked-bottom="isDockedBottom"
        @update:model-value="emit('update:modelValue', $event)"
        @send="emit('send')"
        @stop="emit('stop')"
      />
    </div>
  </footer>
</template>

<style scoped>
.chat-composer-dock {
  position: sticky;
  bottom: 0;
  z-index: 6;
  width: 100%;
  padding: 0 0 16px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0), rgba(248, 250, 252, 0.92) 28%, #f8fafc 62%);
}

.chat-composer-stack {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
  max-width: 820px;
  margin: 0 auto;
}

.chat-composer-plan {
  position: relative;
  z-index: 2;
}

.chat-composer-hint {
  padding: 9px 12px;
  border-radius: 12px;
  background: rgba(239, 246, 255, 0.92);
  border: 1px solid rgba(147, 197, 253, 0.45);
  color: #1d4ed8;
  font-size: 12px;
  line-height: 1.5;
}
</style>
