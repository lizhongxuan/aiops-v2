<script setup>
import { computed } from "vue";
import ExtractSourceSelect from "./ExtractSourceSelect.vue";
import JsonPathEditor from "./JsonPathEditor.vue";
import { cloneOutputParam, normalizeExtractSource } from "./outputTypes";

const props = defineProps({
  output: {
    type: Object,
    required: true,
  },
  index: {
    type: Number,
    required: true,
  },
  issue: {
    type: Object,
    default: null,
  },
});

const emit = defineEmits(["update:output", "move-up", "move-down", "copy", "delete"]);

const outputKey = computed(() => props.output.key || `output-${props.index}`);
const extractSource = computed(() => normalizeExtractSource(props.output.extract_source));

function updateOutput(patch) {
  emit("update:output", {
    ...cloneOutputParam(props.output),
    ...patch,
  });
}

function updateExtractSource(patch) {
  updateOutput({
    extract_source: {
      ...extractSource.value,
      ...patch,
    },
  });
}

function changeExtractSource(type) {
  updateOutput({
    extract_source: {
      type,
      path: type === "stdout_jsonpath" ? "$" : "",
    },
  });
}
</script>

<template>
  <article class="output-param-row">
    <header class="output-param-row-head">
      <strong>{{ output.key || `output-${index + 1}` }}</strong>
      <span>{{ output.type || "string" }} · {{ extractSource.type }}</span>
    </header>
    <div class="output-param-grid">
      <label>
        <span>key</span>
        <input
          :value="output.key"
          :data-testid="`output-key-${outputKey}`"
          @input="updateOutput({ key: $event.target.value })"
        />
      </label>
      <label>
        <span>type</span>
        <select
          :value="output.type"
          :data-testid="`output-type-${outputKey}`"
          @change="updateOutput({ type: $event.target.value })"
        >
          <option value="string">string</option>
          <option value="number">number</option>
          <option value="boolean">boolean</option>
          <option value="object">object</option>
          <option value="array">array</option>
        </select>
      </label>
      <label>
        <span>extract_source</span>
        <ExtractSourceSelect
          :model-value="extractSource.type"
          :test-id="`output-source-${outputKey}`"
          @update:model-value="changeExtractSource"
        />
      </label>
      <label>
        <span>description</span>
        <input
          :value="output.description"
          :data-testid="`output-description-${outputKey}`"
          @input="updateOutput({ description: $event.target.value })"
        />
      </label>
    </div>

    <JsonPathEditor
      v-if="extractSource.type === 'stdout_jsonpath'"
      :model-value="extractSource.path"
      :test-id="`jsonpath-${outputKey}`"
      @update:model-value="updateExtractSource({ path: $event })"
    />
    <label v-else-if="extractSource.type === 'export_var' || extractSource.type === 'subflow_output'" class="jsonpath-editor">
      <span>extract_rule</span>
      <input
        :value="extractSource.path"
        :data-testid="`extract-rule-${outputKey}`"
        @input="updateExtractSource({ path: $event.target.value })"
      />
    </label>

    <p v-if="issue" class="input-param-issue">{{ issue.message }}</p>

    <div class="input-param-actions">
      <button type="button" :data-testid="`output-move-up-${outputKey}-${index}`" @click="emit('move-up')">上移</button>
      <button type="button" :data-testid="`output-move-down-${outputKey}-${index}`" @click="emit('move-down')">下移</button>
      <button type="button" :data-testid="`output-copy-${outputKey}-${index}`" @click="emit('copy')">复制</button>
      <button type="button" :data-testid="`output-delete-${outputKey}-${index}`" @click="emit('delete')">删除</button>
    </div>
  </article>
</template>
