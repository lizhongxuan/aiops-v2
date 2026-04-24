import { computed, nextTick, ref, watch } from "vue";

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function resolveInitialVisibleCount(totalCount, initialCount) {
  return Math.min(Math.max(initialCount, 0), Math.max(totalCount, 0));
}

export function useChatHistoryPager({
  items,
  scrollContainer,
  resetKey,
  pageSize = 12,
  initialCount = pageSize,
  topThreshold = 72,
  onLoadOlder,
} = {}) {
  const visibleCount = ref(resolveInitialVisibleCount(asArray(items?.value).length, initialCount));
  const loadingOlder = ref(false);
  const loadOlderError = ref("");
  const hasLoadedOlder = ref(false);

  const totalCount = computed(() => asArray(items?.value).length);
  const visibleItems = computed(() => {
    const list = asArray(items?.value);
    if (!list.length) return [];
    return list.slice(Math.max(list.length - visibleCount.value, 0));
  });
  const hiddenCount = computed(() => Math.max(totalCount.value - visibleItems.value.length, 0));
  const hasOlder = computed(() => hiddenCount.value > 0);
  const paginationActive = computed(
    () => totalCount.value > initialCount || hasLoadedOlder.value || Boolean(loadOlderError.value) || loadingOlder.value,
  );

  const topSentinel = computed(() => {
    if (!paginationActive.value || !totalCount.value) return null;
    if (loadingOlder.value) {
      return {
        id: "history-loading",
        kind: "loading",
        text: "正在加载更早消息...",
        detail: "",
        actionLabel: "",
      };
    }
    if (loadOlderError.value) {
      return {
        id: "history-error",
        kind: "error",
        text: "更早消息加载失败，请重试。",
        detail: loadOlderError.value,
        actionLabel: "重试",
      };
    }
    if (hasOlder.value) {
      return {
        id: "history-compact",
        kind: "compact",
        text: `更早上下文已折叠 ${hiddenCount.value} 条消息。`,
        detail: "滚动到顶部或点击“加载更早消息”继续查看。",
        actionLabel: "加载更早消息",
      };
    }
    return {
      id: "history-start",
      kind: "start",
      text: "已到会话开头。",
      detail: "",
      actionLabel: "",
    };
  });

  function resetPagination() {
    visibleCount.value = resolveInitialVisibleCount(totalCount.value, initialCount);
    loadingOlder.value = false;
    loadOlderError.value = "";
    hasLoadedOlder.value = false;
  }

  async function loadOlder() {
    if (loadingOlder.value || !hasOlder.value) return false;
    const el = scrollContainer?.value || null;
    const previousScrollHeight = el?.scrollHeight || 0;
    const previousScrollTop = el?.scrollTop || 0;
    const previousVisibleCount = visibleCount.value;
    const previousTotalCount = totalCount.value;

    loadingOlder.value = true;
    loadOlderError.value = "";

    try {
      if (typeof onLoadOlder === "function") {
        await onLoadOlder({
          hiddenCount: hiddenCount.value,
          totalCount: previousTotalCount,
          visibleCount: previousVisibleCount,
        });
      }

      if (visibleCount.value === previousVisibleCount) {
        visibleCount.value = Math.min(totalCount.value, previousVisibleCount + pageSize);
      }
      hasLoadedOlder.value = true;

      await nextTick();
      if (el) {
        const nextScrollHeight = el.scrollHeight || 0;
        el.scrollTop = Math.max(nextScrollHeight - previousScrollHeight + previousScrollTop, 0);
      }
      return true;
    } catch (error) {
      loadOlderError.value = error instanceof Error ? error.message : "请稍后重试";
      return false;
    } finally {
      loadingOlder.value = false;
    }
  }

  function handleScroll(event) {
    const el = event?.target || scrollContainer?.value;
    if (!el || el.scrollTop > topThreshold || !hasOlder.value || loadingOlder.value) return;
    void loadOlder();
  }

  watch(
    () => resetKey?.value,
    () => {
      resetPagination();
    },
  );

  watch(
    totalCount,
    (count, previousCount) => {
      if (previousCount === undefined) {
        visibleCount.value = resolveInitialVisibleCount(count, initialCount);
        return;
      }

      if (count < visibleCount.value) {
        visibleCount.value = count;
      }

      if (count > previousCount) {
        const previousHiddenCount = Math.max(previousCount - visibleCount.value, 0);
        if (previousHiddenCount === 0) {
          visibleCount.value = Math.min(count, visibleCount.value + (count - previousCount));
        }
      }

      if (!count) {
        visibleCount.value = 0;
      }
    },
    { immediate: true },
  );

  return {
    visibleItems,
    visibleCount,
    totalCount,
    hiddenCount,
    hasOlder,
    loadingOlder,
    loadOlderError,
    topSentinel,
    loadOlder,
    handleScroll,
    resetPagination,
  };
}
