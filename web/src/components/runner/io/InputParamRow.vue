<script setup>
import { computed } from "vue";
import MixedVariableTextInput from "./MixedVariableTextInput.vue";
import ValueSourceSwitch from "./ValueSourceSwitch.vue";
import VariableReferencePicker from "./VariableReferencePicker.vue";
import { cloneInputParam, normalizeValueSource } from "./ioTypes";

const props = defineProps({
  param: {
    type: Object,
    required: true,
  },
  index: {
    type: Number,
    required: true,
  },
  variables: {
    type: Array,
    default: () => [],
  },
  issue: {
    type: Object,
    default: null,
  },
});

const emit = defineEmits(["update:param", "move-up", "move-down", "copy", "delete"]);

const paramKey = computed(() => props.param.key || `input-${props.index}`);
const source = computed(() => normalizeValueSource(props.param.value_source));

function updateParam(patch) {
  emit("update:param", {
    ...cloneInputParam(props.param),
    ...patch,
  });
}

function updateValueSource(patch) {
  updateParam({
    value_source: {
      ...source.value,
      ...patch,
    },
  });
}

function changeValueSource(type) {
  if (type === "variable_reference") updateParam({ value_source: { type, variable: null } });
  else if (type === "expression") updateParam({ value_source: { type, expression: "" } });
  else updateParam({ value_source: { type, value: "" } });
}
</script>

<template>
  <article class="input-param-row">
    <div class="input-param-grid">
      <label>
        <span>key</span>
        <input
          :value="param.key"
          :data-testid="`input-key-${paramKey}`"
          @input="updateParam({ key: $event.target.value })"
        />
      </label>
      <label>
        <span>label</span>
        <input
          :value="param.label"
          :data-testid="`input-label-${paramKey}`"
          @input="updateParam({ label: $event.target.value })"
        />
      </label>
      <label>
        <span>type</span>
        <select
          :value="param.type"
          :data-testid="`input-type-${paramKey}`"
          @change="updateParam({ type: $event.target.value })"
        >
          <option value="string">string</option>
          <option value="number">number</option>
          <option value="boolean">boolean</option>
          <option value="object">object</option>
          <option value="array">array</option>
        </select>
      </label>
      <label>
        <span>value_source</span>
        <ValueSourceSwitch
          :model-value="source.type"
          :test-id="`input-source-${paramKey}`"
          @update:model-value="changeValueSource"
        />
      </label>
      <label class="input-param-required">
        <input
          type="checkbox"
          :checked="param.required"
          :data-testid="`input-required-${paramKey}`"
          @change="updateParam({ required: $event.target.checked })"
        />
        <span>required</span>
      </label>
      <label>
        <span>description</span>
        <input
          :value="param.description"
          :data-testid="`input-description-${paramKey}`"
          @input="updateParam({ description: $event.target.value })"
        />
      </label>
    </div>

    <MixedVariableTextInput
      v-if="source.type === 'constant'"
      :model-value="String(source.value ?? '')"
      @update:model-value="updateValueSource({ value: $event })"
    />
    <VariableReferencePicker
      v-else-if="source.type === 'variable_reference'"
      :model-value="source.variable"
      :variables="variables"
      @update:model-value="updateValueSource({ variable: $event })"
    />
    <label v-else class="input-param-expression">
      <span>expression</span>
      <input
        :value="source.expression || ''"
        :data-testid="`input-expression-${paramKey}`"
        @input="updateValueSource({ expression: $event.target.value })"
      />
    </label>

    <p v-if="issue" class="input-param-issue">{{ issue.message }}</p>

    <div class="input-param-actions">
      <button type="button" :data-testid="`input-move-up-${paramKey}-${index}`" @click="emit('move-up')">上移</button>
      <button type="button" :data-testid="`input-move-down-${paramKey}-${index}`" @click="emit('move-down')">下移</button>
      <button type="button" :data-testid="`input-copy-${paramKey}-${index}`" @click="emit('copy')">复制</button>
      <button type="button" :data-testid="`input-delete-${paramKey}-${index}`" @click="emit('delete')">删除</button>
    </div>
  </article>
</template>
