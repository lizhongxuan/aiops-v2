<script setup>
import { ref, watch } from "vue";

const props = defineProps({
  node: {
    type: Object,
    required: true,
  },
});

const emit = defineEmits(["update:node"]);

const draftStep = ref(createStep(props.node));

watch(
  () => props.node,
  (node) => {
    draftStep.value = createStep(node);
  },
  { immediate: true },
);

function createStep(node) {
  return {
    ...(node.step || {}),
    args: { ...(node.step?.args || {}) },
  };
}

function splitList(value) {
  return String(value || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function updateStep(patch, argsPatch = {}) {
  draftStep.value = {
    ...draftStep.value,
    ...patch,
    args: {
      ...(draftStep.value.args || {}),
      ...argsPatch,
    },
  };
  emit("update:node", {
    ...props.node,
    step: { ...draftStep.value, args: { ...(draftStep.value.args || {}) } },
  });
}
</script>

<template>
  <section class="node-config-advanced" data-testid="advanced-tab">
    <section class="node-panel-section">
      <header>
        <strong>通用执行控制</strong>
        <p>这些设置会直接写入当前节点 step，不需要手写 YAML。</p>
      </header>
      <div class="node-config-form">
        <label>
          <span>when</span>
          <input
            :value="draftStep.when || ''"
            data-testid="advanced-when"
            placeholder="vars.ready == true"
            @input="updateStep({ when: $event.target.value })"
          />
        </label>
        <label>
          <span>targets</span>
          <input
            :value="Array.isArray(draftStep.targets) ? draftStep.targets.join(', ') : ''"
            data-testid="advanced-targets"
            placeholder="pg-01, pg-02"
            @input="updateStep({ targets: splitList($event.target.value) })"
          />
        </label>
        <label>
          <span>timeout</span>
          <input
            :value="draftStep.timeout || ''"
            data-testid="advanced-timeout"
            placeholder="30s / 15m"
            @input="updateStep({ timeout: $event.target.value })"
          />
        </label>
        <label>
          <span>retries</span>
          <input
            type="number"
            min="0"
            :value="draftStep.retries ?? 0"
            data-testid="advanced-retries"
            @input="updateStep({ retries: Number($event.target.value || 0) })"
          />
        </label>
        <label class="node-config-checkbox">
          <span>continue_on_error</span>
          <input
            type="checkbox"
            :checked="Boolean(draftStep.continue_on_error)"
            data-testid="advanced-continue-on-error"
            @change="updateStep({ continue_on_error: $event.target.checked })"
          />
        </label>
        <label>
          <span>rollback</span>
          <input
            :value="draftStep.args?.rollback || ''"
            data-testid="advanced-rollback"
            placeholder="rollback-step-name"
            @input="updateStep({}, { rollback: $event.target.value })"
          />
        </label>
        <label>
          <span>secrets</span>
          <input
            :value="Array.isArray(draftStep.args?.secrets) ? draftStep.args.secrets.join(', ') : ''"
            data-testid="advanced-secrets"
            placeholder="PGPASSWORD, SSH_KEY"
            @input="updateStep({}, { secrets: splitList($event.target.value) })"
          />
        </label>
      </div>
      <p class="node-panel-note">join/loop/subflow 的详细结构会在对应节点类型面板中配置。</p>
    </section>
  </section>
</template>
