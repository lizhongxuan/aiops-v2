<script setup lang="ts">
import { computed, ref } from "vue";
import { Search } from "lucide-vue-next";
import type { ActionSpec } from "../types/workflow";
import { filterActionCatalog } from "../utils/actionCatalog";
import { filterControlNodes, type ControlNodeType } from "../utils/graphEditing";

const props = defineProps<{
  actions: ActionSpec[];
}>();

const emit = defineEmits<{
  "add-action": [action: string];
  "add-control-node": [type: ControlNodeType];
}>();

const query = ref("");
const groups = computed(() => filterActionCatalog(props.actions, query.value));
const controlNodes = computed(() => filterControlNodes(query.value));
const visibleCount = computed(() => groups.value.reduce((total, group) => total + group.actions.length, 0) + controlNodes.value.length);

function riskClass(action: ActionSpec) {
  return `risk-${(action.risk || "medium").replace(/[^a-z0-9_-]/gi, "_")}`;
}

function riskLabel(action: ActionSpec) {
  return action.risk || "medium";
}

function handleDragStart(event: DragEvent, action: ActionSpec) {
  event.dataTransfer?.setData("application/runner-action", action.action);
  event.dataTransfer?.setData("text/plain", action.action);
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "copy";
  }
}

function handleControlDragStart(event: DragEvent, type: ControlNodeType) {
  event.dataTransfer?.setData("application/runner-node-type", type);
  event.dataTransfer?.setData("text/plain", type);
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "copy";
  }
}
</script>

<template>
  <aside class="library-panel">
    <div class="panel-heading">
      <span>Actions</span>
      <small>{{ visibleCount }} / {{ actions.length }}</small>
    </div>

    <label class="catalog-search">
      <Search :size="15" />
      <input v-model="query" type="search" placeholder="Search actions" />
    </label>

    <div v-if="controlNodes.length || groups.length" class="catalog-groups">
      <section v-if="controlNodes.length" class="catalog-group">
        <div class="catalog-group-heading">
          <span>Graph controls</span>
          <small>{{ controlNodes.length }}</small>
        </div>
        <button
          v-for="control in controlNodes"
          :key="control.type"
          class="action-card control-card"
          draggable="true"
          type="button"
          @click="emit('add-control-node', control.type)"
          @dragstart="handleControlDragStart($event, control.type)"
        >
          <span class="action-card-title">
            <strong>{{ control.title }}</strong>
            <small class="risk-badge risk-read_only">{{ control.type }}</small>
          </span>
          <span class="action-description">{{ control.description }}</span>
        </button>
      </section>

      <section v-for="group in groups" :key="group.category" class="catalog-group">
        <div class="catalog-group-heading">
          <span>{{ group.category }}</span>
          <small>{{ group.actions.length }}</small>
        </div>
        <button
          v-for="action in group.actions"
          :key="action.action"
          class="action-card"
          draggable="true"
          type="button"
          @click="emit('add-action', action.action)"
          @dragstart="handleDragStart($event, action)"
        >
          <span class="action-card-title">
            <strong>{{ action.title }}</strong>
            <small :class="['risk-badge', riskClass(action)]">{{ riskLabel(action) }}</small>
          </span>
          <small>{{ action.action }}</small>
          <span v-if="action.description" class="action-description">{{ action.description }}</span>
          <span class="action-flags">
            <small v-if="action.experimental">experimental</small>
            <small v-if="action.deprecated">deprecated</small>
            <small v-if="action.required_args?.length">requires {{ action.required_args.join(", ") }}</small>
          </span>
        </button>
      </section>
    </div>

    <div v-else class="empty-state">No actions match this search.</div>
  </aside>
</template>
