<script setup>
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useAppStore } from "../store";
import {
  selectActiveProjection,
  selectApprovalDock,
  selectRuntimeBusy,
  selectRuntimeStatus,
  selectTimelineRows,
} from "../events/agentEventProjection";
import { buildCodexProcessTranscript } from "../lib/codexProcessTranscript";
import ChatComposerDock from "../components/chat/ChatComposerDock.vue";
import ChatProcessFold from "../components/chat/ChatProcessFold.vue";
import IncidentEvidenceList from "../components/incidents/IncidentEvidenceList.vue";
import IncidentHypothesisList from "../components/incidents/IncidentHypothesisList.vue";
import IncidentImpactStrip from "../components/incidents/IncidentImpactStrip.vue";
import IncidentPostmortemPreview from "../components/incidents/IncidentPostmortemPreview.vue";
import IncidentRunbookPanel from "../components/incidents/IncidentRunbookPanel.vue";
import IncidentSeverityBadge from "../components/incidents/IncidentSeverityBadge.vue";
import IncidentStatusBadge from "../components/incidents/IncidentStatusBadge.vue";
import IncidentTimeline from "../components/incidents/IncidentTimeline.vue";
import "./erpSrePages.css";

const route = useRoute();
const store = useAppStore();
const composerDraft = ref("");
const approvalBusy = ref(false);

const incidentId = computed(() => String(route.params.incidentId || ""));
const incident = computed(() => store.incidents.active || {});
const incidentTitle = computed(() => incident.value.title || incident.value.name || `事故 ${incidentId.value}`);
const incidentStatus = computed(() => incident.value.status || "unknown");
const incidentSeverity = computed(() => incident.value.severity || incident.value.sev || "SEV 待定");
const incidentEnvironment = computed(() => incident.value.environment || incident.value.env || "环境待定");
const businessCapability = computed(() => incident.value.businessCapability || incident.value.capability || "业务能力待定");
const incidentSummary = computed(() => incident.value.summary || "暂无事故摘要");
const hypotheses = computed(() => Array.isArray(incident.value.hypotheses) ? incident.value.hypotheses : []);
const evidence = computed(() => Array.isArray(incident.value.evidence) ? incident.value.evidence : []);
const postmortem = computed(() => incident.value.postmortem || {});
const corootEvidence = computed(() =>
  evidence.value.filter((item) => String(item?.source || "").toLowerCase().includes("coroot")),
);

const activeProjectionSessionId = computed(() =>
  incident.value.sessionId || store.activeSessionId || store.snapshot.sessionId || "",
);
const projection = computed(() => selectActiveProjection(store.agentEventState, activeProjectionSessionId.value));
const runtimeStatus = computed(() => selectRuntimeStatus(store.agentEventState, activeProjectionSessionId.value));
const runtimeBusy = computed(() => selectRuntimeBusy(store.agentEventState, activeProjectionSessionId.value));
const approvalDock = computed(() => selectApprovalDock(store.agentEventState, activeProjectionSessionId.value));
const activeApproval = computed(() => approvalDock.value[0] || null);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

const graphNeighbors = computed(() => asArray(store.opsgraph.neighborhood?.neighbors || store.opsgraph.neighborhood?.items));
const impactCapabilities = computed(() => asArray(store.opsgraph.businessImpact?.capabilities));
const impactTenants = computed(() => asArray(store.opsgraph.businessImpact?.tenants));
const runbookMatches = computed(() => asArray(store.runbooks.matches));
const runbookInstances = computed(() => asArray(store.runbooks.instances));

const processItems = computed(() => {
  const currentTurnId = compactText(projection.value?.currentTurnId);
  const groups = projection.value?.processGroups || {};
  if (currentTurnId && Array.isArray(groups[currentTurnId]) && groups[currentTurnId].length) {
    return groups[currentTurnId];
  }
  const groupedItems = Object.values(groups).flatMap((items) => asArray(items));
  if (groupedItems.length) return groupedItems;
  return selectTimelineRows(store.agentEventState, activeProjectionSessionId.value)
    .filter((row) => !["turn", "assistant_final"].includes(row?.kind))
    .map((row) => ({
      ...row,
      text: row.text || row.title || row.summary,
      displayKind: row.displayKind || row.kind,
    }));
});

const processTurn = computed(() => {
  const status = runtimeStatus.value === "blocked"
    ? "blocked"
    : runtimeBusy.value
      ? "running"
      : "completed";
  const turnId = compactText(projection.value?.currentTurnId || "incident-process");
  return {
    id: turnId,
    active: runtimeBusy.value,
    processLabel: "事故处理过程",
    processTranscript: buildCodexProcessTranscript({
      turnId,
      status,
      active: runtimeBusy.value,
      processItems: processItems.value,
      approval: activeApproval.value,
      modelRunning: runtimeBusy.value,
    }),
  };
});

function contextEntityId() {
  return (
    incident.value.entityId ||
    incident.value.serviceId ||
    incident.value.resourceId ||
    incident.value.service?.id ||
    incident.value.service?.name ||
    incident.value.id ||
    incidentId.value
  );
}

async function loadIncidentContext() {
  const id = incidentId.value;
  if (!id) return;
  await store.loadIncident(id);
  const entityId = contextEntityId();
  await Promise.allSettled([
    entityId ? store.loadOpsGraphNeighborhood(entityId, { incidentId: id }) : Promise.resolve(),
    entityId ? store.loadOpsGraphBusinessImpact(entityId) : Promise.resolve(),
    store.matchRunbooks({ incidentId: id, title: incidentTitle.value, capability: businessCapability.value }),
    store.loadRunbookInstances({ incidentId: id }),
  ]);
}

function approvalId(approval = {}) {
  return compactText(approval.approvalId || approval.id || approval.requestId);
}

async function decideApproval(decision) {
  const approval = activeApproval.value;
  const id = approvalId(approval);
  if (!id || approvalBusy.value) return;
  approvalBusy.value = true;
  try {
    await store.submitApprovalDecision(id, decision);
  } finally {
    approvalBusy.value = false;
  }
}

onMounted(() => {
  void loadIncidentContext();
});

watch(incidentId, (next, previous) => {
  if (next && next !== previous) {
    void loadIncidentContext();
  }
});
</script>

<template>
  <section class="erp-sre-page incident-workbench-page">
    <div class="erp-sre-shell incident-workbench-shell">
      <header class="erp-sre-heading incident-header">
        <div>
          <p class="erp-sre-kicker">Incident Detail</p>
          <h2>{{ incidentTitle }}</h2>
          <div class="erp-sre-metadata">
            <IncidentStatusBadge :status="incidentStatus" />
            <IncidentSeverityBadge :severity="incidentSeverity" />
            <span class="erp-sre-pill">{{ incidentEnvironment }}</span>
            <span class="erp-sre-pill">{{ businessCapability }}</span>
          </div>
        </div>
        <div class="erp-sre-actions">
          <RouterLink class="erp-sre-link-secondary" to="/incidents">返回事故列表</RouterLink>
          <RouterLink class="erp-sre-link" to="/opsgraph">查看 ERP 图谱</RouterLink>
        </div>
      </header>

      <IncidentImpactStrip :incident="incident" :impact="store.opsgraph.businessImpact || {}" />

      <div class="incident-workbench-layout">
        <main class="incident-main-column">
          <section class="erp-sre-panel">
            <h3>事故摘要</h3>
            <p>{{ incidentSummary }}</p>
          </section>

          <section class="erp-sre-panel">
            <h3>Hypothesis 排名</h3>
            <IncidentHypothesisList :hypotheses="hypotheses" />
          </section>

          <section class="erp-sre-panel">
            <h3>过程流</h3>
            <ChatProcessFold :turn="processTurn" />
          </section>

          <section class="erp-sre-panel">
            <h3>证据时间线</h3>
            <IncidentEvidenceList :evidence="evidence" />
          </section>

          <section class="erp-sre-panel">
            <h3>复盘草稿</h3>
            <IncidentPostmortemPreview :postmortem="postmortem" />
          </section>
        </main>

        <details class="incident-context-drawer" data-testid="incident-context-drawer" open>
          <summary>上下文面板</summary>
          <aside class="incident-sidebar">
            <section class="erp-sre-panel">
              <h3>ERP 图谱邻域</h3>
              <div v-if="graphNeighbors.length" class="incident-compact-list">
                <div v-for="item in graphNeighbors" :key="item.id || item.name">
                  <strong>{{ item.name || item.id }}</strong>
                  <span>{{ item.relation || item.status || "neighbor" }}</span>
                </div>
              </div>
              <p v-else>暂无邻域</p>
            </section>

            <section class="erp-sre-panel">
              <h3>业务影响</h3>
              <div class="incident-compact-list">
                <div v-for="item in impactCapabilities" :key="item.name || item.id">
                  <strong>{{ item.name || item.id }}</strong>
                  <span>{{ item.impact || item.status || "impact" }}</span>
                </div>
                <div v-for="item in impactTenants" :key="item.id || item.name">
                  <strong>{{ item.name || item.id }}</strong>
                  <span>{{ item.impact || item.status || "tenant" }}</span>
                </div>
                <p v-if="!impactCapabilities.length && !impactTenants.length">暂无业务影响</p>
              </div>
            </section>

            <section class="erp-sre-panel">
              <h3>Coroot 证据</h3>
              <IncidentTimeline :items="corootEvidence" empty-text="暂无 Coroot 证据" />
            </section>

            <section class="erp-sre-panel">
              <h3>Runbook 状态</h3>
              <IncidentRunbookPanel :matches="runbookMatches" :instances="runbookInstances" />
            </section>

            <section class="erp-sre-panel" data-testid="incident-sidebar-approval">
              <h3>等待审批</h3>
              <template v-if="activeApproval">
                <strong>{{ activeApproval.title || activeApproval.reason || "待确认操作" }}</strong>
                <code v-if="activeApproval.command">{{ activeApproval.command }}</code>
                <p v-if="activeApproval.reason">{{ activeApproval.reason }}</p>
              </template>
              <p v-else>暂无待审批</p>
            </section>
          </aside>
        </details>
      </div>

      <ChatComposerDock
        v-model="composerDraft"
        placeholder="在事故上下文中继续补充说明"
        allow-follow-up
        is-docked-bottom
        :show-composer="!activeApproval"
        :disabled="approvalBusy"
        :busy="approvalBusy"
      >
        <template #approval>
          <div v-if="activeApproval" class="codex-approval-inline" data-testid="codex-approval-inline">
            <div class="codex-approval-question">{{ activeApproval.title || "是否允许执行此操作？" }}</div>
            <div v-if="activeApproval.command" class="codex-approval-command">
              <code>{{ activeApproval.command }}</code>
            </div>
            <p v-if="activeApproval.reason">{{ activeApproval.reason }}</p>
            <div class="codex-approval-options">
              <button type="button" :disabled="approvalBusy" @click="decideApproval('accept')">同意</button>
              <button type="button" :disabled="approvalBusy" @click="decideApproval('accept_session')">本次会话同意</button>
              <button type="button" :disabled="approvalBusy" @click="decideApproval('reject')">拒绝</button>
            </div>
          </div>
        </template>
      </ChatComposerDock>
    </div>
  </section>
</template>

<style scoped>
.incident-workbench-shell {
  padding-bottom: 10px;
}

.incident-header h2 {
  max-width: 760px;
}

.incident-workbench-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 0.42fr);
  gap: 14px;
  align-items: start;
}

.incident-main-column,
.incident-sidebar {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 14px;
}

.incident-context-drawer {
  min-width: 0;
}

.incident-context-drawer > summary {
  display: none;
}

.incident-compact-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.incident-compact-list > div {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 8px;
  background: #f8fafc;
}

.incident-compact-list strong {
  min-width: 0;
  overflow: hidden;
  color: #0f172a;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.incident-compact-list span {
  flex: 0 0 auto;
  color: #64748b;
  font-size: 12px;
}

[data-testid="incident-sidebar-approval"] strong {
  display: block;
  color: #0f172a;
  font-size: 13px;
}

[data-testid="incident-sidebar-approval"] code {
  display: block;
  margin-top: 8px;
  overflow-wrap: anywhere;
  border-radius: 6px;
  background: #f8fafc;
  padding: 7px 8px;
  color: #334155;
  font-size: 12px;
}

.codex-approval-inline {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid #bbf7d0;
  border-radius: 8px;
  background: #f0fdf4;
}

.codex-approval-question {
  color: #14532d;
  font-weight: 800;
}

.codex-approval-command code {
  display: block;
  overflow-wrap: anywhere;
  border-radius: 6px;
  background: #dcfce7;
  padding: 8px;
  color: #14532d;
  font-size: 12px;
}

.codex-approval-inline p {
  margin: 0;
  color: #166534;
  font-size: 13px;
}

.codex-approval-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.codex-approval-options button {
  min-height: 32px;
  padding: 0 12px;
  border: 1px solid #86efac;
  border-radius: 8px;
  background: #fff;
  color: #14532d;
  font-weight: 800;
  cursor: pointer;
}

.codex-approval-options button:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

@media (max-width: 980px) {
  .incident-workbench-layout {
    grid-template-columns: 1fr;
  }

  .incident-context-drawer {
    order: 2;
  }

  .incident-context-drawer > summary {
    display: flex;
    align-items: center;
    min-height: 38px;
    padding: 0 12px;
    border: 1px solid #cbd5e1;
    border-radius: 8px;
    background: #fff;
    color: #0f172a;
    font-weight: 800;
    cursor: pointer;
  }

  .incident-sidebar {
    margin-top: 10px;
  }
}
</style>
