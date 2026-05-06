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

const draftApproval = ref(createApproval(props.node));

watch(
  () => props.node,
  (node) => {
    draftApproval.value = createApproval(node);
  },
  { immediate: true },
);

function createApproval(node) {
  const approval = node.approval || {};
  const args = node.step?.args || {};
  return {
    subjects: approval.subjects || args.subjects || [],
    timeout: approval.timeout || args.timeout || "30m",
    on_timeout: approval.on_timeout || args.on_timeout || "reject",
    risk_reason: approval.risk_reason || args.risk_reason || "",
  };
}

function splitList(value) {
  return String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function updateApproval(patch) {
  draftApproval.value = {
    ...draftApproval.value,
    ...patch,
  };
  emit("update:node", {
    ...props.node,
    approval: { ...draftApproval.value },
    step: {
      ...(props.node.step || {}),
      action: props.node.step?.action || "manual.approval",
      args: {
        ...(props.node.step?.args || {}),
        ...draftApproval.value,
      },
    },
  });
}
</script>

<template>
  <section class="node-panel-form" data-testid="approval-node-panel">
    <section class="node-panel-section">
      <header>
        <strong>人工审批</strong>
        <p>配置审批人、超时策略和风险说明。</p>
      </header>
      <label class="node-panel-field">
        <span>审批人</span>
        <input
          :value="draftApproval.subjects.join(', ')"
          data-testid="approval-subjects"
          placeholder="oncall, dba"
          @input="updateApproval({ subjects: splitList($event.target.value) })"
        />
      </label>
      <label class="node-panel-field">
        <span>超时</span>
        <input
          :value="draftApproval.timeout"
          data-testid="approval-timeout"
          placeholder="30m"
          @input="updateApproval({ timeout: $event.target.value })"
        />
      </label>
      <label class="node-panel-field">
        <span>超时策略</span>
        <select
          :value="draftApproval.on_timeout"
          data-testid="approval-on-timeout"
          @change="updateApproval({ on_timeout: $event.target.value })"
        >
          <option value="reject">reject</option>
          <option value="approve">approve</option>
          <option value="fail">fail</option>
        </select>
      </label>
      <label class="node-panel-field">
        <span>风险说明</span>
        <RunnerVariableTokenInput
          :model-value="draftApproval.risk_reason"
          :variables="variables"
          input-test-id="approval-risk-reason"
          :rows="4"
          @update:model-value="updateApproval({ risk_reason: $event })"
          @locate-node="emit('locate-node', $event)"
        />
      </label>
    </section>
  </section>
</template>
