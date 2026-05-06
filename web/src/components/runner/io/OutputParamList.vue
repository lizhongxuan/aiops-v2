<script setup>
import { computed, ref, watch } from "vue";
import OutputParamRow from "./OutputParamRow.vue";
import { cloneOutputParam, createOutputParam, normalizeOutputParams, validateOutputParams } from "./outputTypes";

const props = defineProps({
  outputs: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:outputs"]);

const draftOutputs = ref([]);
const normalizedOutputs = computed(() => draftOutputs.value);
const issues = computed(() => validateOutputParams(draftOutputs.value));

watch(
  () => props.outputs,
  (outputs) => {
    draftOutputs.value = normalizeOutputParams(outputs);
  },
  { immediate: true },
);

function issueFor(index, output) {
  return issues.value.find((issue) => issue.key === output.key && (issue.code !== "duplicate_key" || index > 0)) || null;
}

function emitOutputs(outputs) {
  draftOutputs.value = normalizeOutputParams(outputs);
  emit("update:outputs", draftOutputs.value);
}

function updateAt(index, output) {
  const next = normalizedOutputs.value.slice();
  next[index] = cloneOutputParam(output);
  emitOutputs(next);
}

function move(index, direction) {
  const target = index + direction;
  if (target < 0 || target >= normalizedOutputs.value.length) return;
  const next = normalizedOutputs.value.slice();
  [next[index], next[target]] = [next[target], next[index]];
  emitOutputs(next);
}

function copyAt(index) {
  const next = normalizedOutputs.value.slice();
  next.splice(index + 1, 0, cloneOutputParam(next[index]));
  emitOutputs(next);
}

function deleteAt(index) {
  const next = normalizedOutputs.value.slice();
  next.splice(index, 1);
  emitOutputs(next);
}

function addOutput() {
  emitOutputs([...normalizedOutputs.value, createOutputParam(`output_${normalizedOutputs.value.length + 1}`)]);
}
</script>

<template>
  <section class="output-param-list">
    <header class="input-param-list-head">
      <div>
        <strong>结构化输出</strong>
        <span>{{ normalizedOutputs.length }} outputs</span>
      </div>
      <button type="button" data-testid="output-add" @click="addOutput">新增输出</button>
    </header>

    <p v-for="issue in issues" :key="`${issue.code}-${issue.key}`" class="input-param-list-issue">
      {{ issue.message }}
    </p>

    <OutputParamRow
      v-for="(output, index) in normalizedOutputs"
      :key="`${output.key}-${index}`"
      :output="output"
      :index="index"
      :issue="issueFor(index, output)"
      @update:output="updateAt(index, $event)"
      @move-up="move(index, -1)"
      @move-down="move(index, 1)"
      @copy="copyAt(index)"
      @delete="deleteAt(index)"
    />

    <p v-if="normalizedOutputs.length === 0" class="runner-studio-empty">暂无输出变量。</p>
  </section>
</template>
