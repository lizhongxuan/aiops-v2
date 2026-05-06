<script setup>
import { ref, watch } from "vue";
import RunnerVariableTokenInput from "../RunnerVariableTokenInput.vue";

const props = defineProps({
  node: {
    type: Object,
    required: true,
  },
  variables: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:node", "locate-node"]);

const draftArgs = ref({ ...(props.node.step?.args || {}) });

watch(
  () => props.node,
  (node) => {
    draftArgs.value = { ...(node.step?.args || {}) };
  },
  { immediate: true },
);

function splitList(value) {
  return String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function updateArgs(patch) {
  draftArgs.value = {
    ...draftArgs.value,
    ...patch,
  };
  emit("update:node", {
    ...props.node,
    step: {
      ...(props.node.step || {}),
      action: props.node.step?.action || "notify.send",
      args: { ...draftArgs.value },
    },
  });
}
</script>

<template>
  <section class="node-panel-form" data-testid="notify-node-panel">
    <section class="node-panel-section">
      <header>
        <strong>通知</strong>
        <p>配置通知渠道、接收人、模板和失败策略。</p>
      </header>
      <label class="node-panel-field">
        <span>渠道</span>
        <select
          :value="draftArgs.channel || 'slack'"
          data-testid="notify-channel"
          @change="updateArgs({ channel: $event.target.value })"
        >
          <option value="slack">slack</option>
          <option value="email">email</option>
          <option value="webhook">webhook</option>
          <option value="pagerduty">pagerduty</option>
        </select>
      </label>
      <label class="node-panel-field">
        <span>接收人</span>
        <input
          :value="Array.isArray(draftArgs.recipients) ? draftArgs.recipients.join(', ') : ''"
          data-testid="notify-recipients"
          placeholder="sre, dba"
          @input="updateArgs({ recipients: splitList($event.target.value) })"
        />
      </label>
      <label class="node-panel-field">
        <span>模板</span>
        <RunnerVariableTokenInput
          :model-value="draftArgs.template || ''"
          :variables="variables"
          input-test-id="notify-template"
          :rows="5"
          placeholder="恢复失败: ${node.restore.stderr}"
          @update:model-value="updateArgs({ template: $event })"
          @locate-node="emit('locate-node', $event)"
        />
      </label>
      <label class="node-panel-field">
        <span>失败策略</span>
        <select
          :value="draftArgs.on_failure || 'fail'"
          data-testid="notify-on-failure"
          @change="updateArgs({ on_failure: $event.target.value })"
        >
          <option value="fail">fail</option>
          <option value="continue">continue</option>
          <option value="retry">retry</option>
        </select>
      </label>
    </section>
  </section>
</template>
