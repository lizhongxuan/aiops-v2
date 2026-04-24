<script setup>
import { computed, ref, watch } from "vue";
import { ListIcon } from "lucide-vue-next";
import { NCard, NRadioGroup, NRadio, NCheckboxGroup, NCheckbox, NButton, NTag, NSpace } from "naive-ui";

const OTHER_OPTION_VALUE = "__other__";
const DEFAULT_OPTION_DESCRIPTION = "选择后会按该方案继续推进。";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  sessionKind: {
    type: String,
    default: "",
  },
  submitting: {
    type: Boolean,
    default: false,
  },
  errorMessage: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["choice"]);

const selectedValues = ref([]);
const multiSelectedValues = ref([]);
const otherValues = ref([]);
const noteValues = ref([]);
const noteExpanded = ref([]);
const selectionSignature = ref("");

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function getOptionValue(option, index) {
  return option?.value || option?.label || `option-${index}`;
}

function getOptionLabel(option) {
  return option?.label || option?.value || "未命名选项";
}

function isRecommendedOption(option) {
  if (option?.recommended === true) return true;
  return /^推荐[:：]/.test(getOptionLabel(option));
}

function normalizeOption(option, index) {
  return {
    ...option,
    _value: getOptionValue(option, index),
    _label: getOptionLabel(option),
    _description: String(option?.description || "").trim() || DEFAULT_OPTION_DESCRIPTION,
    _recommended: isRecommendedOption(option),
    _originalIndex: index,
  };
}

function normalizeQuestion(question, index, fallbackTitle) {
  const normalizedOptions = asArray(question?.options)
    .map((option, optionIndex) => normalizeOption(option, optionIndex))
    .sort((left, right) => {
      if (left._recommended !== right._recommended) return left._recommended ? -1 : 1;
      return left._originalIndex - right._originalIndex;
    });

  return {
    header: question?.header || (index === 0 ? fallbackTitle : ""),
    question: question?.question || "",
    isOther: Boolean(question?.isOther),
    isSecret: Boolean(question?.isSecret),
    multiSelect: Boolean(question?.multiSelect),
    options: normalizedOptions,
    allowSupplementNote: question?.allowSupplementNote !== false,
    notePlaceholder: String(question?.notePlaceholder || "").trim() || "补充偏好、风险边界，或你已经确认过的信息（选填）",
  };
}

const choiceQuestions = computed(() => {
  if (props.card.questions?.length) {
    return props.card.questions.map((question, index) =>
      normalizeQuestion(question, index, props.card.title || ""),
    );
  }
  if (props.card.question || props.card.options?.length) {
    return [
      normalizeQuestion(
        {
          header: props.card.title || "",
          question: props.card.question || "",
          options: props.card.options || [],
          isOther: false,
          isSecret: false,
          allowSupplementNote: true,
          notePlaceholder: props.card.notePlaceholder || "",
        },
        0,
        props.card.title || "",
      ),
    ];
  }
  return [];
});

const resolvedSummary = computed(() => props.card.answerSummary || []);
const contextLabel = computed(() => (props.sessionKind === "workspace" ? "工作台输入请求" : ""));

const canSubmit = computed(() => {
  if (props.card.status !== "pending") return false;
  return choiceQuestions.value.every((question, index) => {
    if (question.multiSelect) {
      const selected = multiSelectedValues.value[index] || [];
      return selected.length > 0;
    }
    const selectedValue = selectedValues.value[index];
    if (question.options?.length && selectedValue !== OTHER_OPTION_VALUE) {
      return Boolean(selectedValue);
    }
    return Boolean(otherValues.value[index]?.trim());
  });
});

function getQuestionHeader(question, index) {
  if (question.header) return question.header;
  if (choiceQuestions.value.length > 1) return `问题 ${index + 1}`;
  return "";
}

function showInlineInput(question, index) {
  if (!question.options?.length) return true;
  return question.isOther && selectedValues.value[index] === OTHER_OPTION_VALUE;
}

function showSupplementNote(question) {
  return question.allowSupplementNote !== false;
}

function toggleSupplementNote(index) {
  noteExpanded.value[index] = !noteExpanded.value[index];
}

function defaultSelectedValue(question) {
  if (question.options?.length) return question.options[0]._value;
  return OTHER_OPTION_VALUE;
}

function hasOptionValue(question, value) {
  if (question.isOther && value === OTHER_OPTION_VALUE) return true;
  return Boolean(question.options?.some((option) => option._value === value));
}

function buildSelectionSignature(questions) {
  return JSON.stringify({
    requestId: props.card.requestId || props.card.id || "",
    questions: questions.map((question) => ({
      header: question.header,
      question: question.question,
      isOther: question.isOther,
      isSecret: question.isSecret,
      options: asArray(question.options).map((option) => ({
        value: option._value,
        label: option._label,
      })),
    })),
  });
}

function alignValues(values, questions, fallback) {
  return questions.map((_, index) => values[index] ?? fallback(index));
}

watch(
  choiceQuestions,
  (questions) => {
    const nextSignature = buildSelectionSignature(questions);
    const isSamePrompt = selectionSignature.value === nextSignature;
    const previousSelected = selectedValues.value;

    selectedValues.value = questions.map((question, index) => {
      if (question.multiSelect) return null;
      const previous = previousSelected[index];
      if (isSamePrompt && hasOptionValue(question, previous)) return previous;
      return defaultSelectedValue(question);
    });
    multiSelectedValues.value = isSamePrompt
      ? alignValues(multiSelectedValues.value, questions, () => [])
      : questions.map(() => []);
    otherValues.value = isSamePrompt ? alignValues(otherValues.value, questions, () => "") : questions.map(() => "");
    noteValues.value = isSamePrompt ? alignValues(noteValues.value, questions, () => "") : questions.map(() => "");
    noteExpanded.value = isSamePrompt ? alignValues(noteExpanded.value, questions, () => false) : questions.map(() => false);
    selectionSignature.value = nextSignature;
  },
  { immediate: true },
);

function onSubmit() {
  if (!canSubmit.value || props.submitting) return;
  emit("choice", {
    requestId: props.card.requestId,
    answers: choiceQuestions.value.map((question, index) => {
      const note = noteValues.value[index]?.trim() || "";

      if (question.multiSelect) {
        const selected = multiSelectedValues.value[index] || [];
        const selectedOptions = selected
          .map((val) => question.options.find((opt) => opt._value === val))
          .filter(Boolean);
        return {
          values: selectedOptions.map((opt) => ({ value: opt._value, label: opt._label })),
          multiSelect: true,
          isOther: false,
          note,
        };
      }

      const selectedValue = selectedValues.value[index];
      if (!question.options?.length || selectedValue === OTHER_OPTION_VALUE) {
        const value = otherValues.value[index].trim();
        return { value, label: value, isOther: true, note };
      }

      const selectedOption = question.options.find((option) => option._value === selectedValue);
      return {
        value: selectedOption ? selectedOption._value : selectedValue,
        label: selectedOption ? selectedOption._label : selectedValue,
        isOther: false,
        note,
      };
    }),
  });
}
</script>

<template>
  <n-card class="choice-card" size="small">
    <template #header>
      <div class="choice-header-content">
        <ListIcon size="16" class="choice-icon" />
        <div class="choice-title-group">
          <span v-if="contextLabel" class="choice-context">{{ contextLabel }}</span>
          <span class="choice-title">{{ card.title || "请选择" }}</span>
        </div>
      </div>
    </template>

    <div class="choice-body">
      <div
        v-for="(question, index) in choiceQuestions"
        :key="`${card.id}-${index}`"
        class="choice-question-block"
      >
        <p v-if="getQuestionHeader(question, index)" class="choice-block-header">
          {{ getQuestionHeader(question, index) }}
        </p>
        <p v-if="question.question" class="choice-question">{{ question.question }}</p>

        <div v-if="card.status === 'pending' && question.options?.length" class="choice-options">
          <!-- Multi-select: checkboxes -->
          <template v-if="question.multiSelect">
            <n-checkbox-group :value="multiSelectedValues[index] || []" @update:value="multiSelectedValues[index] = $event">
              <n-space vertical :size="4">
                <n-checkbox
                  v-for="(option, optionIndex) in question.options"
                  :key="`${card.id}-${index}-${optionIndex}`"
                  :value="option._value"
                  :label="option._label"
                />
              </n-space>
            </n-checkbox-group>
          </template>

          <!-- Single-select: radio group -->
          <template v-else>
            <n-radio-group :value="selectedValues[index]" @update:value="selectedValues[index] = $event">
              <n-space vertical :size="4">
                <n-radio
                  v-for="(option, optionIndex) in question.options"
                  :key="`${card.id}-${index}-${optionIndex}`"
                  :value="option._value"
                >
                  {{ option._label }}
                  <n-tag v-if="option._recommended" size="tiny" type="info" round style="margin-left: 6px">推荐</n-tag>
                </n-radio>
                <n-radio v-if="question.isOther" :value="OTHER_OPTION_VALUE">其他</n-radio>
              </n-space>
            </n-radio-group>
          </template>
        </div>

        <div v-if="card.status === 'pending' && showInlineInput(question, index)" class="choice-inline-input">
          <input
            v-model="otherValues[index]"
            class="choice-input"
            :type="question.isSecret ? 'password' : 'text'"
            :placeholder="question.isSecret ? '请输入保密内容' : '请输入内容'"
          />
        </div>

        <div v-if="card.status === 'pending' && showSupplementNote(question)" class="choice-note-block">
          <button
            type="button"
            class="choice-note-toggle"
            data-testid="choice-note-toggle"
            @click="toggleSupplementNote(index)"
          >
            {{ noteExpanded[index] ? "收起补充说明" : "补充说明（选填）" }}
          </button>
          <textarea
            v-if="noteExpanded[index]"
            v-model="noteValues[index]"
            class="choice-note-input"
            data-testid="choice-note-input"
            :placeholder="question.notePlaceholder"
          ></textarea>
        </div>
      </div>

      <div v-if="card.status === 'pending'" class="choice-footer">
        <p v-if="errorMessage" class="choice-error" data-testid="choice-error-message">{{ errorMessage }}</p>
        <n-button type="primary" size="small" :disabled="!canSubmit || submitting" :loading="submitting" @click="onSubmit">
          提交 ↵
        </n-button>
      </div>

      <div v-if="card.status !== 'pending'" class="choice-resolved">
        <div v-if="resolvedSummary.length" class="choice-resolved-list">
          <div v-for="(entry, index) in resolvedSummary" :key="`${card.id}-resolved-${index}`">{{ entry }}</div>
        </div>
        <div v-else>已完成选择</div>
      </div>
    </div>
  </n-card>
</template>

<style scoped>
.choice-card {
  margin-top: 2px;
  margin-left: 36px;
  max-width: 680px;
}

.choice-header-content {
  display: flex;
  align-items: center;
  gap: 8px;
}

.choice-icon {
  color: #6b7280;
}

.choice-title {
  font-size: 13px;
  font-weight: 600;
  color: #1f2937;
}

.choice-title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.choice-context {
  font-size: 11px;
  color: #475569;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.choice-body {
  padding: 4px 0;
}

.choice-question-block + .choice-question-block {
  margin-top: 20px;
  padding-top: 18px;
  border-top: 1px solid #f3f4f6;
}

.choice-block-header {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}

.choice-question {
  margin: 0 0 10px;
  font-size: 13px;
  line-height: 1.5;
  color: #374151;
}

.choice-options {
  margin-top: 4px;
}

.choice-inline-input {
  margin-top: 12px;
}

.choice-input,
.choice-note-input {
  width: 100%;
  border-radius: 10px;
  border: 1px solid #d1d5db;
  background: #ffffff;
  padding: 11px 14px;
  font-size: 14px;
  color: #1f2937;
  outline: none;
}

.choice-note-input {
  min-height: 92px;
  resize: vertical;
  margin-top: 8px;
  font-family: inherit;
}

.choice-input:focus,
.choice-note-input:focus {
  border-color: #0f172a;
  box-shadow: 0 0 0 3px rgba(15, 23, 42, 0.08);
}

.choice-note-block {
  margin-top: 12px;
}

.choice-note-toggle {
  border: none;
  background: transparent;
  color: #334155;
  font-size: 12px;
  font-weight: 600;
  padding: 0;
  cursor: pointer;
}

.choice-note-toggle:hover {
  color: #0f172a;
}

.choice-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 18px;
}

.choice-error {
  margin: 0 auto 0 0;
  color: #b91c1c;
  font-size: 12px;
  font-weight: 600;
}

.choice-resolved {
  padding: 12px 14px;
  background: #f9fafb;
  border-radius: 8px;
  color: #6b7280;
  font-size: 13px;
  font-weight: 500;
}

.choice-resolved-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
</style>
