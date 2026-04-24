<script setup>
import { computed, defineAsyncComponent } from "vue";
import MessageCard from "./MessageCard.vue";
import PlanCard from "./PlanCard.vue";
import AuthCard from "./AuthCard.vue";
import ThinkingCard from "./ThinkingCard.vue";
import NoticeCard from "./NoticeCard.vue";
import ErrorCard from "./ErrorCard.vue";
import ChoiceCard from "./ChoiceCard.vue";
import ResultSummaryCard from "./ResultSummaryCard.vue";
import ProcessLineCard from "./ProcessLineCard.vue";

const TerminalCard = defineAsyncComponent(() => import("./TerminalCard.vue"));
const CodeCard = defineAsyncComponent(() => import("./CodeCard.vue"));

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  sessionKind: {
    type: String,
    default: "",
  },
  isOverlay: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["approval", "choice", "retry", "refresh"]);

function handleApproval(payload) {
  emit("approval", payload);
}

function handleChoice(payload) {
  emit("choice", payload);
}

/* Backward-compatible type detection for StepCard */
const isTerminal = computed(() => {
  return (
    props.card.type === "CommandCard" ||
    (props.card.type === "StepCard" && !!props.card.command)
  );
});

const isCode = computed(() => {
  return (
    props.card.type === "FilePreviewCard" ||
    props.card.type === "FileChangeCard" ||
    (props.card.type === "StepCard" && !props.card.command && props.card.changes?.length > 0)
  );
});

const isMessage = computed(() => {
  return (
    props.card.type === "MessageCard" ||
    props.card.type === "UserMessageCard" ||
    props.card.type === "AssistantMessageCard" ||
    (props.card.type === "StepCard" && !isTerminal.value && !isCode.value)
  );
});
</script>

<template>
  <div class="card-root" :class="{ 'is-workspace': sessionKind === 'workspace' }">
    <!-- ThinkingCard -->
    <template v-if="card.type === 'ThinkingCard'">
      <ThinkingCard :card="card" />
    </template>

    <template v-else-if="isMessage">
      <MessageCard :card="card" />
    </template>

    <!-- PlanCard -->
    <template v-else-if="card.type === 'PlanCard'">
      <PlanCard :card="card" :session-kind="sessionKind" />
    </template>

    <template v-else-if="card.type === 'ProcessLineCard'">
      <ProcessLineCard :card="card" :session-kind="sessionKind" />
    </template>

    <!-- Approval cards -->
    <template v-else-if="card.type === 'CommandApprovalCard' || card.type === 'FileChangeApprovalCard'">
      <AuthCard :card="card" :is-overlay="isOverlay" @approval="handleApproval" />
    </template>

    <!-- ChoiceCard -->
    <template v-else-if="card.type === 'ChoiceCard'">
      <ChoiceCard :card="card" :session-kind="sessionKind" @choice="handleChoice" />
    </template>

    <!-- Terminal / Command card -->
    <template v-else-if="isTerminal">
      <TerminalCard :card="card" />
    </template>

    <!-- Code / File change card -->
    <template v-else-if="isCode">
      <CodeCard :card="card" />
    </template>

    <!-- ResultSummaryCard -->
    <template v-else-if="card.type === 'ResultSummaryCard'">
      <ResultSummaryCard :card="card" :session-kind="sessionKind" />
    </template>

    <!-- NoticeCard -->
    <template v-else-if="card.type === 'NoticeCard'">
      <NoticeCard :card="card" :session-kind="sessionKind" />
    </template>

    <!-- ErrorCard -->
    <template v-else-if="card.type === 'ErrorCard'">
      <ErrorCard :card="card" @retry="emit('retry', $event)" @refresh="emit('refresh')" />
    </template>

    <!-- Fallback generic renderer -->
    <template v-else>
      <div class="generic-card">
        <p v-if="card.text">{{ card.text }}</p>
        <pre v-if="card.output" class="mono">{{ card.output }}</pre>
      </div>
    </template>
  </div>
</template>

<style scoped>
.card-root {
  width: 100%;
}

.card-root.is-workspace {
  width: 100%;
}

.generic-card {
  margin-top: 2px;
  margin-left: 36px;
  padding: 10px;
  background: white;
  border-radius: 10px;
  border: 1px dashed #cbd5e1;
  font-size: 12.5px;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
