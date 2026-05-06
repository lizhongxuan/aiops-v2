<script setup>
import { computed, ref, watch } from "vue";
import BasicTab from "../node-config/BasicTab.vue";
import RunnerVariableTokenInput from "../RunnerVariableTokenInput.vue";

const props = defineProps({
  node: {
    type: Object,
    required: true,
  },
  actions: {
    type: Array,
    default: () => [],
  },
  variables: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:node", "locate-node"]);

const draftNode = ref(cloneNode(props.node));

const actionSpec = computed(() => {
  const action = draftNode.value?.step?.action || "";
  return props.actions.find((item) => (item.action || item.name) === action) || null;
});

const schema = computed(() =>
  normalizeSchema(actionSpec.value?.input_schema || actionSpec.value?.inputs_schema || actionSpec.value?.args_schema),
);
const fields = computed(() =>
  Object.entries(schema.value.properties || {})
    .filter(([key]) => key !== "env")
    .map(([key, value]) => ({
      key,
      schema: value || {},
      required: (schema.value.required || []).includes(key),
    })),
);
const inputRows = computed(() => (Array.isArray(draftNode.value?.inputs) ? draftNode.value.inputs : []));
const envRows = computed(() => {
  const env = draftNode.value?.step?.args?.env;
  if (!env || typeof env !== "object" || Array.isArray(env)) return [];
  return Object.entries(env).map(([key, value]) => ({ key, value: String(value ?? "") }));
});

function cloneNode(node) {
  return node ? JSON.parse(JSON.stringify(node)) : null;
}

watch(
  () => props.node,
  (node) => {
    draftNode.value = cloneNode(node);
  },
  { immediate: true },
);

function normalizeSchema(raw) {
  if (!raw) return { type: "object", properties: {}, required: [] };
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw);
    } catch {
      return { type: "object", properties: {}, required: [] };
    }
  }
  return raw;
}

function fieldLabel(field) {
  return field.schema.title || field.key;
}

function inputType(field) {
  const type = schemaType(field.schema);
  if (type === "integer" || type === "number") return "number";
  return "text";
}

function schemaType(fieldSchema = {}) {
  if (Array.isArray(fieldSchema.type)) return fieldSchema.type.find((item) => item !== "null") || "string";
  return fieldSchema.type || "string";
}

function argValue(key) {
  return draftNode.value?.step?.args?.[key] ?? "";
}

function updateDraft(node) {
  draftNode.value = cloneNode(node);
  emit("update:node", cloneNode(draftNode.value));
}

function updateArg(key, rawValue, fieldSchema = {}) {
  const nextArgs = {
    ...(draftNode.value?.step?.args || {}),
    [key]: normalizeValue(rawValue, fieldSchema),
  };
  updateDraft({
    ...draftNode.value,
    step: {
      ...(draftNode.value.step || {}),
      args: nextArgs,
    },
  });
}

function updateInputs(nextInputs) {
  updateDraft({
    ...draftNode.value,
    inputs: nextInputs.map((input) => ({
      key: String(input.key || "").trim(),
      type: String(input.type || "string"),
      required: Boolean(input.required),
    })).filter((input) => input.key),
  });
}

function addInputRow() {
  updateInputs([
    ...inputRows.value,
    { key: uniqueRowKey("input", inputRows.value.map((row) => row.key)), type: "string", required: false },
  ]);
}

function updateInputRow(index, patch = {}) {
  const next = inputRows.value.map((row, rowIndex) => (rowIndex === index ? { ...row, ...patch } : row));
  updateInputs(next);
}

function removeInputRow(index) {
  updateInputs(inputRows.value.filter((_, rowIndex) => rowIndex !== index));
}

function setEnvRows(rows) {
  const env = {};
  for (const row of rows) {
    const key = String(row.key || "").trim();
    if (!key) continue;
    env[key] = String(row.value ?? "");
  }
  const nextArgs = { ...(draftNode.value?.step?.args || {}) };
  if (Object.keys(env).length > 0) nextArgs.env = env;
  else delete nextArgs.env;
  updateDraft({
    ...draftNode.value,
    step: {
      ...(draftNode.value.step || {}),
      args: nextArgs,
    },
  });
}

function addEnvRow() {
  setEnvRows([
    ...envRows.value,
    { key: uniqueRowKey("ENV", envRows.value.map((row) => row.key)), value: "" },
  ]);
}

function updateEnvRow(index, patch = {}) {
  const next = envRows.value.map((row, rowIndex) => (rowIndex === index ? { ...row, ...patch } : row));
  setEnvRows(next);
}

function removeEnvRow(index) {
  setEnvRows(envRows.value.filter((_, rowIndex) => rowIndex !== index));
}

function uniqueRowKey(prefix, existingKeys = []) {
  const used = new Set(existingKeys.filter(Boolean));
  let index = 1;
  while (used.has(`${prefix}_${index}`)) index += 1;
  return `${prefix}_${index}`;
}

function normalizeValue(value, fieldSchema = {}) {
  const type = schemaType(fieldSchema);
  if (type === "boolean") return Boolean(value);
  if (type === "integer") return Number.parseInt(value || "0", 10);
  if (type === "number") return Number(value || 0);
  if (type === "object") return parseJsonObject(value);
  if (type === "array") return parseArray(value);
  return String(value ?? "");
}

function parseJsonObject(value) {
  if (typeof value === "object" && value !== null && !Array.isArray(value)) return value;
  try {
    const parsed = JSON.parse(String(value || "{}"));
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function parseArray(value) {
  if (Array.isArray(value)) return value;
  const text = String(value || "").trim();
  if (!text) return [];
  try {
    const parsed = JSON.parse(text);
    if (Array.isArray(parsed)) return parsed;
  } catch {
    // Fall through to comma parsing.
  }
  return text
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function objectText(key) {
  const value = argValue(key);
  if (!value || typeof value !== "object") return "";
  return JSON.stringify(value);
}

function arrayText(key) {
  const value = argValue(key);
  return Array.isArray(value) ? value.join(", ") : String(value || "");
}

function isTokenStringField(field) {
  return schemaType(field.schema) === "string" && !field.schema.enum;
}
</script>

<template>
  <section class="node-panel-form" data-testid="action-node-panel">
    <BasicTab :node="draftNode" :actions="actions" @update:node="updateDraft" />

    <section class="node-panel-section">
      <header class="runner-structured-section-head">
        <div>
          <strong>输入变量</strong>
          <p>为当前模块声明可选入参，运行时可由上游节点、工作流输入或手工变量赋值。</p>
        </div>
        <button type="button" class="node-panel-secondary" data-testid="action-input-add" @click="addInputRow">
          + 添加输入
        </button>
      </header>
      <div v-if="inputRows.length" class="runner-kv-list">
        <div v-for="(input, index) in inputRows" :key="`${input.key}-${index}`" class="runner-io-row">
          <input
            :value="input.key"
            :data-testid="`action-input-key-${index}`"
            placeholder="变量名"
            @input="updateInputRow(index, { key: $event.target.value })"
          />
          <select
            :value="input.type || 'string'"
            :data-testid="`action-input-type-${index}`"
            @change="updateInputRow(index, { type: $event.target.value })"
          >
            <option value="string">String</option>
            <option value="number">Number</option>
            <option value="boolean">Boolean</option>
            <option value="object">Object</option>
            <option value="array">Array</option>
          </select>
          <label class="runner-inline-check">
            <input
              type="checkbox"
              :checked="Boolean(input.required)"
              :data-testid="`action-input-required-${index}`"
              @change="updateInputRow(index, { required: $event.target.checked })"
            />
            必填
          </label>
          <button
            type="button"
            class="node-panel-danger"
            :data-testid="`action-input-delete-${index}`"
            @click="removeInputRow(index)"
          >
            删除
          </button>
        </div>
      </div>
      <p v-else class="node-panel-empty">未声明输入变量。</p>
    </section>

    <section class="node-panel-section">
      <header class="runner-structured-section-head">
        <div>
          <strong>环境变量</strong>
          <p>按键值行配置模块运行环境；不需要时保持为空。</p>
        </div>
        <button type="button" class="node-panel-secondary" data-testid="action-env-add" @click="addEnvRow">
          + 添加环境变量
        </button>
      </header>
      <div v-if="envRows.length" class="runner-kv-list">
        <div v-for="(row, index) in envRows" :key="`${row.key}-${index}`" class="runner-kv-row">
          <input
            :value="row.key"
            :data-testid="`action-env-key-${index}`"
            placeholder="KEY"
            @input="updateEnvRow(index, { key: $event.target.value })"
          />
          <RunnerVariableTokenInput
            :model-value="row.value"
            :variables="variables"
            expected-type="string"
            :input-test-id="`action-env-value-${index}`"
            :rows="1"
            placeholder="value 或 {{node.output}}"
            @update:model-value="updateEnvRow(index, { value: $event })"
            @locate-node="emit('locate-node', $event)"
          />
          <button
            type="button"
            class="node-panel-danger"
            :data-testid="`action-env-delete-${index}`"
            @click="removeEnvRow(index)"
          >
            删除
          </button>
        </div>
      </div>
      <p v-else class="node-panel-empty">未设置环境变量。</p>
    </section>

    <section class="node-panel-section">
      <header>
        <strong>{{ actionSpec?.title || actionSpec?.label || draftNode.step?.action || "Action 参数" }}</strong>
        <p>{{ actionSpec?.description || "根据 action catalog schema 配置运行参数。" }}</p>
      </header>

      <div v-if="fields.length" class="node-config-form">
        <label v-for="field in fields" :key="field.key">
          <span>{{ fieldLabel(field) }}<em v-if="field.required">*</em></span>
          <select
            v-if="field.schema.enum"
            :value="argValue(field.key)"
            :data-testid="`action-schema-field-${field.key}`"
            @change="updateArg(field.key, $event.target.value, field.schema)"
          >
            <option v-for="option in field.schema.enum" :key="option" :value="option">{{ option }}</option>
          </select>
          <input
            v-else-if="schemaType(field.schema) === 'boolean'"
            type="checkbox"
            :checked="Boolean(argValue(field.key))"
            :data-testid="`action-schema-field-${field.key}`"
            @change="updateArg(field.key, $event.target.checked, field.schema)"
          />
          <textarea
            v-else-if="schemaType(field.schema) === 'object'"
            :value="objectText(field.key)"
            :data-testid="`action-schema-field-${field.key}`"
            placeholder="{ }"
            @input="updateArg(field.key, $event.target.value, field.schema)"
          />
          <RunnerVariableTokenInput
            v-else-if="isTokenStringField(field)"
            :model-value="String(argValue(field.key) ?? '')"
            :variables="variables"
            expected-type="string"
            :input-test-id="`action-schema-field-${field.key}`"
            :rows="field.key === 'script' ? 6 : 3"
            :placeholder="field.schema.placeholder || ''"
            @update:model-value="updateArg(field.key, $event, field.schema)"
            @locate-node="emit('locate-node', $event)"
          />
          <input
            v-else
            :type="inputType(field)"
            :value="schemaType(field.schema) === 'array' ? arrayText(field.key) : argValue(field.key)"
            :data-testid="`action-schema-field-${field.key}`"
            @input="updateArg(field.key, $event.target.value, field.schema)"
          />
          <small v-if="field.schema.description">{{ field.schema.description }}</small>
        </label>
      </div>
      <p v-else class="node-panel-empty">当前 action 没有声明参数 schema。</p>
    </section>
  </section>
</template>
