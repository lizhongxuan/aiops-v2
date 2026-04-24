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
});

const emit = defineEmits(["detail", "refresh"]);

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function toneColor(tone = "") {
  switch (String(tone || "").toLowerCase()) {
    case "danger":
      return "#dc2626";
    case "warning":
      return "#d97706";
    case "positive":
    case "success":
      return "#059669";
    default:
      return "#2563eb";
  }
}

const viewBoxWidth = 360;
const viewBoxHeight = 120;

const seriesList = computed(() => {
  const visual = asObject(props.card?.visual);
  const rawSeries = asArray(visual.series);
  if (rawSeries.length) {
    return rawSeries.map((series, index) => {
      const source = asObject(series);
      // Accept both `points` (native) and `data` (Coroot adapter format)
      const rawPoints = asArray(source.points).length ? asArray(source.points) : asArray(source.data);
      return {
        id: source.id || `series-${index + 1}`,
        label: source.label || source.name || `Series ${index + 1}`,
        tone: source.tone || "info",
        points: rawPoints.map((point, pointIndex) => {
          const item = asObject(point);
          return {
            x: Number(item.x ?? item.timestamp ?? pointIndex),
            y: Number(item.y ?? item.value ?? 0),
          };
        }),
      };
    });
  }

  const rawPoints = asArray(visual.points);
  if (rawPoints.length) {
    return [{
      id: "series-1",
      label: visual.label || "Series 1",
      tone: visual.tone || "info",
      points: rawPoints.map((point, index) => {
        const item = asObject(point);
        return {
          x: Number(item.x ?? index),
          y: Number(item.y ?? item.value ?? 0),
        };
      }),
    }];
  }

  return [];
});

const yRange = computed(() => {
  const values = seriesList.value.flatMap((series) => series.points.map((point) => point.y));
  if (!values.length) return { min: 0, max: 1 };
  const min = Math.min(...values);
  const max = Math.max(...values);
  if (min === max) return { min: min - 1, max: max + 1 };
  return { min, max };
});

function pathForSeries(points = []) {
  if (!points.length) return "";
  const range = yRange.value;
  const xStep = points.length > 1 ? viewBoxWidth / (points.length - 1) : viewBoxWidth;
  return points
    .map((point, index) => {
      const x = index * xStep;
      const normalizedY = (point.y - range.min) / (range.max - range.min || 1);
      const y = viewBoxHeight - normalizedY * viewBoxHeight;
      return `${index === 0 ? "M" : "L"} ${x.toFixed(2)} ${y.toFixed(2)}`;
    })
    .join(" ");
}

function circlesForSeries(points = []) {
  if (!points.length) return [];
  const range = yRange.value;
  const xStep = points.length > 1 ? viewBoxWidth / (points.length - 1) : viewBoxWidth;
  return points.map((point, index) => {
    const x = index * xStep;
    const normalizedY = (point.y - range.min) / (range.max - range.min || 1);
    const y = viewBoxHeight - normalizedY * viewBoxHeight;
    return {
      id: `${index}-${point.y}`,
      x,
      y,
    };
  });
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
    <div class="chart-shell">
      <svg
        viewBox="0 0 360 120"
        class="chart-svg"
        aria-label="Timeseries chart"
      >
        <line
          v-for="step in 4"
          :key="`grid-${step}`"
          x1="0"
          :y1="step * 24"
          :x2="viewBoxWidth"
          :y2="step * 24"
          class="grid-line"
        />
        <g
          v-for="series in seriesList"
          :key="series.id"
        >
          <path
            :d="pathForSeries(series.points)"
            fill="none"
            stroke-width="3"
            stroke-linecap="round"
            stroke-linejoin="round"
            :stroke="toneColor(series.tone)"
          />
          <circle
            v-for="point in circlesForSeries(series.points)"
            :key="`${series.id}-${point.id}`"
            :cx="point.x"
            :cy="point.y"
            r="3"
            :fill="toneColor(series.tone)"
          />
        </g>
      </svg>

      <div class="legend">
        <span
          v-for="series in seriesList"
          :key="`${series.id}-legend`"
          class="legend-item"
        >
          <span
            class="legend-dot"
            :style="{ backgroundColor: toneColor(series.tone) }"
          ></span>
          {{ series.label }}
        </span>
      </div>
    </div>
  </McpReadonlyCardFrame>
</template>

<style scoped>
.chart-shell {
  display: grid;
  gap: 10px;
}

.chart-svg {
  width: 100%;
  height: auto;
  border-radius: 14px;
  background: linear-gradient(180deg, rgba(239, 246, 255, 0.9), rgba(248, 250, 252, 0.96));
  border: 1px solid rgba(37, 99, 235, 0.08);
  padding: 8px;
  box-sizing: border-box;
}

.grid-line {
  stroke: rgba(148, 163, 184, 0.35);
  stroke-width: 1;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #475569;
}

.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
}
</style>
