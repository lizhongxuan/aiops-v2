<script setup>
import { computed, ref, watch } from "vue";
import RunnerVariableSelector from "./RunnerVariableSelector.vue";

const props = defineProps({
  modelValue: {
    type: String,
    default: "",
  },
  variables: {
    type: Array,
    default: () => [],
  },
  expectedType: {
    type: String,
    default: "",
  },
  inputTestId: {
    type: String,
    default: "runner-variable-token-input",
  },
  placeholder: {
    type: String,
    default: "",
  },
  rows: {
    type: Number,
    default: 4,
  },
});

const emit = defineEmits(["update:modelValue", "locate-node"]);
const draftValue = ref(props.modelValue || "");

watch(
  () => props.modelValue,
  (value) => {
    draftValue.value = value || "";
  },
);

const variableByExpression = computed(() => {
  const out = new Map();
  for (const variable of props.variables || []) {
    const expression = expressionOf(variable);
    if (expression) out.set(expression, variable);
  }
  return out;
});

const previewSegments = computed(() => splitVariableTokens(draftValue.value, variableByExpression.value));

function updateValue(value) {
  draftValue.value = value;
  emit("update:modelValue", value);
}

function insertVariable(variable = {}) {
  const expression = expressionOf(variable);
  if (!expression) return;
  const delimiter = draftValue.value && !/\s$/.test(draftValue.value) ? " " : "";
  updateValue(`${draftValue.value}${delimiter}\${${expression}}`);
}

function locateSegment(segment) {
  const variable = segment.variable;
  const nodeId = variable?.sourceNodeId || variable?.nodeId || variable?.node_id || variable?.selector?.nodeId;
  if (nodeId) emit("locate-node", nodeId);
}

function expressionOf(variable = {}) {
  return variable.expression || variable.path || "";
}

function splitVariableTokens(value = "", variables = new Map()) {
  const segments = [];
  const pattern = /(\$\{([^}]+)\}|\{\{\s*([^}]+?)\s*\}\})/g;
  let lastIndex = 0;
  let match;
  while ((match = pattern.exec(value))) {
    if (match.index > lastIndex) {
      segments.push({ type: "text", value: value.slice(lastIndex, match.index) });
    }
    const expression = String(match[2] || match[3] || "").trim();
    const variable = variables.get(expression) || null;
    segments.push({
      type: "variable",
      value: expression,
      variable,
      stale: !variable,
    });
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < value.length) {
    segments.push({ type: "text", value: value.slice(lastIndex) });
  }
  return segments;
}
</script>

<template>
  <section class="runner-variable-token-input">
    <textarea
      :value="draftValue"
      :rows="rows"
      :placeholder="placeholder"
      :data-testid="inputTestId"
      @input="updateValue($event.target.value)"
    />

    <RunnerVariableSelector
      :variables="variables"
      :expected-type="expectedType"
      @select="insertVariable"
      @locate-node="emit('locate-node', $event)"
    />

    <div class="runner-variable-token-preview" data-testid="runner-variable-token-preview">
      <template v-for="(segment, index) in previewSegments" :key="`${segment.type}-${index}-${segment.value}`">
        <button
          v-if="segment.type === 'variable'"
          type="button"
          class="runner-variable-token"
          :class="{ stale: segment.stale, secret: segment.variable?.secret }"
          :data-testid="`runner-variable-token-${segment.value}`"
          @click="locateSegment(segment)"
        >
          <span>{{ segment.value }}</span>
          <small
            v-if="segment.stale"
            :data-testid="`runner-variable-token-warning-${segment.value}`"
          >
            已失效
          </small>
          <small v-else>{{ segment.variable?.type || "any" }}</small>
        </button>
        <span v-else>{{ segment.value }}</span>
      </template>
    </div>
  </section>
</template>
