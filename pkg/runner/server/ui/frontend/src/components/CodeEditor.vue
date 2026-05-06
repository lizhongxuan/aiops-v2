<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import { NInput } from "naive-ui";

type MonacoModule = typeof import("monaco-editor/esm/vs/editor/editor.api");
type MonacoEditor = import("monaco-editor/esm/vs/editor/editor.api").editor.IStandaloneCodeEditor;
type MonacoDisposable = import("monaco-editor/esm/vs/editor/editor.api").IDisposable;

const props = withDefaults(
  defineProps<{
    modelValue: string;
    language?: string;
    readonly?: boolean;
    height?: string;
    placeholder?: string;
  }>(),
  {
    language: "json",
    readonly: false,
    height: "240px",
    placeholder: "",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const container = ref<HTMLElement | null>(null);
const fallback = ref(false);
const editor = shallowRef<MonacoEditor | null>(null);
const monaco = shallowRef<MonacoModule | null>(null);
let modelListener: MonacoDisposable | null = null;

const editorStyle = computed(() => ({ height: props.height }));

onMounted(() => {
  void mountMonaco();
});

onBeforeUnmount(() => {
  modelListener?.dispose();
  editor.value?.dispose();
  modelListener = null;
  editor.value = null;
});

watch(
  () => props.modelValue,
  (value) => {
    if (!editor.value || editor.value.getValue() === value) return;
    editor.value.setValue(value);
  },
);

watch(
  () => props.readonly,
  (value) => {
    editor.value?.updateOptions({ readOnly: value });
  },
);

watch(
  () => props.language,
  (language) => {
    const model = editor.value?.getModel();
    if (model && monaco.value) {
      monaco.value.editor.setModelLanguage(model, language);
    }
  },
);

async function mountMonaco() {
  if (!container.value || typeof window === "undefined") {
    fallback.value = true;
    return;
  }

  try {
    const loaded = await import("monaco-editor/esm/vs/editor/editor.api");
    await nextTick();
    if (!container.value) return;
    monaco.value = loaded;
    editor.value = loaded.editor.create(container.value, {
      value: props.modelValue,
      language: props.language,
      readOnly: props.readonly,
      automaticLayout: true,
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      fontSize: 12,
      tabSize: 2,
      wordWrap: "on",
      lineNumbersMinChars: 3,
      padding: { top: 10, bottom: 10 },
      theme: "vs-dark",
    });
    modelListener = editor.value.onDidChangeModelContent(() => {
      const value = editor.value?.getValue() || "";
      if (value !== props.modelValue) emit("update:modelValue", value);
    });
  } catch {
    fallback.value = true;
  }
}
</script>

<template>
  <NInput
    v-if="fallback"
    :value="modelValue"
    type="textarea"
    :autosize="{ minRows: 8, maxRows: 22 }"
    :readonly="readonly"
    :placeholder="placeholder"
    @update:value="emit('update:modelValue', $event)"
  />
  <div v-else ref="container" class="code-editor" :style="editorStyle" />
</template>
