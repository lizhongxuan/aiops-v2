import { computed, onBeforeUnmount, onMounted, ref } from "vue";

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function defaultGetItemId(item) {
  return String(item?.id || "");
}

function defaultGetPreview(item) {
  return String(item?.text || item?.summary || item?.label || "").trim();
}

function now() {
  return Date.now();
}

function formatAwayDuration(ms) {
  const totalSeconds = Math.max(1, Math.round(ms / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  if (hours > 0) {
    return `${hours} 小时 ${minutes || 0} 分钟`;
  }
  if (minutes > 0) {
    return `${minutes} 分钟`;
  }
  return `${totalSeconds} 秒`;
}

export function useAwaySummary({
  items,
  getItemId = defaultGetItemId,
  getPreview = defaultGetPreview,
  minAwayMs = 45_000,
} = {}) {
  const awayStartedAt = ref(null);
  const awayBaselineItemIds = ref([]);
  const returnedSummary = ref(null);
  const lastSummarySignature = ref("");

  const resolvedItems = computed(() => asArray(items?.value));

  function startAway() {
    if (awayStartedAt.value) return;
    awayStartedAt.value = now();
    awayBaselineItemIds.value = resolvedItems.value.map((item) => getItemId(item)).filter(Boolean);
  }

  function finishAway() {
    if (!awayStartedAt.value) return;
    const currentItems = resolvedItems.value;
    const baselineIds = new Set(awayBaselineItemIds.value);
    const newItems = currentItems.filter((item) => {
      const id = getItemId(item);
      return id && !baselineIds.has(id);
    });
    const awayMs = Math.max(0, now() - awayStartedAt.value);
    awayStartedAt.value = null;
    awayBaselineItemIds.value = [];

    if (awayMs < minAwayMs || !newItems.length) {
      return;
    }

    const anchorId = getItemId(newItems[0]);
    const latestItem = newItems[newItems.length - 1];
    const latestPreview = String(getPreview(latestItem) || "").trim();
    const newTurnCount = newItems.filter((item) => item?.kind === "turn").length;
    const signature = `${anchorId}:${newItems.map((item) => getItemId(item)).join(",")}`;
    if (!anchorId || signature === lastSummarySignature.value) {
      return;
    }

    lastSummarySignature.value = signature;
    returnedSummary.value = {
      id: `away-summary-${anchorId}`,
      anchorId,
      awayMs,
      durationLabel: formatAwayDuration(awayMs),
      newEntryCount: newItems.length,
      newTurnCount,
      latestPreview,
    };
  }

  function handleVisibilityChange() {
    if (typeof document === "undefined") return;
    if (document.visibilityState === "hidden") {
      startAway();
      return;
    }
    if (document.visibilityState === "visible") {
      finishAway();
    }
  }

  function handleWindowBlur() {
    startAway();
  }

  function handleWindowFocus() {
    if (typeof document !== "undefined" && document.visibilityState === "hidden") {
      return;
    }
    finishAway();
  }

  onMounted(() => {
    if (typeof document !== "undefined") {
      document.addEventListener("visibilitychange", handleVisibilityChange);
    }
    if (typeof window !== "undefined") {
      window.addEventListener("blur", handleWindowBlur);
      window.addEventListener("focus", handleWindowFocus);
    }
  });

  onBeforeUnmount(() => {
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    }
    if (typeof window !== "undefined") {
      window.removeEventListener("blur", handleWindowBlur);
      window.removeEventListener("focus", handleWindowFocus);
    }
  });

  return {
    awaySummary: computed(() => returnedSummary.value),
    clearAwaySummary() {
      returnedSummary.value = null;
    },
  };
}
