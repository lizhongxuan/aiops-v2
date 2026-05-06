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

const draftCondition = ref(createCondition(props.node));

watch(
  () => props.node,
  (node) => {
    draftCondition.value = createCondition(node);
  },
  { immediate: true },
);

function createCondition(node) {
  const condition = node.condition || {};
  return {
    if: condition.if || node.step?.args?.expression || node.step?.when || "",
    elif: Array.isArray(condition.elif) ? condition.elif.map((item) => ({ expression: item.expression || "" })) : [],
    else: condition.else ?? true,
  };
}

function emitCondition() {
  emit("update:node", {
    ...props.node,
    condition: {
      if: draftCondition.value.if,
      elif: draftCondition.value.elif.map((item) => ({ expression: item.expression || "" })),
      else: true,
    },
    step: {
      ...(props.node.step || {}),
      action: props.node.step?.action || "condition.evaluate",
      when: draftCondition.value.if,
      args: {
        ...(props.node.step?.args || {}),
        expression: draftCondition.value.if,
      },
    },
  });
}

function updateIf(value) {
  draftCondition.value.if = value;
  emitCondition();
}

function addElif() {
  draftCondition.value.elif.push({ expression: "" });
  emitCondition();
}

function updateElif(index, value) {
  draftCondition.value.elif[index] = { expression: value };
  emitCondition();
}
</script>

<template>
  <section class="node-panel-form" data-testid="condition-node-panel">
    <section class="node-panel-section">
      <header>
        <strong>条件分支</strong>
        <p>按 IF / ELIF / ELSE 结构配置分支条件。</p>
      </header>
      <label class="node-panel-field">
        <span>IF</span>
        <RunnerVariableTokenInput
          :model-value="draftCondition.if"
          :variables="variables"
          expected-type="boolean"
          input-test-id="condition-if-expression"
          :rows="3"
          placeholder="vars.ready == true"
          @update:model-value="updateIf"
          @locate-node="emit('locate-node', $event)"
        />
      </label>
      <label v-for="(item, index) in draftCondition.elif" :key="index" class="node-panel-field">
        <span>ELIF {{ index + 1 }}</span>
        <RunnerVariableTokenInput
          :model-value="item.expression"
          :variables="variables"
          expected-type="boolean"
          :input-test-id="`condition-elif-expression-${index}`"
          :rows="3"
          placeholder="vars.force == true"
          @update:model-value="updateElif(index, $event)"
          @locate-node="emit('locate-node', $event)"
        />
      </label>
      <button type="button" class="node-panel-secondary" data-testid="condition-add-elif" @click="addElif">
        + ELIF
      </button>
      <p class="node-panel-note">ELSE 分支默认保留，用于 IF 和 ELIF 都不满足时的后续路径。</p>
    </section>
  </section>
</template>
