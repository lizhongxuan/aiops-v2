<script setup>
import { computed } from "vue";
import McpReadonlyCardFrame from "./McpReadonlyCardFrame.vue";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  embedded: {
    type: Boolean,
    default: false,
  },
  nodes: {
    type: Array,
    default: () => [],
  },
  edges: {
    type: Array,
    default: () => [],
  },
  currentServiceId: {
    type: String,
    default: "",
  },
});

const emit = defineEmits(["detail", "refresh", "node-click"]);

/* ── Topology graph layout ── */

const safeNodes = computed(() =>
  Array.isArray(props.nodes) ? props.nodes.filter((n) => n && n.id) : [],
);

const safeEdges = computed(() =>
  Array.isArray(props.edges) ? props.edges.filter((e) => e && e.source && e.target) : [],
);

const currentId = computed(() => String(props.currentServiceId || ""));

const upstreamNodes = computed(() => {
  if (!currentId.value) return [];
  const upIds = new Set(
    safeEdges.value
      .filter((e) => e.target === currentId.value)
      .map((e) => e.source),
  );
  return safeNodes.value.filter((n) => upIds.has(n.id));
});

const downstreamNodes = computed(() => {
  if (!currentId.value) return [];
  const downIds = new Set(
    safeEdges.value
      .filter((e) => e.source === currentId.value)
      .map((e) => e.target),
  );
  return safeNodes.value.filter((n) => downIds.has(n.id));
});

const currentNode = computed(() =>
  safeNodes.value.find((n) => n.id === currentId.value) || null,
);

/* ── SVG layout constants ── */
const NODE_W = 140;
const NODE_H = 36;
const COL_GAP = 180;
const ROW_GAP = 52;
const PAD_X = 20;
const PAD_Y = 20;

function columnX(colIndex) {
  return PAD_X + colIndex * (NODE_W + COL_GAP);
}

function rowY(rowIndex, totalRows) {
  const totalHeight = totalRows * NODE_H + (totalRows - 1) * (ROW_GAP - NODE_H);
  const startY = PAD_Y + Math.max(0, (svgHeight.value - 2 * PAD_Y - totalHeight) / 2);
  return startY + rowIndex * ROW_GAP;
}

const hasUpstream = computed(() => upstreamNodes.value.length > 0);
const hasDownstream = computed(() => downstreamNodes.value.length > 0);

const totalColumns = computed(() => {
  let cols = 1; // current node always present
  if (hasUpstream.value) cols++;
  if (hasDownstream.value) cols++;
  return cols;
});

const upstreamCol = computed(() => (hasUpstream.value ? 0 : -1));
const currentCol = computed(() => (hasUpstream.value ? 1 : 0));
const downstreamCol = computed(() => {
  if (!hasDownstream.value) return -1;
  return hasUpstream.value ? 2 : 1;
});

const maxRows = computed(() =>
  Math.max(1, upstreamNodes.value.length, downstreamNodes.value.length),
);

const svgWidth = computed(() =>
  2 * PAD_X + totalColumns.value * NODE_W + (totalColumns.value - 1) * COL_GAP,
);

const svgHeight = computed(() =>
  2 * PAD_Y + maxRows.value * NODE_H + Math.max(0, maxRows.value - 1) * (ROW_GAP - NODE_H),
);

/* ── Positioned node objects for SVG rendering ── */

const positionedUpstream = computed(() =>
  upstreamNodes.value.map((n, i) => ({
    ...n,
    x: columnX(upstreamCol.value),
    y: rowY(i, upstreamNodes.value.length),
    direction: "upstream",
  })),
);

const positionedCurrent = computed(() => {
  if (!currentNode.value) return null;
  return {
    ...currentNode.value,
    x: columnX(currentCol.value),
    y: rowY(0, 1),
    direction: "current",
  };
});

const positionedDownstream = computed(() =>
  downstreamNodes.value.map((n, i) => ({
    ...n,
    x: columnX(downstreamCol.value),
    y: rowY(i, downstreamNodes.value.length),
    direction: "downstream",
  })),
);

const allPositioned = computed(() => {
  const list = [...positionedUpstream.value];
  if (positionedCurrent.value) list.push(positionedCurrent.value);
  list.push(...positionedDownstream.value);
  return list;
});

const nodeById = computed(() => {
  const map = new Map();
  for (const n of allPositioned.value) {
    map.set(n.id, n);
  }
  return map;
});

const renderedEdges = computed(() =>
  safeEdges.value
    .map((e) => {
      const src = nodeById.value.get(e.source);
      const tgt = nodeById.value.get(e.target);
      if (!src || !tgt) return null;
      return {
        key: `${e.source}-${e.target}`,
        x1: src.x + NODE_W,
        y1: src.y + NODE_H / 2,
        x2: tgt.x,
        y2: tgt.y + NODE_H / 2,
        label: e.label || "",
      };
    })
    .filter(Boolean),
);

/* ── Helpers ── */

function statusColor(status) {
  const s = String(status || "").toLowerCase();
  if (s === "ok" || s === "healthy" || s === "running") return "#22c55e";
  if (s === "warning" || s === "degraded") return "#f59e0b";
  if (s === "critical" || s === "error" || s === "down") return "#ef4444";
  return "#94a3b8";
}

function nodeClass(node) {
  return [
    "topo-node",
    node.direction === "current" ? "topo-node--current" : "",
    node.direction === "upstream" ? "topo-node--upstream" : "",
    node.direction === "downstream" ? "topo-node--downstream" : "",
  ]
    .filter(Boolean)
    .join(" ");
}

function handleNodeClick(node) {
  emit("node-click", { id: node.id, name: node.name, direction: node.direction });
}

function forwardDetail(payload) {
  emit("detail", payload);
}

function forwardRefresh(payload) {
  emit("refresh", payload);
}
</script>

<template>
  <McpReadonlyCardFrame
    :card="card"
    :embedded="embedded"
    @detail="forwardDetail"
    @refresh="forwardRefresh"
  >
    <article class="topology-shell" data-testid="mcp-topology-card">
      <div v-if="!safeNodes.length" class="topology-empty">
        暂无拓扑数据。
      </div>

      <svg
        v-else
        class="topology-svg"
        :viewBox="`0 0 ${svgWidth} ${svgHeight}`"
        :width="svgWidth"
        :height="svgHeight"
        data-testid="mcp-topology-svg"
      >
        <defs>
          <marker
            id="arrow"
            markerWidth="8"
            markerHeight="6"
            refX="8"
            refY="3"
            orient="auto"
          >
            <path d="M0,0 L8,3 L0,6 Z" fill="#94a3b8" />
          </marker>
        </defs>

        <!-- Edges -->
        <g data-testid="mcp-topology-edges">
          <line
            v-for="edge in renderedEdges"
            :key="edge.key"
            :x1="edge.x1"
            :y1="edge.y1"
            :x2="edge.x2"
            :y2="edge.y2"
            class="topo-edge"
            marker-end="url(#arrow)"
          />
        </g>

        <!-- Nodes -->
        <g
          v-for="node in allPositioned"
          :key="node.id"
          :class="nodeClass(node)"
          :transform="`translate(${node.x}, ${node.y})`"
          :data-testid="`topo-node-${node.id}`"
          :data-direction="node.direction"
          role="button"
          tabindex="0"
          @click="handleNodeClick(node)"
          @keydown.enter="handleNodeClick(node)"
        >
          <rect
            :width="NODE_W"
            :height="NODE_H"
            rx="8"
            ry="8"
            :class="node.direction === 'current' ? 'node-rect--current' : 'node-rect'"
          />
          <circle
            :cx="12"
            :cy="NODE_H / 2"
            r="5"
            :fill="statusColor(node.status)"
          />
          <text
            :x="24"
            :y="NODE_H / 2 + 4"
            class="node-label"
          >
            {{ node.name.length > 14 ? node.name.slice(0, 13) + '…' : node.name }}
          </text>
        </g>
      </svg>

      <!-- Legend -->
      <footer v-if="safeNodes.length" class="topology-legend" data-testid="mcp-topology-legend">
        <span v-if="hasUpstream" class="legend-item legend-upstream">
          ↑ 上游 ({{ upstreamNodes.length }})
        </span>
        <span v-if="currentNode" class="legend-item legend-current">
          ● 当前服务
        </span>
        <span v-if="hasDownstream" class="legend-item legend-downstream">
          ↓ 下游 ({{ downstreamNodes.length }})
        </span>
      </footer>
    </article>
  </McpReadonlyCardFrame>
</template>

<style scoped>
.topology-shell {
  padding: 12px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(15, 23, 42, 0.06);
  overflow-x: auto;
}

.topology-empty {
  padding: 24px;
  text-align: center;
  font-size: 13px;
  color: #64748b;
}

.topology-svg {
  display: block;
  max-width: 100%;
  height: auto;
}

/* Edges */
.topo-edge {
  stroke: #cbd5e1;
  stroke-width: 1.5;
  fill: none;
}

/* Node rects */
.node-rect {
  fill: rgba(241, 245, 249, 0.95);
  stroke: #cbd5e1;
  stroke-width: 1;
}

.node-rect--current {
  fill: rgba(219, 234, 254, 0.95);
  stroke: #3b82f6;
  stroke-width: 2;
}

/* Node labels */
.node-label {
  font-size: 12px;
  fill: #0f172a;
  font-family: inherit;
  pointer-events: none;
}

/* Hover / focus */
.topo-node {
  cursor: pointer;
  outline: none;
}

.topo-node:hover .node-rect,
.topo-node:focus .node-rect {
  stroke: #3b82f6;
  stroke-width: 1.5;
}

.topo-node:hover .node-rect--current,
.topo-node:focus .node-rect--current {
  stroke: #2563eb;
}

/* Legend */
.topology-legend {
  display: flex;
  gap: 12px;
  padding-top: 8px;
  border-top: 1px solid rgba(226, 232, 240, 0.6);
  margin-top: 8px;
}

.legend-item {
  font-size: 12px;
  color: #64748b;
}

.legend-upstream {
  color: #f59e0b;
}

.legend-current {
  color: #3b82f6;
  font-weight: 600;
}

.legend-downstream {
  color: #22c55e;
}
</style>
