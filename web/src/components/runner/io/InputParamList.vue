<script setup>
import { computed, ref, watch } from "vue";
import InputParamRow from "./InputParamRow.vue";
import { cloneInputParam, createInputParam, normalizeInputParams, validateInputParams } from "./ioTypes";

const props = defineProps({
  params: {
    type: Array,
    default: () => [],
  },
  variables: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(["update:params"]);

const draftParams = ref([]);
const normalizedParams = computed(() => draftParams.value);
const issues = computed(() => validateInputParams(draftParams.value));

watch(
  () => props.params,
  (params) => {
    draftParams.value = normalizeInputParams(params);
  },
  { immediate: true },
);

function issueFor(index, param) {
  return issues.value.find((issue) => issue.key === param.key && (issue.code !== "duplicate_key" || index > 0)) || null;
}

function emitParams(params) {
  draftParams.value = normalizeInputParams(params);
  emit("update:params", draftParams.value);
}

function updateAt(index, param) {
  const next = normalizedParams.value.slice();
  next[index] = cloneInputParam(param);
  emitParams(next);
}

function move(index, direction) {
  const target = index + direction;
  if (target < 0 || target >= normalizedParams.value.length) return;
  const next = normalizedParams.value.slice();
  [next[index], next[target]] = [next[target], next[index]];
  emitParams(next);
}

function copyAt(index) {
  const next = normalizedParams.value.slice();
  next.splice(index + 1, 0, cloneInputParam(next[index]));
  emitParams(next);
}

function deleteAt(index) {
  const next = normalizedParams.value.slice();
  next.splice(index, 1);
  emitParams(next);
}

function addParam() {
  emitParams([...normalizedParams.value, createInputParam(`input_${normalizedParams.value.length + 1}`)]);
}
</script>

<template>
  <section class="input-param-list">
    <header class="input-param-list-head">
      <div>
        <strong>结构化输入</strong>
        <span>{{ normalizedParams.length }} inputs</span>
      </div>
      <button type="button" data-testid="input-add" @click="addParam">新增输入</button>
    </header>

    <p v-for="issue in issues" :key="`${issue.code}-${issue.key}`" class="input-param-list-issue">
      {{ issue.message }}
    </p>

    <InputParamRow
      v-for="(param, index) in normalizedParams"
      :key="`${param.key}-${index}`"
      :param="param"
      :index="index"
      :variables="variables"
      :issue="issueFor(index, param)"
      @update:param="updateAt(index, $event)"
      @move-up="move(index, -1)"
      @move-down="move(index, 1)"
      @copy="copyAt(index)"
      @delete="deleteAt(index)"
    />

    <p v-if="normalizedParams.length === 0" class="runner-studio-empty">暂无输入参数。</p>
  </section>
</template>
