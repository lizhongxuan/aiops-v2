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

const tableShape = computed(() => {
  const visual = asObject(props.card?.visual);
  const directTable = asObject(props.card?.table);
  const table = Object.keys(directTable).length ? directTable : visual;

  const rows = asArray(table.rows);
  const columns = asArray(table.columns);
  if (columns.length) {
    // Normalize rows: if a row is an object with a `cells` array, unwrap it.
    const normalizedRows = rows.map((row) => {
      if (row && typeof row === "object" && !Array.isArray(row) && Array.isArray(row.cells)) {
        return row.cells;
      }
      return row;
    });
    return { columns, rows: normalizedRows };
  }
  if (rows.length && !Array.isArray(rows[0])) {
    const first = asObject(rows[0]);
    // If rows are objects with `cells`, unwrap and auto-detect columns
    if (Array.isArray(first.cells)) {
      return {
        columns: [],
        rows: rows.map((row) => asArray(asObject(row).cells)),
      };
    }
    const keys = Object.keys(first);
    return {
      columns: keys,
      rows: rows.map((row) => keys.map((key) => row?.[key] ?? "")),
    };
  }
  return { columns: [], rows };
});

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
    <div class="table-wrap">
      <table class="status-table">
        <thead data-testid="mcp-status-table-head">
          <tr>
            <th
              v-for="column in tableShape.columns"
              :key="column"
            >
              {{ column }}
            </th>
          </tr>
        </thead>
        <tbody data-testid="mcp-status-table-row">
          <tr
            v-for="(row, rowIndex) in tableShape.rows"
            :key="`row-${rowIndex}`"
          >
            <td
              v-for="(cell, cellIndex) in row"
              :key="`cell-${rowIndex}-${cellIndex}`"
            >
              {{ cell }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </McpReadonlyCardFrame>
</template>

<style scoped>
.table-wrap {
  overflow-x: auto;
}

.status-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  background: rgba(255, 255, 255, 0.92);
  border-radius: 14px;
  overflow: hidden;
}

.status-table th,
.status-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
}

.status-table th {
  font-weight: 600;
  color: #334155;
  background: rgba(241, 245, 249, 0.92);
}

.status-table td {
  color: #0f172a;
}

.status-table tr:last-child td {
  border-bottom: none;
}
</style>
