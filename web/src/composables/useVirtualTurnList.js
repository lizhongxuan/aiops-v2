import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

function resolveEstimate(estimateSize, item, index) {
  if (typeof estimateSize === "function") {
    return Number(estimateSize(item, index)) || 0;
  }
  return Number(estimateSize) || 0;
}

function findIndexForOffset(offsets, target) {
  const itemCount = Math.max(offsets.length - 1, 0);
  if (!itemCount) return 0;
  if (target <= 0) return 0;

  let low = 0;
  let high = itemCount - 1;

  while (low < high) {
    const mid = Math.floor((low + high + 1) / 2);
    if (offsets[mid] <= target) {
      low = mid;
    } else {
      high = mid - 1;
    }
  }

  return clamp(low, 0, itemCount - 1);
}

export function useVirtualTurnList({
  items,
  scrollContainer,
  estimateSize = 148,
  overscan = 4,
  minItemCount = 14,
  suspended = false,
  getItemKey = (item, index) => item?.id || index,
} = {}) {
  const scrollTop = ref(0);
  const viewportHeight = ref(0);
  const measuredHeights = ref(new Map());
  const observedItems = new Map();
  let containerObserver = null;
  let heightUpdateTimer = null;

  const resolvedItems = computed(() => asArray(items?.value));
  const virtualizationSuspended = computed(() =>
    typeof suspended === "object" && suspended !== null && "value" in suspended ? Boolean(suspended.value) : Boolean(suspended),
  );
  const itemKeys = computed(() =>
    resolvedItems.value.map((item, index) => String(getItemKey(item, index) ?? index)),
  );
  const itemHeights = computed(() =>
    resolvedItems.value.map((item, index) => {
      const key = itemKeys.value[index];
      return measuredHeights.value.get(key) || resolveEstimate(estimateSize, item, index) || 1;
    }),
  );
  const itemOffsets = computed(() => {
    const offsets = [0];
    for (const height of itemHeights.value) {
      offsets.push(offsets[offsets.length - 1] + height);
    }
    return offsets;
  });
  const totalHeight = computed(() => itemOffsets.value[itemOffsets.value.length - 1] || 0);
  const enabled = computed(
    () => !virtualizationSuspended.value && resolvedItems.value.length >= minItemCount && viewportHeight.value > 0,
  );

  const startIndex = computed(() => {
    if (!enabled.value || !resolvedItems.value.length) return 0;
    return Math.max(findIndexForOffset(itemOffsets.value, scrollTop.value) - overscan, 0);
  });

  const endIndex = computed(() => {
    if (!resolvedItems.value.length) return -1;
    if (!enabled.value) return resolvedItems.value.length - 1;
    const viewportBottom = scrollTop.value + viewportHeight.value;
    return Math.min(
      findIndexForOffset(itemOffsets.value, viewportBottom) + overscan,
      resolvedItems.value.length - 1,
    );
  });

  const virtualItems = computed(() => {
    if (!resolvedItems.value.length) return [];
    if (!enabled.value) {
      return resolvedItems.value.map((item, index) => ({
        index,
        item,
        key: itemKeys.value[index],
      }));
    }
    const start = startIndex.value;
    const end = endIndex.value;
    return resolvedItems.value.slice(start, end + 1).map((item, offset) => {
      const index = start + offset;
      return {
        index,
        item,
        key: itemKeys.value[index],
      };
    });
  });

  const topSpacerHeight = computed(() =>
    enabled.value && resolvedItems.value.length ? itemOffsets.value[startIndex.value] || 0 : 0,
  );
  const bottomSpacerHeight = computed(() => {
    if (!enabled.value || !resolvedItems.value.length) return 0;
    const bottomOffset = itemOffsets.value[endIndex.value + 1] || 0;
    return Math.max(totalHeight.value - bottomOffset, 0);
  });

  function syncViewport() {
    const el = scrollContainer?.value || null;
    viewportHeight.value = el?.clientHeight || 0;
    scrollTop.value = el?.scrollTop || 0;
  }

  function updateMeasuredHeight(key, height) {
    const roundedHeight = Math.max(Math.round(height), 1);
    if (!roundedHeight) return;
    const previous = measuredHeights.value.get(key);
    if (previous === roundedHeight) return;
    // Skip tiny fluctuations (< 4px) to prevent layout thrashing during streaming
    if (previous && Math.abs(previous - roundedHeight) < 4) return;
    const next = new Map(measuredHeights.value);
    next.set(key, roundedHeight);
    measuredHeights.value = next;
  }

  function observeItem(key, el) {
    const existing = observedItems.get(key);
    if (existing?.observer) {
      existing.observer.disconnect();
    }

    if (!el) {
      observedItems.delete(key);
      return;
    }

    updateMeasuredHeight(key, el.getBoundingClientRect?.().height || el.offsetHeight || 0);

    if (typeof ResizeObserver === "undefined") {
      observedItems.set(key, { el, observer: null });
      return;
    }

    const observer = new ResizeObserver((entries) => {
      const nextHeight = entries[0]?.contentRect?.height || el.getBoundingClientRect?.().height || 0;
      // Batch height updates with a short debounce to prevent layout thrashing
      if (heightUpdateTimer) clearTimeout(heightUpdateTimer);
      heightUpdateTimer = setTimeout(() => {
        heightUpdateTimer = null;
        updateMeasuredHeight(key, nextHeight);
      }, 50);
    });
    observer.observe(el);
    observedItems.set(key, { el, observer });
  }

  function setItemRef(key) {
    return (el) => {
      observeItem(String(key), el);
    };
  }

  function handleScroll(event) {
    const el = event?.target || scrollContainer?.value;
    if (!el) return;
    scrollTop.value = el.scrollTop || 0;
    viewportHeight.value = el.clientHeight || viewportHeight.value || 0;
  }

  watch(
    () => scrollContainer?.value,
    async (el, previousEl) => {
      if (containerObserver) {
        containerObserver.disconnect();
        containerObserver = null;
      }

      if (previousEl === el) return;
      await nextTick();
      syncViewport();

      if (typeof ResizeObserver !== "undefined" && el) {
        containerObserver = new ResizeObserver(() => syncViewport());
        containerObserver.observe(el);
      }
    },
    { immediate: true },
  );

  watch(
    itemKeys,
    async (keys) => {
      const nextKeys = new Set(keys);
      for (const [key, binding] of observedItems.entries()) {
        if (!nextKeys.has(key)) {
          binding.observer?.disconnect?.();
          observedItems.delete(key);
        }
      }

      const prunedHeights = new Map();
      for (const key of keys) {
        if (measuredHeights.value.has(key)) {
          prunedHeights.set(key, measuredHeights.value.get(key));
        }
      }
      measuredHeights.value = prunedHeights;

      await nextTick();
      syncViewport();
    },
    { immediate: true },
  );

  onBeforeUnmount(() => {
    if (heightUpdateTimer) { clearTimeout(heightUpdateTimer); heightUpdateTimer = null; }
    if (containerObserver) {
      containerObserver.disconnect();
      containerObserver = null;
    }
    for (const binding of observedItems.values()) {
      binding.observer?.disconnect?.();
    }
    observedItems.clear();
  });

  return {
    enabled,
    virtualItems,
    topSpacerHeight,
    bottomSpacerHeight,
    startIndex,
    endIndex,
    totalHeight,
    handleScroll,
    setItemRef,
    syncViewport,
  };
}
