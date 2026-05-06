<script setup lang="ts">
import { computed, ref, watch } from "vue";
import {
  NAlert,
  NDescriptions,
  NDescriptionsItem,
  NDynamicTags,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
} from "naive-ui";
import CodeEditor from "./CodeEditor.vue";
import type { GraphDiffSummary } from "../utils/graphDiff";
import type { ActionArgField } from "../utils/actionForm";
import {
  createArgPatch,
  createStepPatch,
  createSubflowPatch,
  envTextToObject,
  findActionSpec,
  formatJSON,
  getActionArgFields,
  getTargetOptions,
  nodeAction,
  nodeArgs,
  nodeExecutableName,
  normalizeStringList,
  objectToEnvText,
  readSubflowVars,
  readSubflowWorkflowName,
  readStepTargets,
  replaceStepFromJSON,
  validateActionArgs,
  validateTargets,
} from "../utils/actionForm";
import type { ActionFieldIssue } from "../utils/actionForm";
import type { ActionSpec, WorkflowDefinition, WorkflowNode, WorkflowStep, WorkflowSummary } from "../types/workflow";

const props = defineProps<{
  node: WorkflowNode | null;
  actions: ActionSpec[];
  workflow: WorkflowDefinition | null;
  workflows: WorkflowSummary[];
  diffSummary?: GraphDiffSummary;
}>();

const emit = defineEmits<{
  "update-node": [nodeId: string, patch: Partial<WorkflowNode>];
  "update-workflow": [patch: Partial<WorkflowDefinition>];
}>();

const activeTab = ref("config");
const stepJSON = ref("{}");
const stepJSONError = ref<string | null>(null);
const workflowVarsJSON = ref("{}");
const workflowInventoryJSON = ref("{}");
const workflowVarsJSONError = ref<string | null>(null);
const workflowInventoryJSONError = ref<string | null>(null);
const subflowVarsJSON = ref("{}");
const subflowVarsJSONError = ref<string | null>(null);
const argJSONErrors = ref<Record<string, string>>({});

const canEditStep = computed(() => {
  const type = props.node?.type;
  return type === "action" || type === "condition" || type === "subflow" || type === "handler";
});
const isHandlerNode = computed(() => props.node?.type === "handler");
const currentAction = computed(() => nodeAction(props.node));
const actionSpec = computed(() => findActionSpec(props.actions, currentAction.value));
const argFields = computed(() => getActionArgFields(actionSpec.value, currentAction.value));
const outputs = computed(() => actionSpec.value?.outputs || []);
const targets = computed(() => (isHandlerNode.value ? [] : readStepTargets(props.node?.step)));
const targetOptions = computed(() => getTargetOptions(props.workflow, targets.value).map(({ label, value }) => ({ label, value })));
const targetIssues = computed(() => (isHandlerNode.value ? [] : validateTargets(props.workflow, currentAction.value, targets.value)));
const subflowWorkflowName = computed(() => readSubflowWorkflowName(props.node));
const subflowWorkflowOptions = computed(() => {
  const options = new Map<string, { label: string; value: string }>();
  for (const workflow of props.workflows) {
    if (!workflow.name) continue;
    options.set(workflow.name, {
      label: workflow.version ? `${workflow.name} · ${workflow.version}` : workflow.name,
      value: workflow.name,
    });
  }
  const selected = subflowWorkflowName.value;
  if (selected && !options.has(selected)) {
    options.set(selected, { label: `${selected} · current`, value: selected });
  }
  return [...options.values()].sort((left, right) => left.value.localeCompare(right.value));
});
const subflowWorkflowFeedback = computed(() => {
  if (props.node?.type !== "subflow") return "";
  if (!subflowWorkflowName.value) return "Workflow is required before validation, dry-run, or run.";
  if (subflowWorkflowName.value === props.workflow?.name) return "This subflow points to the current workflow. Confirm recursion is intended.";
  return "Select an existing workflow or enter a workflow name.";
});
const subflowWorkflowStatus = computed(() => {
  if (props.node?.type !== "subflow") return undefined;
  if (!subflowWorkflowName.value) return "error";
  if (subflowWorkflowName.value === props.workflow?.name) return "warning";
  return undefined;
});
const fieldIssues = computed(() => [...targetIssues.value, ...validateActionArgs(actionSpec.value, currentAction.value, nodeArgs(props.node))]);
const fieldIssueMap = computed(() => {
  const map: Record<string, ActionFieldIssue> = {};
  for (const issue of fieldIssues.value) {
    if (!map[issue.field] || issue.severity === "error") map[issue.field] = issue;
  }
  return map;
});
const hasBlockingFieldIssue = computed(() => fieldIssues.value.some((issue) => issue.severity === "error"));
const runtimeHostsJSON = computed(() => formatJSON(props.node?.state?.hosts || {}));
const diffSections = computed(() => props.diffSummary?.sections || []);

const actionOptions = computed(() => {
  const options = props.actions
    .filter((action) => !action.node_type || action.node_type === "action")
    .map((action) => ({
      label: `${action.title} (${action.action})`,
      value: action.action,
    }));
  if (currentAction.value && !options.some((option) => option.value === currentAction.value)) {
    options.unshift({ label: currentAction.value, value: currentAction.value });
  }
  return options;
});

watch(
  () => props.node?.id,
  () => {
    activeTab.value = "config";
    refreshStepJSON();
    refreshSubflowJSON();
    argJSONErrors.value = {};
  },
  { immediate: true },
);

watch(activeTab, (tab) => {
  if (tab === "advanced") refreshStepJSON();
  if (tab === "workflow") refreshWorkflowJSON();
});

watch(
  () => props.workflow?.name,
  () => refreshWorkflowJSON(),
  { immediate: true },
);

function updateLabel(value: string) {
  if (!props.node) return;
  emit("update-node", props.node.id, { label: value });
}

function updateStep(patch: Partial<WorkflowStep>) {
  if (!props.node) return;
  emit("update-node", props.node.id, createStepPatch(props.node, patch));
}

function updateStepName(value: string) {
  updateStep({ name: value.trim() || undefined });
}

function updateAction(value: string) {
  updateStep({ action: value, args: nodeArgs(props.node) || {} });
}

function updateTargets(value: string[]) {
  updateStep({ targets: normalizeStringList(value), target: undefined });
}

function updateWhen(value: string) {
  updateStep({ when: value.trim() || undefined });
}

function updateRetries(value: number | null) {
  updateStep({ retries: typeof value === "number" && Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : 0 });
}

function updateTimeout(value: string) {
  updateStep({ timeout: value.trim() || undefined });
}

function updateExpectVars(value: string[]) {
  updateStep({ expect_vars: normalizeStringList(value) });
}

function updateMustVars(value: string[]) {
  updateStep({ must_vars: normalizeStringList(value) });
}

function updateArg(key: string, value: unknown) {
  if (!props.node) return;
  emit("update-node", props.node.id, createArgPatch(props.node, key, value));
}

function updateStringArg(key: string, value: string) {
  updateArg(key, value === "" ? undefined : value);
}

function updateArrayArg(key: string, value: string[]) {
  updateArg(key, normalizeStringList(value));
}

function updateEnvArg(key: string, value: string) {
  updateArg(key, envTextToObject(value));
}

function updateJSONArg(key: string, value: string) {
  try {
    updateArg(key, JSON.parse(value));
    const nextErrors = { ...argJSONErrors.value };
    delete nextErrors[key];
    argJSONErrors.value = nextErrors;
  } catch (error) {
    argJSONErrors.value = {
      ...argJSONErrors.value,
      [key]: error instanceof Error ? error.message : "Invalid JSON.",
    };
  }
}

function refreshStepJSON() {
  stepJSON.value = formatJSON(isHandlerNode.value ? props.node?.handler : props.node?.step || {});
  stepJSONError.value = null;
}

function updateStepJSON(value: string) {
  stepJSON.value = value;
  if (!props.node) return;
  try {
    emit("update-node", props.node.id, replaceStepFromJSON(props.node, value));
    stepJSONError.value = null;
  } catch (error) {
    stepJSONError.value = error instanceof Error ? error.message : "Invalid step JSON.";
  }
}

function refreshWorkflowJSON() {
  workflowVarsJSON.value = formatJSON(props.workflow?.vars || {});
  workflowInventoryJSON.value = formatJSON(props.workflow?.inventory || {});
  workflowVarsJSONError.value = null;
  workflowInventoryJSONError.value = null;
}

function updateWorkflowVarsJSON(value: string) {
  workflowVarsJSON.value = value;
  try {
    emit("update-workflow", { vars: parseWorkflowObject(value, "Vars") });
    workflowVarsJSONError.value = null;
  } catch (error) {
    workflowVarsJSONError.value = error instanceof Error ? error.message : "Invalid vars JSON.";
  }
}

function updateWorkflowInventoryJSON(value: string) {
  workflowInventoryJSON.value = value;
  try {
    emit("update-workflow", { inventory: parseWorkflowObject(value, "Inventory") });
    workflowInventoryJSONError.value = null;
  } catch (error) {
    workflowInventoryJSONError.value = error instanceof Error ? error.message : "Invalid inventory JSON.";
  }
}

function refreshSubflowJSON() {
  subflowVarsJSON.value = formatJSON(readSubflowVars(props.node));
  subflowVarsJSONError.value = null;
}

function updateSubflowWorkflowName(value: string | null) {
  if (!props.node) return;
  emit(
    "update-node",
    props.node.id,
    createSubflowPatch(props.node, {
      workflow_name: (value || "").trim(),
    }),
  );
}

function updateSubflowVarsJSON(value: string) {
  subflowVarsJSON.value = value;
  if (!props.node) return;
  try {
    emit(
      "update-node",
      props.node.id,
      createSubflowPatch(props.node, {
        vars: parseWorkflowObject(value, "Subflow input vars"),
      }),
    );
    subflowVarsJSONError.value = null;
  } catch (error) {
    subflowVarsJSONError.value = error instanceof Error ? error.message : "Invalid subflow vars JSON.";
  }
}

function parseWorkflowObject(value: string, label: string): Record<string, unknown> {
  const parsed = JSON.parse(value);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error(`${label} JSON must be an object.`);
  }
  return parsed as Record<string, unknown>;
}

function updateApprovalSubjects(value: string[]) {
  if (!props.node) return;
  emit("update-node", props.node.id, {
    approval: {
      ...props.node.approval,
      subjects: normalizeStringList(value),
    },
  });
}

function updateApprovalTimeout(value: string) {
  if (!props.node) return;
  emit("update-node", props.node.id, {
    approval: {
      ...props.node.approval,
      timeout: value.trim() || undefined,
    },
  });
}

function updateApprovalPolicy(value: string) {
  if (!props.node) return;
  emit("update-node", props.node.id, {
    approval: {
      ...props.node.approval,
      on_timeout: value,
    },
  });
}

function updateJoinStrategy(value: string) {
  if (!props.node) return;
  emit("update-node", props.node.id, {
    join: {
      ...props.node.join,
      strategy: value,
    },
  });
}

function updateJoinFailureThreshold(value: number | null) {
  if (!props.node) return;
  emit("update-node", props.node.id, {
    join: {
      ...props.node.join,
      failure_threshold: typeof value === "number" && Number.isFinite(value) ? Math.max(1, Math.trunc(value)) : undefined,
    },
  });
}

function argValue(key: string): unknown {
  return nodeArgs(props.node)?.[key];
}

function stringArgValue(key: string): string {
  const value = argValue(key);
  return value === undefined || value === null ? "" : String(value);
}

function booleanArgValue(key: string): boolean {
  return argValue(key) === true;
}

function arrayArgValue(key: string): string[] {
  return normalizeStringList(argValue(key));
}

function jsonArgValue(key: string): string {
  return formatJSON(argValue(key));
}

function approvalSubjects(): string[] {
  return normalizeStringList(props.node?.approval?.subjects);
}

function renderFieldDescription(field: ActionArgField): string {
  const issue = fieldIssueMap.value[field.key];
  if (issue) return `${issue.message} ${issue.suggestion}`;
  return field.required ? `${field.description || ""}${field.description ? " " : ""}Required.` : field.description || "";
}

function fieldValidationStatus(key: string): "error" | "warning" | undefined {
  const issue = fieldIssueMap.value[key];
  return issue?.severity;
}

function targetFeedback(): string {
  const issue = fieldIssueMap.value.targets;
  if (!issue) return "Select inventory hosts or groups. Custom targets are allowed but should pass dry-run.";
  return `${issue.message} ${issue.suggestion}`;
}
</script>

<template>
  <aside class="property-panel">
    <div class="panel-heading">
      <span>Properties</span>
      <NTag v-if="node" size="small" :bordered="false">{{ node.type }}</NTag>
    </div>

    <NEmpty v-if="!node" description="Select a node" />

    <template v-else>
      <label class="field-block">
        <span>Label</span>
        <NInput :value="node.label || node.id" size="small" @update:value="updateLabel" />
      </label>

      <NTabs v-model:value="activeTab" type="line" animated>
        <NTabPane name="config" tab="Config">
          <NDescriptions class="node-summary" :column="1" size="small" label-placement="left" bordered>
            <NDescriptionsItem label="Node ID">{{ node.id }}</NDescriptionsItem>
            <NDescriptionsItem v-if="nodeAction(node)" label="Action">{{ nodeAction(node) }}</NDescriptionsItem>
            <NDescriptionsItem v-if="node.state?.status" label="Run">
              <NTag size="small" :bordered="false">{{ node.state.status }}</NTag>
            </NDescriptionsItem>
          </NDescriptions>

          <NForm v-if="node.type === 'subflow'" class="property-form" label-placement="top" size="small">
            <NFormItem label="Workflow" :validation-status="subflowWorkflowStatus" :feedback="subflowWorkflowFeedback">
              <NSelect
                :value="subflowWorkflowName"
                :options="subflowWorkflowOptions"
                filterable
                tag
                clearable
                @update:value="updateSubflowWorkflowName"
              />
            </NFormItem>

            <NFormItem label="Input vars">
              <CodeEditor
                :model-value="subflowVarsJSON"
                language="json"
                height="220px"
                placeholder="{ &quot;backup_id&quot;: &quot;${vars.backup_id}&quot; }"
                @update:model-value="updateSubflowVarsJSON"
              />
              <NAlert v-if="subflowVarsJSONError" class="editor-alert" type="error" :bordered="false">
                {{ subflowVarsJSONError }}
              </NAlert>
            </NFormItem>
          </NForm>

          <NForm v-else-if="canEditStep" class="property-form" label-placement="top" size="small">
            <NFormItem :label="isHandlerNode ? 'Handler name' : 'Step name'">
              <NInput :value="nodeExecutableName(node)" @update:value="updateStepName" />
            </NFormItem>

            <NFormItem label="Action">
              <NSelect :value="currentAction" :options="actionOptions" filterable @update:value="updateAction" />
            </NFormItem>

            <NAlert v-if="actionSpec?.description" type="info" :bordered="false" class="inline-alert">
              {{ actionSpec.description }}
            </NAlert>

            <NAlert v-if="fieldIssues.length" :type="hasBlockingFieldIssue ? 'error' : 'warning'" :bordered="false" class="inline-alert">
              {{ fieldIssues.length }} field issue{{ fieldIssues.length > 1 ? "s" : "" }} found. Fix highlighted fields before running.
            </NAlert>

            <NFormItem v-if="!isHandlerNode" label="Targets" :validation-status="fieldValidationStatus('targets')" :feedback="targetFeedback()">
              <NSelect
                :value="targets"
                :options="targetOptions"
                multiple
                filterable
                tag
                clearable
                @update:value="updateTargets"
              />
            </NFormItem>

            <NFormItem v-if="!isHandlerNode" label="When">
              <NInput :value="node.step?.when || ''" placeholder="${environment} == 'staging'" @update:value="updateWhen" />
            </NFormItem>

            <div v-if="!isHandlerNode" class="form-grid">
              <NFormItem label="Retries">
                <NInputNumber :value="node.step?.retries || 0" :min="0" @update:value="updateRetries" />
              </NFormItem>
              <NFormItem label="Timeout">
                <NInput :value="node.step?.timeout || ''" placeholder="5m" @update:value="updateTimeout" />
              </NFormItem>
            </div>

            <NFormItem v-if="!isHandlerNode" label="Expect vars">
              <NDynamicTags :value="node.step?.expect_vars || []" @update:value="updateExpectVars" />
            </NFormItem>

            <NFormItem v-if="!isHandlerNode" label="Must vars">
              <NDynamicTags :value="node.step?.must_vars || []" @update:value="updateMustVars" />
            </NFormItem>

            <div class="section-title">Action arguments</div>

            <NAlert v-if="!argFields.length" type="warning" :bordered="false" class="inline-alert">
              No action schema is available. Use Advanced JSON for custom arguments.
            </NAlert>

            <NFormItem
              v-for="field in argFields"
              :key="field.key"
              :label="field.title"
              :validation-status="fieldValidationStatus(field.key)"
              :feedback="renderFieldDescription(field)"
            >
              <template v-if="field.kind === 'boolean'">
                <NSwitch :value="booleanArgValue(field.key)" @update:value="updateArg(field.key, $event)" />
              </template>

              <template v-else-if="field.kind === 'string-array'">
                <NDynamicTags :value="arrayArgValue(field.key)" @update:value="updateArrayArg(field.key, $event)" />
              </template>

              <template v-else-if="field.kind === 'env'">
                <NInput
                  :value="objectToEnvText(argValue(field.key))"
                  type="textarea"
                  :autosize="{ minRows: 3, maxRows: 8 }"
                  placeholder="KEY=value"
                  @update:value="updateEnvArg(field.key, $event)"
                />
              </template>

              <template v-else-if="field.kind === 'json'">
                <CodeEditor
                  :model-value="jsonArgValue(field.key)"
                  language="json"
                  height="180px"
                  @update:model-value="updateJSONArg(field.key, $event)"
                />
                <NAlert v-if="argJSONErrors[field.key]" type="error" :bordered="false" class="editor-alert">
                  {{ argJSONErrors[field.key] }}
                </NAlert>
              </template>

              <template v-else>
                <NInput
                  :value="stringArgValue(field.key)"
                  :type="field.kind === 'multiline' ? 'textarea' : 'text'"
                  :autosize="field.kind === 'multiline' ? { minRows: 3, maxRows: 10 } : undefined"
                  @update:value="updateStringArg(field.key, $event)"
                />
              </template>
            </NFormItem>

            <div v-if="outputs.length" class="section-title">Outputs</div>
            <div v-if="outputs.length" class="output-list">
              <NTag v-for="output in outputs" :key="output.name" size="small" :bordered="false">
                {{ output.name }}<span v-if="output.type">: {{ output.type }}</span>
              </NTag>
            </div>
          </NForm>

          <NForm v-else-if="node.type === 'manual_approval'" class="property-form" label-placement="top" size="small">
            <NFormItem label="Approvers">
              <NDynamicTags :value="approvalSubjects()" @update:value="updateApprovalSubjects" />
            </NFormItem>
            <NFormItem label="Timeout">
              <NInput :value="node.approval?.timeout || ''" placeholder="30m" @update:value="updateApprovalTimeout" />
            </NFormItem>
            <NFormItem label="Timeout policy">
              <NSelect
                :value="node.approval?.on_timeout || 'reject'"
                :options="[
                  { label: 'Reject', value: 'reject' },
                  { label: 'Approve', value: 'approve' },
                  { label: 'Continue', value: 'continue' },
                ]"
                @update:value="updateApprovalPolicy"
              />
            </NFormItem>
          </NForm>

          <NForm v-else-if="node.type === 'join'" class="property-form" label-placement="top" size="small">
            <NFormItem label="Join strategy">
              <NSelect
                :value="node.join?.strategy || 'all_success'"
                :options="[
                  { label: 'All success', value: 'all_success' },
                  { label: 'Any success', value: 'any_success' },
                  { label: 'Always', value: 'always' },
                  { label: 'Failure threshold', value: 'failure_threshold' },
                ]"
                @update:value="updateJoinStrategy"
              />
            </NFormItem>
            <NFormItem v-if="node.join?.strategy === 'failure_threshold'" label="Failure threshold">
              <NInputNumber :value="node.join?.failure_threshold || 1" :min="1" @update:value="updateJoinFailureThreshold" />
            </NFormItem>
          </NForm>
        </NTabPane>

        <NTabPane name="run" tab="Run state">
          <NDescriptions v-if="node.state" class="node-summary" :column="1" size="small" label-placement="left" bordered>
            <NDescriptionsItem label="Status">
              <NTag size="small" :bordered="false">{{ node.state.status || "unknown" }}</NTag>
            </NDescriptionsItem>
            <NDescriptionsItem v-if="node.state.message" label="Message">{{ node.state.message }}</NDescriptionsItem>
            <NDescriptionsItem v-if="node.state.started_at" label="Started">{{ node.state.started_at }}</NDescriptionsItem>
            <NDescriptionsItem v-if="node.state.finished_at" label="Finished">{{ node.state.finished_at }}</NDescriptionsItem>
          </NDescriptions>
          <NEmpty v-else description="No run state for this node" />

          <template v-if="node.state?.hosts">
            <div class="modal-tab-heading workflow-editor-heading">
              <NTag size="small" :bordered="false">Host results</NTag>
              <small>runtime overlay</small>
            </div>
            <CodeEditor :model-value="runtimeHostsJSON" language="json" height="200px" readonly />
          </template>
        </NTabPane>

        <NTabPane name="diff" tab="YAML diff">
          <div v-if="diffSections.length" class="diff-section-list">
            <div
              v-for="section in diffSections"
              :key="section.kind"
              class="diff-section"
              :class="{ 'is-changed': section.changed }"
            >
              <div class="diff-section-heading">
                <NTag size="small" :bordered="false">{{ section.kind }}</NTag>
                <strong>{{ section.title }}</strong>
                <small>{{ section.changed ? `${section.paths.length} change${section.paths.length === 1 ? "" : "s"}` : "unchanged" }}</small>
              </div>
              <ul v-if="section.paths.length">
                <li v-for="path in section.paths" :key="path"><code>{{ path }}</code></li>
              </ul>
            </div>
          </div>
          <NEmpty v-else description="No baseline diff available" />
        </NTabPane>

        <NTabPane name="advanced" tab="Advanced">
          <template v-if="canEditStep">
            <div class="modal-tab-heading">
              <NTag size="small" :bordered="false">{{ isHandlerNode ? "Handler JSON" : "Step JSON" }}</NTag>
              <small>Valid JSON is applied immediately.</small>
            </div>
            <CodeEditor
              :model-value="stepJSON"
              language="json"
              height="360px"
              :placeholder="isHandlerNode ? '{ &quot;name&quot;: &quot;notify&quot;, &quot;action&quot;: &quot;cmd.run&quot;, &quot;args&quot;: {} }' : '{ &quot;name&quot;: &quot;step&quot;, &quot;action&quot;: &quot;cmd.run&quot;, &quot;args&quot;: {} }'"
              @update:model-value="updateStepJSON"
            />
            <NAlert v-if="stepJSONError" class="editor-alert" type="error" :bordered="false">
              {{ stepJSONError }}
            </NAlert>
          </template>
          <NAlert v-else type="info" :bordered="false">
            This node does not have step JSON.
          </NAlert>
        </NTabPane>

        <NTabPane name="workflow" tab="Workflow">
          <div class="modal-tab-heading">
            <NTag size="small" :bordered="false">Workflow vars</NTag>
            <small>JSON object, applied immediately.</small>
          </div>
          <CodeEditor
            :model-value="workflowVarsJSON"
            language="json"
            height="180px"
            placeholder="{ &quot;service&quot;: &quot;billing-api&quot; }"
            @update:model-value="updateWorkflowVarsJSON"
          />
          <NAlert v-if="workflowVarsJSONError" class="editor-alert" type="error" :bordered="false">
            {{ workflowVarsJSONError }}
          </NAlert>

          <div class="modal-tab-heading workflow-editor-heading">
            <NTag size="small" :bordered="false">Inventory</NTag>
            <small>hosts / groups / vars</small>
          </div>
          <CodeEditor
            :model-value="workflowInventoryJSON"
            language="json"
            height="260px"
            placeholder="{ &quot;hosts&quot;: {}, &quot;groups&quot;: {} }"
            @update:model-value="updateWorkflowInventoryJSON"
          />
          <NAlert v-if="workflowInventoryJSONError" class="editor-alert" type="error" :bordered="false">
            {{ workflowInventoryJSONError }}
          </NAlert>
        </NTabPane>
      </NTabs>
    </template>
  </aside>
</template>
