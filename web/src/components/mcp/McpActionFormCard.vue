<script setup>
import { computed, reactive } from "vue";
import { normalizeMcpUiAction, normalizeMcpUiActions } from "../../lib/mcpUiCardModel";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  embedded: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["action", "detail", "refresh"]);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

const resolvedActions = computed(() => {
  const rawActions = props.card?.actions?.length
    ? props.card.actions
    : props.card?.action
      ? [props.card.action]
      : [];
  return normalizeMcpUiActions(asArray(rawActions), {
    uiKind: "form_panel",
  });
});

const primaryAction = computed(() => {
  return resolvedActions.value.find((action) => action.intent !== "refresh" && !action.disabled)
    || normalizeMcpUiAction(props.card?.action || {}, 0, "form_panel");
});

const refreshAction = computed(() => {
  return resolvedActions.value.find((action) => action.intent === "refresh" && !action.disabled) || null;
});

const payloadSchema = computed(() => {
  return asObject(
    primaryAction.value?.payloadSchema
      || props.card?.payloadSchema
      || props.card?.payload_schema
      || props.card?.form,
  );
});

const fields = computed(() => {
  return asArray(payloadSchema.value.fields || props.card?.fields).map((field, index) => {
    const source = asObject(field);
    const type = compactText(source.type || "text").toLowerCase();
    return {
      id: compactText(source.id || source.name || `field-${index + 1}`),
      name: compactText(source.name || source.id || `field_${index + 1}`),
      label: compactText(source.label || source.name || `字段 ${index + 1}`),
      type: ["text", "select", "textarea", "checkbox"].includes(type) ? type : "text",
      placeholder: compactText(source.placeholder || ""),
      options: asArray(source.options).map((option, optionIndex) => {
        const item = asObject(option);
        return {
          id: compactText(item.id || `option-${optionIndex + 1}`),
          label: compactText(item.label || item.name || item.value || `选项 ${optionIndex + 1}`),
          value: item.value ?? item.label ?? `${optionIndex + 1}`,
        };
      }),
      defaultValue: source.defaultValue ?? source.default ?? (type === "checkbox" ? false : ""),
    };
  });
});

const confirmationText = computed(() => {
  return compactText(
    payloadSchema.value.confirmDescription
      || payloadSchema.value.confirm_description
      || primaryAction.value?.confirmText
      || props.card?.confirmText
      || props.card?.risk,
  );
});

const formState = reactive({});

function ensureFieldValue(field) {
  if (Object.prototype.hasOwnProperty.call(formState, field.name)) return;
  formState[field.name] = field.defaultValue;
}

function fieldValue(field) {
  ensureFieldValue(field);
  return formState[field.name];
}

function fieldChecked(field) {
  ensureFieldValue(field);
  return Boolean(formState[field.name]);
}

function emitAction() {
  emit("action", {
    ...primaryAction.value,
    formValues: Object.fromEntries(fields.value.map((field) => [field.name, formState[field.name]])),
  });
}

function emitDetail() {
  emit("detail", props.card);
}

function emitRefresh() {
  emit("refresh", refreshAction.value || { intent: "refresh", cardId: props.card?.id || "" });
}
</script>

<template>
  <section
    class="mcp-action-form-card"
    :class="{ embedded }"
    data-testid="mcp-action-form-card"
  >
    <header class="form-header">
      <div class="form-copy">
        <p class="form-eyebrow">结构化表单</p>
        <h4 class="form-title">{{ card.title || "执行表单" }}</h4>
        <p v-if="card.summary" class="form-summary">{{ card.summary }}</p>
      </div>
      <span class="form-permission">{{ primaryAction.permissionPath || props.card?.permissionPath || "未声明权限路径" }}</span>
    </header>

    <p
      v-if="confirmationText"
      class="form-confirmation"
      data-testid="mcp-form-confirmation"
    >
      {{ confirmationText }}
    </p>

    <div
      v-if="!fields.length"
      class="form-empty-state"
      data-testid="mcp-form-empty-state"
    >
      当前没有可填写的字段，建议先查看详情确认参数来源。
    </div>

    <form
      v-else
      class="form-grid"
      @submit.prevent="emitAction"
    >
      <label
        v-for="field in fields"
        :key="field.id"
        class="form-field"
        :data-testid="`mcp-form-field-${field.name}`"
      >
        <span class="field-label">{{ field.label }}</span>
        <input
          v-if="field.type === 'text'"
          :value="fieldValue(field)"
          type="text"
          :placeholder="field.placeholder"
          @input="formState[field.name] = $event.target.value"
        >
        <select
          v-else-if="field.type === 'select'"
          :value="fieldValue(field)"
          @change="formState[field.name] = $event.target.value"
        >
          <option
            v-for="option in field.options"
            :key="option.id"
            :value="option.value"
          >
            {{ option.label }}
          </option>
        </select>
        <textarea
          v-else-if="field.type === 'textarea'"
          :value="fieldValue(field)"
          :placeholder="field.placeholder"
          rows="3"
          @input="formState[field.name] = $event.target.value"
        />
        <input
          v-else
          :checked="fieldChecked(field)"
          type="checkbox"
          @change="formState[field.name] = $event.target.checked"
        >
      </label>
    </form>

    <footer class="form-actions">
      <button
        type="button"
        class="secondary-btn"
        data-testid="mcp-action-form-detail"
        @click="emitDetail"
      >
        查看详情
      </button>
      <button
        type="button"
        class="secondary-btn"
        data-testid="mcp-action-form-refresh"
        @click="emitRefresh"
      >
        {{ refreshAction?.label || "刷新上下文" }}
      </button>
      <button
        type="button"
        class="primary-btn"
        data-testid="mcp-action-form-submit"
        @click="emitAction"
      >
        {{ primaryAction.label || "提交" }}
      </button>
    </footer>
  </section>
</template>

<style scoped>
.mcp-action-form-card {
  display: grid;
  gap: 12px;
  padding: 14px;
  border: 1px solid rgba(15, 23, 42, 0.09);
  border-radius: 16px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(241, 245, 249, 0.98));
}

.mcp-action-form-card.embedded {
  padding: 0;
  border: none;
  background: transparent;
}

.form-header,
.form-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 10px;
}

.form-copy {
  display: grid;
  gap: 4px;
}

.form-eyebrow {
  margin: 0;
  font-size: 11px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #475569;
}

.form-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #0f172a;
}

.form-summary,
.form-confirmation {
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
  color: #334155;
}

.form-permission {
  align-self: flex-start;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(226, 232, 240, 0.88);
  font-size: 12px;
  color: #0f172a;
}

.form-grid {
  display: grid;
  gap: 12px;
}

.form-field {
  display: grid;
  gap: 6px;
}

.field-label {
  font-size: 12px;
  color: #475569;
}

.form-field input[type="text"],
.form-field select,
.form-field textarea {
  width: 100%;
  min-height: 40px;
  border: 1px solid rgba(148, 163, 184, 0.45);
  border-radius: 12px;
  padding: 10px 12px;
  background: rgba(255, 255, 255, 0.96);
  color: #0f172a;
  font-size: 13px;
}

.form-field input[type="checkbox"] {
  width: 18px;
  height: 18px;
}

.form-empty-state {
  padding: 12px;
  border-radius: 14px;
  background: rgba(241, 245, 249, 0.95);
  color: #475569;
  font-size: 13px;
}

.primary-btn,
.secondary-btn {
  border: none;
  border-radius: 12px;
  padding: 8px 12px;
  font-size: 13px;
  cursor: pointer;
}

.primary-btn {
  background: #0f172a;
  color: white;
}

.secondary-btn {
  background: rgba(226, 232, 240, 0.86);
  color: #0f172a;
}
</style>
