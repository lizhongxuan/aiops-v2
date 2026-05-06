<script setup>
import { computed, ref, watch } from "vue";

const TYPE_OPTIONS = ["string", "number", "integer", "boolean", "object", "array", "secret"];
const OUTPUT_SOURCE_OPTIONS = [
  "stdout_text",
  "stdout_jsonpath",
  "stderr_text",
  "exit_code",
  "export_var",
  "approval_result",
  "subflow_output",
];

const props = defineProps({
  mode: {
    type: String,
    default: "inputs",
    validator: (value) => ["inputs", "outputs"].includes(value),
  },
  inputs: {
    type: Array,
    default: () => [],
  },
  outputs: {
    type: Array,
    default: () => [],
  },
  title: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["update:inputs", "update:outputs"]);

const draftInputs = ref([]);
const draftOutputs = ref([]);

const isInputsMode = computed(() => props.mode === "inputs");
const duplicateKeys = computed(() => duplicateKeySet(isInputsMode.value ? draftInputs.value : draftOutputs.value));
const editorTitle = computed(() => props.title || (isInputsMode.value ? "工作流输入 Schema" : "节点输出 Schema"));

watch(
  () => props.inputs,
  (inputs) => {
    draftInputs.value = normalizeInputs(inputs);
  },
  { immediate: true, deep: true },
);

watch(
  () => props.outputs,
  (outputs) => {
    draftOutputs.value = normalizeOutputs(outputs);
  },
  { immediate: true, deep: true },
);

function updateInput(index, patch) {
  const next = draftInputs.value.slice();
  next[index] = normalizeInput({ ...next[index], ...patch });
  if (patch.secret === true || patch.type === "secret") {
    next[index].secret = true;
    next[index].type = "secret";
    delete next[index].default;
  }
  emitInputs(next);
}

function updateOutput(index, patch) {
  const next = draftOutputs.value.slice();
  next[index] = normalizeOutput({ ...next[index], ...patch });
  emitOutputs(next);
}

function updateOutputSource(index, patch) {
  const output = draftOutputs.value[index] || {};
  updateOutput(index, {
    extract_source: {
      ...(output.extract_source || {}),
      ...patch,
    },
  });
}

function updateOutputExample(index, value) {
  const output = draftOutputs.value[index] || {};
  updateOutput(index, {
    ui: {
      ...(output.ui || {}),
      example: value,
    },
  });
}

function addInput() {
  emitInputs([...draftInputs.value, createInput(`input_${draftInputs.value.length + 1}`)]);
}

function addOutput() {
  emitOutputs([...draftOutputs.value, createOutput(`output_${draftOutputs.value.length + 1}`)]);
}

function deleteInput(index) {
  const next = draftInputs.value.slice();
  next.splice(index, 1);
  emitInputs(next);
}

function deleteOutput(index) {
  const next = draftOutputs.value.slice();
  next.splice(index, 1);
  emitOutputs(next);
}

function emitInputs(inputs) {
  draftInputs.value = normalizeInputs(inputs);
  emit("update:inputs", draftInputs.value);
}

function emitOutputs(outputs) {
  draftOutputs.value = normalizeOutputs(outputs);
  emit("update:outputs", draftOutputs.value);
}

function createInput(key) {
  return {
    key,
    type: "string",
    required: false,
    default: "",
    description: "",
  };
}

function createOutput(key) {
  return {
    key,
    type: "string",
    description: "",
    extract_source: { type: "stdout_text", path: "" },
    ui: { example: "" },
  };
}

function normalizeInputs(inputs = []) {
  return Array.isArray(inputs) ? inputs.map(normalizeInput).filter((item) => item.key || item.type) : [];
}

function normalizeInput(input = {}) {
  const secret = Boolean(input.secret || input.type === "secret");
  const out = {
    ...createInput(input.key || "input"),
    ...JSON.parse(JSON.stringify(input || {})),
    type: secret ? "secret" : (input.type || "string"),
    secret,
    required: Boolean(input.required),
    description: input.description || "",
  };
  if (secret) delete out.default;
  else out.default = input.default ?? "";
  return out;
}

function normalizeOutputs(outputs = []) {
  return Array.isArray(outputs) ? outputs.map(normalizeOutput).filter((item) => item.key || item.type) : [];
}

function normalizeOutput(output = {}) {
  return {
    ...createOutput(output.key || "output"),
    ...JSON.parse(JSON.stringify(output || {})),
    type: output.type || "string",
    description: output.description || "",
    extract_source: {
      type: output.extract_source?.type || "stdout_text",
      path: output.extract_source?.path || "",
      expression: output.extract_source?.expression || "",
      value: output.extract_source?.value,
    },
    ui: {
      ...(output.ui || {}),
      example: output.ui?.example ?? output.example ?? "",
    },
  };
}

function duplicateKeySet(items = []) {
  const seen = new Set();
  const duplicates = new Set();
  for (const item of items) {
    const key = String(item.key || "").trim();
    if (!key) continue;
    if (seen.has(key)) duplicates.add(key);
    seen.add(key);
  }
  return duplicates;
}
</script>

<template>
  <section class="runner-schema-editor" :data-testid="`runner-schema-editor-${mode}`">
    <header class="runner-schema-editor-head">
      <div>
        <strong>{{ editorTitle }}</strong>
        <span v-if="isInputsMode">{{ draftInputs.length }} inputs</span>
        <span v-else>{{ draftOutputs.length }} outputs</span>
      </div>
      <button
        v-if="isInputsMode"
        type="button"
        data-testid="input-add"
        @click="addInput"
      >
        新增输入
      </button>
      <button
        v-else
        type="button"
        data-testid="output-add"
        @click="addOutput"
      >
        新增输出
      </button>
    </header>

    <p
      v-for="key in duplicateKeys"
      :key="key"
      class="input-param-list-issue"
    >
      {{ key }} key 重复
    </p>

    <template v-if="isInputsMode">
      <article
        v-for="(input, index) in draftInputs"
        :key="`${input.key}-${index}`"
        class="runner-schema-row"
      >
        <div class="runner-schema-grid">
          <label>
            <span>名称</span>
            <input
              :value="input.key"
              :data-testid="`input-key-${input.key}`"
              @input="updateInput(index, { key: $event.target.value })"
            />
          </label>
          <label>
            <span>类型</span>
            <select
              :value="input.type"
              :data-testid="`input-type-${input.key}`"
              @change="updateInput(index, { type: $event.target.value, secret: $event.target.value === 'secret' })"
            >
              <option v-for="type in TYPE_OPTIONS" :key="type" :value="type">{{ type }}</option>
            </select>
          </label>
          <label>
            <span>默认值</span>
            <input
              :value="input.default ?? ''"
              :disabled="input.secret"
              :data-testid="`schema-input-default-${input.key}`"
              @input="updateInput(index, { default: $event.target.value })"
            />
          </label>
          <label class="runner-schema-checkbox">
            <input
              type="checkbox"
              :checked="input.required"
              :data-testid="`input-required-${input.key}`"
              @change="updateInput(index, { required: $event.target.checked })"
            />
            <span>必填</span>
          </label>
          <label class="runner-schema-checkbox">
            <input
              type="checkbox"
              :checked="input.secret"
              :data-testid="`schema-input-secret-${input.key}`"
              @change="updateInput(index, { secret: $event.target.checked, type: $event.target.checked ? 'secret' : 'string' })"
            />
            <span>secret</span>
          </label>
          <label class="wide">
            <span>描述</span>
            <input
              :value="input.description"
              :data-testid="`input-description-${input.key}`"
              @input="updateInput(index, { description: $event.target.value })"
            />
          </label>
        </div>
        <button
          type="button"
          class="node-panel-secondary"
          :data-testid="`schema-input-delete-${input.key}-${index}`"
          @click="deleteInput(index)"
        >
          删除
        </button>
      </article>
      <p v-if="draftInputs.length === 0" class="runner-studio-empty">暂无工作流输入。</p>
    </template>

    <template v-else>
      <article
        v-for="(output, index) in draftOutputs"
        :key="`${output.key}-${index}`"
        class="runner-schema-row"
      >
        <div class="runner-schema-grid">
          <label>
            <span>变量名</span>
            <input
              :value="output.key"
              :data-testid="`output-key-${output.key}`"
              @input="updateOutput(index, { key: $event.target.value })"
            />
          </label>
          <label>
            <span>类型</span>
            <select
              :value="output.type"
              :data-testid="`output-type-${output.key}`"
              @change="updateOutput(index, { type: $event.target.value })"
            >
              <option v-for="type in TYPE_OPTIONS.filter((item) => item !== 'secret')" :key="type" :value="type">
                {{ type }}
              </option>
            </select>
          </label>
          <label>
            <span>示例</span>
            <input
              :value="output.ui?.example || ''"
              :data-testid="`schema-output-example-${output.key}`"
              @input="updateOutputExample(index, $event.target.value)"
            />
          </label>
          <label>
            <span>来源</span>
            <select
              :value="output.extract_source?.type || 'stdout_text'"
              :data-testid="`output-source-${output.key}`"
              @change="updateOutputSource(index, { type: $event.target.value, path: $event.target.value === 'stdout_jsonpath' ? '$' : '' })"
            >
              <option v-for="source in OUTPUT_SOURCE_OPTIONS" :key="source" :value="source">{{ source }}</option>
            </select>
          </label>
          <label>
            <span>来源路径</span>
            <input
              :value="output.extract_source?.path || ''"
              :data-testid="`schema-output-source-path-${output.key}`"
              placeholder="$.restore_lsn / RUNNER_EXPORT_*"
              @input="updateOutputSource(index, { path: $event.target.value })"
            />
          </label>
          <label class="wide">
            <span>描述</span>
            <input
              :value="output.description"
              :data-testid="`output-description-${output.key}`"
              @input="updateOutput(index, { description: $event.target.value })"
            />
          </label>
        </div>
        <button
          type="button"
          class="node-panel-secondary"
          :data-testid="`schema-output-delete-${output.key}-${index}`"
          @click="deleteOutput(index)"
        >
          删除
        </button>
      </article>
      <p v-if="draftOutputs.length === 0" class="runner-studio-empty">暂无节点输出。</p>
    </template>
  </section>
</template>
