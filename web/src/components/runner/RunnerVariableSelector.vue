<script setup>
import { computed, ref } from "vue";

const props = defineProps({
  variables: {
    type: Array,
    default: () => [],
  },
  expectedType: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["select", "locate-node"]);

const query = ref("");

const filteredVariables = computed(() => {
  const needle = query.value.trim().toLowerCase();
  return props.variables
    .filter((variable) => variable?.expression || variable?.path)
    .filter((variable) => {
      if (!needle) return true;
      return [variable.expression, variable.path, variable.name, variable.scope, variable.description]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle));
    });
});

function expressionOf(variable = {}) {
  return variable.expression || variable.path || "";
}

function typeCompatible(variable = {}) {
  const expected = normalizeType(props.expectedType);
  const actual = normalizeType(variable.type);
  if (!expected || expected === "any" || !actual || actual === "any") return true;
  if (expected === actual) return true;
  if (expected === "number" && actual === "integer") return true;
  if (expected === "integer" && actual === "number") return true;
  return false;
}

function normalizeType(type = "") {
  return String(type || "").trim().toLowerCase();
}

function choose(variable) {
  emit("select", variable);
}

function locate(variable) {
  const nodeId = variable.sourceNodeId || variable.nodeId || variable.node_id || variable.selector?.nodeId;
  if (nodeId) emit("locate-node", nodeId);
}
</script>

<template>
  <section class="runner-variable-selector" data-testid="runner-variable-selector">
    <label class="runner-variable-selector-search">
      <span>变量</span>
      <input
        v-model="query"
        data-testid="runner-variable-selector-search"
        placeholder="搜索 input、env、node、sys"
      />
    </label>

    <div class="runner-variable-selector-list">
      <article
        v-for="variable in filteredVariables"
        :key="expressionOf(variable)"
        class="runner-variable-option"
        :class="{ mismatch: !typeCompatible(variable), secret: variable.secret }"
      >
        <button
          type="button"
          :data-testid="`runner-variable-option-${expressionOf(variable)}`"
          @click="choose(variable)"
        >
          <strong>{{ expressionOf(variable) }}</strong>
          <small>{{ variable.scope }} · {{ variable.type || "any" }}</small>
          <em v-if="variable.secret">secret</em>
          <span v-if="!typeCompatible(variable)">类型可能不匹配</span>
        </button>
        <button
          v-if="variable.sourceNodeId || variable.nodeId || variable.node_id || variable.selector?.nodeId"
          type="button"
          class="runner-variable-locate"
          :data-testid="`runner-variable-locate-${expressionOf(variable)}`"
          @click="locate(variable)"
        >
          定位
        </button>
      </article>
      <p v-if="!filteredVariables.length" class="runner-variable-empty">暂无可用变量。</p>
    </div>
  </section>
</template>
