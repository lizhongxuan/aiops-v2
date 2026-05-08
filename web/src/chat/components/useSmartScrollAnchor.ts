import { useCallback, useLayoutEffect, useRef, useState, type WheelEvent } from "react";

export function isNearTranscriptBottom(el: HTMLElement, threshold = 24) {
  return el.scrollHeight - el.clientHeight - el.scrollTop <= threshold;
}

export function useSmartScrollAnchor(deps: readonly unknown[], threshold = 24) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const shouldAutoScrollRef = useRef(true);
  const [showScrollToBottom, setShowScrollToBottom] = useState(false);

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) {
      return;
    }
    shouldAutoScrollRef.current = true;
    el.scrollTop = el.scrollHeight;
    setShowScrollToBottom(false);
  }, []);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) {
      return;
    }
    const nearBottom = isNearTranscriptBottom(el, threshold);
    shouldAutoScrollRef.current = nearBottom;
    setShowScrollToBottom(!nearBottom);
  }, [threshold]);

  const handleWheel = useCallback((event: WheelEvent<HTMLElement>) => {
    if (event.deltaY < 0) {
      shouldAutoScrollRef.current = false;
      setShowScrollToBottom(true);
    }
  }, []);

  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (!el || !shouldAutoScrollRef.current) {
      return;
    }
    el.scrollTop = el.scrollHeight;
    setShowScrollToBottom(false);
    // deps is intentionally supplied by the transcript owner, which knows what state changes append content.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return {
    scrollRef,
    showScrollToBottom,
    handleScroll,
    handleWheel,
    scrollToBottom,
  };
}
