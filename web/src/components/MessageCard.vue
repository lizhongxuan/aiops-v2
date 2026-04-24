<script setup>
import { computed, ref, watch } from "vue";
import { UserIcon, BotIcon, CopyIcon, CheckIcon, ChevronDownIcon } from "lucide-vue-next";
import { NSkeleton } from "naive-ui";
import MarkdownIt from "markdown-it";
import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import json from "highlight.js/lib/languages/json";
import yaml from "highlight.js/lib/languages/yaml";
import nginx from "highlight.js/lib/languages/nginx";
import python from "highlight.js/lib/languages/python";
import go from "highlight.js/lib/languages/go";
import "highlight.js/styles/github.css";
import Modal from "./Modal.vue";
import InlineEntityChip from "./chat/InlineEntityChip.vue";
import { fetchFilePreview } from "../api/files";

// Register highlight.js languages on-demand
hljs.registerLanguage("bash", bash);
hljs.registerLanguage("json", json);
hljs.registerLanguage("yaml", yaml);
hljs.registerLanguage("nginx", nginx);
hljs.registerLanguage("python", python);
hljs.registerLanguage("go", go);

// Configure markdown-it with highlight.js
const md = new MarkdownIt({
  html: false,
  breaks: true,
  linkify: true,
  highlight(str, lang) {
    if (lang && hljs.getLanguage(lang)) {
      try {
        return hljs.highlight(str, { language: lang }).value;
      } catch { /* fallback */ }
    }
    return ""; // use external default escaping
  },
});

// --- LRU Cache for markdown rendering (max 80 entries) ---
const MD_CACHE_MAX = 80;
const mdCache = new Map();

function renderMarkdownCached(text) {
  if (mdCache.has(text)) {
    // Move to end (most recently used)
    const val = mdCache.get(text);
    mdCache.delete(text);
    mdCache.set(text, val);
    return val;
  }
  const rendered = md.render(text);
  if (mdCache.size >= MD_CACHE_MAX) {
    // Evict oldest (first key)
    const firstKey = mdCache.keys().next().value;
    mdCache.delete(firstKey);
  }
  mdCache.set(text, rendered);
  return rendered;
}

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  showCopyButton: {
    type: Boolean,
    default: true,
  },
});

const isUser = computed(() => props.card.role === "user");
const rawText = computed(() => props.card.text || props.card.title || "");
const messageText = computed(() => isUser.value ? rawText.value : cleanDisplayText(rawText.value));
const mcpAppHtml = computed(() => {
  if (isUser.value) return "";
  return String(props.card?.detail?.mcpApp?.html || "").trim();
});
const showSkeleton = computed(() => !isUser.value && props.card.status === "inProgress" && !rawText.value.trim());

const avatarIcon = computed(() => {
  return isUser.value ? UserIcon : BotIcon;
});

const renderAsCode = computed(() => {
  if (isUser.value) return false;
  if (containsMarkdownLinks(messageText.value)) return false;
  return looksStructuredText(messageText.value);
});

// Always render assistant messages as Markdown — markdown-it handles plain text fine
// and properly formats lists, paragraphs, bold, code blocks, etc.
const renderAsMarkdown = computed(() => {
  if (isUser.value) return false;
  if (renderAsCode.value) return false;
  return true;
});

// --- Auto-collapse for long messages (>8000 chars) ---
const COLLAPSE_THRESHOLD = 8000;
const COLLAPSE_PREVIEW_LEN = 2000;
const isCollapsed = ref(true);

const isLongMessage = computed(() => messageText.value.length > COLLAPSE_THRESHOLD);

function toggleCollapse() {
  isCollapsed.value = !isCollapsed.value;
}

const displayText = computed(() => {
  if (isLongMessage.value && isCollapsed.value) {
    return messageText.value.slice(0, COLLAPSE_PREVIEW_LEN);
  }
  return messageText.value;
});

// --- Render markdown only after streaming settles to keep final output smooth ---
const isStreaming = computed(() => props.card.status === "inProgress");
const useStreamingPlainText = computed(() => isStreaming.value && renderAsMarkdown.value);
const throttledHtml = ref("");

function updateRenderedHtml() {
  if (!renderAsMarkdown.value || useStreamingPlainText.value) {
    throttledHtml.value = "";
    return;
  }
  try {
    throttledHtml.value = renderMarkdownCached(displayText.value);
  } catch {
    throttledHtml.value = "";
  }
}

watch(
  [displayText, renderAsMarkdown, useStreamingPlainText],
  () => {
    updateRenderedHtml();
  },
  { immediate: true },
);

// When streaming ends, do a final render to ensure we have the latest
watch(isStreaming, (val, oldVal) => {
  if (oldVal && !val) {
    updateRenderedHtml();
  }
});

const renderedMarkdown = computed(() => throttledHtml.value);
const decoratedMarkdown = computed(() => decorateInlineEntitiesHtml(renderedMarkdown.value));

function containsMarkdownLinks(value) {
  return /\[([^\]]+)\]\(([^)]+)\)/.test(value || "");
}

function looksStructuredText(value) {
  const trimmed = value.trim();
  if (!trimmed.includes("\n")) return false;
  const lines = trimmed
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  if (lines.length < 2) return false;

  let structuredCount = 0;
  for (const line of lines) {
    if (
      /^[./~\w-][./~\w\s-]*$/.test(line) ||
      /[\\/]/.test(line) ||
      /\.[A-Za-z0-9_-]+$/.test(line) ||
      /^[A-Za-z0-9_.-]+$/.test(line)
    ) {
      structuredCount += 1;
    }
  }
  return structuredCount / lines.length >= 0.6;
}

/**
 * Clean assistant message text for display:
 * - Remove embedded JSON routing blocks (```json {"route":...} ```)
 * - Remove inline JSON routing objects
 * - Filter out system routing preamble lines
 */
function cleanDisplayText(text) {
  if (!text) return text;
  let cleaned = text;
  // Remove ```json ... ``` fenced blocks containing routing metadata (multiline)
  cleaned = cleaned.replace(/`{3}json[\s\S]*?`{3}/g, (match) => {
    if (/"route"\s*:/.test(match)) return "";
    return match; // keep non-routing code blocks
  });
  // Fallback: remove unclosed ```json blocks that contain routing metadata
  cleaned = cleaned.replace(/`{3}json\s*\{[^`]*"route"\s*:[^`]*/g, "");
  // Remove inline JSON objects containing "route" key
  cleaned = cleaned.replace(/\{[^{}]*"route"\s*:\s*"[^"]*"[^{}]*\}/g, "");
  // Remove system routing preamble lines
  cleaned = cleaned.replace(/^主\s*Agent\s*正在判断[^\n]*\n?/gm, "");
  cleaned = cleaned.replace(/^这是简单对话[^\n]*\n?/gm, "");
  cleaned = cleaned.replace(/^(这是|当前).*(简单|直接).*(对话|回答|回复)[^\n]*\n?/gm, "");
  cleaned = cleaned.replace(/不会生成计划或派发\s*worker[^\n]*\n?/gm, "");
  cleaned = stripLeadingGenericConclusionHeading(cleaned);
  cleaned = stripCommandEvidenceSummary(cleaned);
  cleaned = stripMarketSnapshotSectionHeadings(cleaned);
  cleaned = compactMarketSnapshotDisplay(cleaned);
  // Normalize "1." / "-" lines that are separated from their actual content by blank lines,
  // which otherwise render as visually fragmented list items.
  cleaned = cleaned.replace(/^(\s*\d+\.)\s*\n+\s*(?=\S)/gm, "$1 ");
  cleaned = cleaned.replace(/^(\s*[-*+])\s*\n+\s*(?=\S)/gm, "$1 ");
  cleaned = cleaned.replace(/(^\s*(?:\d+\.\s+[^\n]+|[-*+]\s+[^\n]+))\n{2,}(?=\s*[○•◦▪■▸▹►]\s)/gm, "$1\n");
  cleaned = normalizeOutlineMarkdown(cleaned);
  // Collapse excessive newlines
  cleaned = cleaned.replace(/\n{3,}/g, "\n\n").trim();
  return cleaned || text;
}

function stripLeadingGenericConclusionHeading(text) {
  return String(text || "").replace(
    /^\s*(?:#{1,6}\s*)?(?:\*\*)?(?:最终)?结论(?:\*\*)?\s*(?:\r?\n)+/u,
    "",
  );
}

function stripCommandEvidenceSummary(text) {
  const lines = String(text || "").split("\n");
  const normalized = [];
  let skippingEvidence = false;

  for (let index = 0; index < lines.length; index += 1) {
    const rawLine = lines[index];
    const trimmed = rawLine.trim();

    if (!skippingEvidence && /^证据摘要[:：]?\s*$/u.test(trimmed)) {
      skippingEvidence = true;
      continue;
    }

    if (skippingEvidence) {
      if (!trimmed) {
        continue;
      }
      if (
        /^(?:[-*+]|[○•◦▪■▸▹►])\s+/u.test(trimmed) ||
        /^(?:返回|输出)[:：]/u.test(trimmed) ||
        /^(?:已运行|正在运行|已执行|正在执行)\s+/u.test(trimmed) ||
        /[`$]/u.test(trimmed) ||
        /(?:systemctl|journalctl|tail\b|cat\b|grep\b|top\b|ps\b|curl\b|kubectl\b|docker\b|ssh\b)/iu.test(trimmed)
      ) {
        continue;
      }
      skippingEvidence = false;
    }

    normalized.push(rawLine);
  }

  return normalized.join("\n").replace(/\n{3,}/g, "\n\n").trim();
}

function stripMarketSnapshotSectionHeadings(text) {
  const value = String(text || "");
  if (!looksLikeMarketSnapshot(value)) return value;
  return value.replace(
    /^\s*(?:#{1,6}\s*)?(?:\*\*)?(关键证据|主流报价|市场状态|简要解读|详细分析|证据详情|来源详解|补充说明)(?:\*\*)?\s*(?:\r?\n)+/gmu,
    "",
  );
}

function looksLikeMarketSnapshot(text) {
  const value = String(text || "");
  return (
    /(BTC|比特币|CoinMarketCap|CoinGecko|Binance|Crypto\.com|24h|24小时|市值|成交额)/i.test(value) ||
    /(A股|股市|上证|深证|创业板|指数|行情|盘面|涨停|跌停)/i.test(value)
  );
}

function compactMarketSnapshotDisplay(text) {
  const value = String(text || "").trim();
  if (!shouldCompactMarketSnapshot(value)) return value;

  const flattened = flattenMarketSnapshotBullets(value);
  const blocks = flattened
    .split(/\n{2,}/)
    .map((block) => block.trim())
    .filter(Boolean);

  const introBlocks = [];
  const judgmentBlocks = [];
  const sourceRefs = [];
  const bulletCandidates = [];
  for (const block of blocks) {
    if (looksLikeSourcesLabel(block)) continue;
    if (isFollowUpPrompt(block)) {
      continue;
    }

    const blockLines = block
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);

    if (blockLines.length === 1 && isInlineSourceSummaryLine(blockLines[0])) {
      sourceRefs.push(...extractSourceReferencesFromInlineSummary(blockLines[0]));
      continue;
    }

    if (blockLines.length && looksLikeSourcesLabel(blockLines[0])) {
      const normalizedLinks = blockLines
        .slice(1)
        .filter((line) => isSourceLikeLine(line))
        .map((line) => normalizeSourceReference(line))
        .filter(Boolean);
      const normalizedInline = blockLines
        .slice(1)
        .filter((line) => isInlineSourceSummaryLine(line))
        .flatMap((line) => extractSourceReferencesFromInlineSummary(line));
      if (normalizedLinks.length || normalizedInline.length) {
        sourceRefs.push(...normalizedInline);
        sourceRefs.push(...normalizedLinks);
        continue;
      }
    }

    const allBullets = blockLines.length > 0 && blockLines.every((line) => isBulletLine(line) || isSourceLikeLine(line));
    if (allBullets) {
      for (const line of blockLines) {
        if (isSourceLikeLine(line)) {
          const ref = normalizeSourceReference(line);
          if (ref) sourceRefs.push(ref);
          continue;
        }
        const content = normalizeMarketBulletCandidate(line);
        if (content) bulletCandidates.push(content);
      }
      continue;
    }

    if (blockLines.every((line) => isSourceLikeLine(line))) {
      for (const line of blockLines) {
        const ref = normalizeSourceReference(line);
        if (ref) sourceRefs.push(ref);
      }
      continue;
    }

    if (!introBlocks.length) {
      introBlocks.push(normalizeMarketIntro(block));
      continue;
    }

    if (blockLines.some((line) => isBulletLine(line))) {
      for (const line of blockLines) {
        if (!isBulletLine(line)) continue;
        const content = normalizeMarketBulletCandidate(line);
        if (content) bulletCandidates.push(content);
      }
      continue;
    }

    if (looksLikeMarketJudgment(block)) {
      judgmentBlocks.push(block);
      continue;
    }

    if (containsMarkdownLinks(block)) {
      const normalizedLinks = blockLines
        .filter((line) => isSourceLikeLine(line))
        .map((line) => normalizeSourceReference(line))
        .filter(Boolean);
      if (normalizedLinks.length) {
        sourceRefs.push(...normalizedLinks);
        continue;
      }
    }

    judgmentBlocks.push(block);
  }

  const compactBullets = selectMarketBullets(bulletCandidates);
  const compactSources = dedupeSourceReferences([
    ...sourceRefs,
    ...collectImplicitMarketSources(compactBullets).map((label) => createLabelSourceReference(label)),
  ])
    .filter((entry) => isUsableMarketSourceLabel(entry.label))
    .slice(0, 2);
  const introText = introBlocks[0] || "";
  const judgmentText = extractExplicitMarketJudgment(flattened) || chooseCompactJudgment(judgmentBlocks);
  const compactJudgment = normalizeCompactMarketJudgment(judgmentText);

  const introWithJudgment = mergeCompactMarketIntroAndJudgment(introText, compactJudgment);
  const parts = [];
  if (introWithJudgment.intro) parts.push(introWithJudgment.intro);
  if (compactBullets.length) parts.push(compactBullets.map((line) => `- ${line}`).join("\n"));
  if (introWithJudgment.judgment && !isDuplicateParagraph(introWithJudgment.judgment, introWithJudgment.intro)) {
    parts.push(introWithJudgment.judgment);
  }
  if (compactSources.length) parts.push(buildCompactSourceReferenceBlock(compactSources));

  const compacted = parts
    .join("\n\n")
    .replace(/(?:^|\n)\s*(?:短判断|一句话判断|简判断)[:：]\s*/gu, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
  return compacted || value;
}

function shouldCompactMarketSnapshot(text) {
  const value = String(text || "");
  if (!looksLikeMarketSnapshot(value)) return false;
  const bulletCount = (value.match(/^\s*[-*+]\s+/gm) || []).length;
  const headingCount = (
    value.match(/^\s*(?:#{1,6}\s*)?(?:\*\*)?(关键证据|主流报价|市场状态|简要解读|详细分析|证据详情|来源详解|补充说明)(?:\*\*)?\s*$/gmu) ||
    []
  ).length;
  const labeledJudgmentOrSources = /(?:^|\n)\s*(?:短判断|一句话判断|来源)\s*[:：]/u.test(value);
  const rawSourceUrl = /(?:^|\n)\s*(?:[-*+]\s+)?(?:[A-Za-z0-9.\u4e00-\u9fa5_-]+\s*[:：]\s*)?https?:\/\/\S+/iu.test(value);
  const followUpPrompt = /(?:^|\n)\s*(如果你要|如果需要|要的话|如果你愿意)/u.test(value);
  return headingCount > 0 || bulletCount > 4 || value.length > 420 || labeledJudgmentOrSources || rawSourceUrl || followUpPrompt;
}

function flattenMarketSnapshotBullets(text) {
  const lines = String(text || "").split("\n");
  const flattened = [];

  for (let index = 0; index < lines.length; index += 1) {
    const current = lines[index].replace(/\s+$/g, "");
    const trimmed = current.trim();
    if (!isBulletLine(trimmed) || /^\s{2,}/.test(current) || /^[\t ]+[○•◦▪■▸▹►]\s+/.test(current)) {
      flattened.push(current);
      continue;
    }

    const nestedParts = [];
    let offset = index + 1;
    while (offset < lines.length) {
      const nestedRaw = lines[offset].replace(/\s+$/g, "");
      const nestedTrimmed = nestedRaw.trim();
      if (!nestedTrimmed) {
        offset += 1;
        continue;
      }
      if (/^[\t ]{2,}(?:[-*+]|[○•◦▪■▸▹►])\s+/.test(nestedRaw)) {
        nestedParts.push(normalizeBulletContent(nestedTrimmed));
        offset += 1;
        continue;
      }
      break;
    }

    if (nestedParts.length > 0) {
      flattened.push(`- ${joinMarketBulletParts(normalizeMarketBulletCandidate(trimmed), nestedParts.map((part) => normalizeMarketBulletCandidate(part)))}`);
      index = offset - 1;
      continue;
    }

    flattened.push(`- ${normalizeMarketBulletCandidate(trimmed)}`);
  }

  return flattened.join("\n");
}

function joinMarketBulletParts(parent, nestedParts) {
  const parts = [String(parent || "").trim(), ...nestedParts.map((part) => String(part || "").trim()).filter(Boolean)];
  return parts.filter(Boolean).join("，");
}

function selectMarketBullets(candidates) {
  const deduped = dedupeStable(candidates.map((line) => normalizeInlineSpacing(line)).filter(Boolean));
  if (deduped.length <= 4) return deduped;

  const scored = deduped.map((line, index) => ({ line, index, score: scoreMarketBullet(line) }));
  return scored
    .sort((a, b) => b.score - a.score || a.index - b.index)
    .slice(0, 4)
    .sort((a, b) => a.index - b.index)
    .map((entry) => entry.line);
}

function scoreMarketBullet(line) {
  let score = 0;
  const value = String(line || "");
  if (/(CoinGecko|CoinMarketCap|Binance|Crypto\.com|The Block|新浪|东方财富|上证|深证|创业板)/i.test(value)) score += 5;
  if (/(\$|¥|元|点|亿|万|T\b|B\b|%)/.test(value)) score += 4;
  if (/(24h|24小时|区间|成交额|成交量|市值|总市值|支撑|压力|涨跌|涨幅|跌幅|现价|报价)/i.test(value)) score += 3;
  if (/(搜索结果|搜索快照|页面头部|页面汇率|转换页|摘要显示|摘显示)/.test(value)) score -= 4;
  if (/(多个来源都指向|不同平台价格差异|口径差异|正常现象|参考|说明当前)/.test(value)) score -= 3;
  return score;
}

function chooseCompactJudgment(blocks) {
  const candidates = blocks
    .map((block) => normalizeMarketJudgment(block))
    .filter(Boolean)
    .filter((block) => !containsMarkdownLinks(block))
    .filter((block) => !/^(我这里|这里的|说明当前|这说明|换句话说)/u.test(block))
    .filter((block) => looksLikeMarketJudgment(block) || block.length <= 120);
  return candidates[0] || "";
}

function extractExplicitMarketJudgment(text) {
  const match = String(text || "").match(/(?:^|\n)(一句话判断[:：]?\s*[^\n]+)(?=\n|$)/u);
  return match ? normalizeMarketJudgment(match[1]) : "";
}

function looksLikeMarketJudgment(text) {
  return /(偏强|偏弱|震荡|突破|承压|支撑|压力|短线|结构|强弱|行情|走势|判断|不是单边|偏震荡|偏活跃)/.test(String(text || ""));
}

function looksLikeSourcesLabel(text) {
  return /^(?:#{1,6}\s*)?(?:\*\*)?来源(?:\*\*)?[:：]?$/.test(String(text || "").trim());
}

function isSourceLinkLine(line) {
  return /^\s*(?:[-*+]\s+)?\[[^\]]+\]\([^)]+\)\s*$/.test(String(line || ""));
}

function isPlainSourceLine(line) {
  return /^(?:\s*[-*+]\s+)?(?:[A-Za-z0-9.\u4e00-\u9fa5_-]+\s*[:：]\s*)?https?:\/\/\S+$/iu.test(String(line || "").trim());
}

function isSourceLikeLine(line) {
  return isSourceLinkLine(line) || isPlainSourceLine(line);
}

function normalizeSourceLine(line) {
  const value = String(line || "").trim().replace(/^[-*+]\s+/, "");
  if (isSourceLinkLine(value)) {
    const match = value.match(/^\[([^\]]+)\]\([^)]+\)$/);
    return match ? match[1].trim() : value;
  }
  if (isPlainSourceLine(value)) {
    const sourceLabel = extractImplicitMarketSourceLabel(value);
    if (sourceLabel) return sourceLabel;
    const urlMatch = value.match(/https?:\/\/([^/\s?#]+)/i);
    if (urlMatch) return urlMatch[1].replace(/^www\./i, "");
  }
  return value;
}

function createLabelSourceReference(label) {
  const normalized = normalizeSourceLine(label);
  return {
    type: "label",
    label: normalized,
    key: `label:${normalized.replace(/\s+/g, "").toLowerCase()}`,
    markdown: "",
  };
}

function normalizeSourceReference(line) {
  const value = String(line || "").trim().replace(/^[-*+]\s+/, "");
  if (!value) return null;
  if (isSourceLinkLine(value)) {
    const match = value.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
    if (match) {
      const label = match[1].trim();
      const url = match[2].trim();
      return {
        type: "markdown",
        label,
        key: `markdown:${label.replace(/\s+/g, "").toLowerCase()}:${url}`,
        markdown: `[${label}](${url})`,
      };
    }
  }
  if (isPlainSourceLine(value)) {
    const urlMatch = value.match(/https?:\/\/\S+/iu);
    if (urlMatch) {
      const url = urlMatch[0];
      const label = extractImplicitMarketSourceLabel(value) || normalizeSourceLine(value);
      return {
        type: "markdown",
        label,
        key: `markdown:${label.replace(/\s+/g, "").toLowerCase()}:${url}`,
        markdown: `[${label}](${url})`,
      };
    }
  }
  return createLabelSourceReference(value);
}

function isUsableMarketSourceLabel(value) {
  return /[A-Za-z\u4e00-\u9fa5]{2,}/u.test(String(value || ""));
}

function isInlineSourceSummaryLine(line) {
  return /^来源[:：]\s*/u.test(String(line || "").trim());
}

function extractSourceLabelsFromInlineSummary(line) {
  const raw = String(line || "").trim().replace(/^来源[:：]\s*/u, "");
  if (!raw) return [];
  return dedupeStable(
    raw
      .split(/[；;,，、]\s*/u)
      .map((entry) => normalizeSourceLine(entry))
      .filter((entry) => isUsableMarketSourceLabel(entry)),
  );
}

function extractSourceReferencesFromInlineSummary(line) {
  return extractSourceLabelsFromInlineSummary(line).map((label) => createLabelSourceReference(label));
}

function dedupeSourceReferences(items) {
  const seen = new Set();
  const result = [];
  for (const item of items || []) {
    if (!item?.key || seen.has(item.key)) continue;
    seen.add(item.key);
    result.push(item);
  }
  return result;
}

function buildCompactSourceReferenceBlock(sourceRefs) {
  const refs = (sourceRefs || []).filter((entry) => entry?.label);
  if (!refs.length) return "";
  const hasMarkdown = refs.some((entry) => entry.type === "markdown" && entry.markdown);
  if (!hasMarkdown) {
    return `来源：${refs.map((entry) => entry.label).join("；")}`;
  }
  return [
    "来源：",
    ...refs.map((entry) => entry.type === "markdown" && entry.markdown ? entry.markdown : entry.label),
  ].join("\n");
}

function isBulletLine(line) {
  return /^\s*(?:[-*+]|[○•◦▪■▸▹►])\s+/.test(String(line || ""));
}

function normalizeBulletContent(line) {
  return normalizeInlineSpacing(String(line || "").replace(/^\s*(?:[-*+]|[○•◦▪■▸▹►])\s+/, "").trim());
}

function normalizeInlineSpacing(line) {
  return String(line || "")
    .replace(/[ \t]+/g, " ")
    .replace(/\s+([，。！？；：])/g, "$1")
    .trim();
}

function dedupeStable(items) {
  const seen = new Set();
  const result = [];
  for (const item of items) {
    const key = String(item || "")
      .replace(/\*\*/g, "")
      .replace(/[` ]+/g, "")
      .toLowerCase();
    if (!key || seen.has(key)) continue;
    seen.add(key);
    result.push(item);
  }
  return result;
}

function isFollowUpPrompt(text) {
  return /^(如果你要|如果需要|要的话|如果你愿意)/.test(String(text || "").trim());
}

function isDuplicateParagraph(candidate, baseline) {
  const normalize = (value) => String(value || "").replace(/\*\*/g, "").replace(/\s+/g, "").trim();
  const left = normalize(candidate);
  const right = normalize(baseline);
  return Boolean(left) && left === right;
}

function normalizeMarketIntro(text) {
  return normalizeInlineSpacing(
    String(text || "")
      .replace(/\n+/g, " ")
      .replace(/（[^）]*(?:快照|报价|行情|搜索结果)[^）]*）/gu, "")
      .replace(/的搜索快照/u, "")
      .replace(/基于刚检索到的公开行情快照/u, "")
      .replace(/当前主流报价(?:大致)?在/g, "当前大致在")
      .trim(),
  );
}

function normalizeMarketJudgment(text) {
  return normalizeInlineSpacing(
    String(text || "")
      .replace(/\n+/g, " ")
      .replace(/^\s*(?:一句话判断|短判断|简判断|判断|结论)[:：]\s*/u, "")
      .replace(/^从你贴出的(?:实时)?搜索结果看[，,]?\s*/u, "")
      .replace(/^综合多方(?:实时)?报价看[，,]?\s*/u, "")
      .trim(),
  );
}

function normalizeCompactMarketJudgment(text) {
  return normalizeMarketJudgment(text)
    .replace(/^当前/u, "")
    .replace(/^(?:今天|今日)(?:还是|属于)?/u, "今天")
    .trim();
}

function mergeCompactMarketIntroAndJudgment(intro, judgment) {
  const normalizedIntro = normalizeInlineSpacing(intro);
  const normalizedJudgment = normalizeCompactMarketJudgment(judgment);
  if (!normalizedIntro) {
    return { intro: "", judgment: normalizedJudgment };
  }
  if (!normalizedJudgment) {
    return { intro: normalizedIntro, judgment: "" };
  }
  if (containsDirectionalMarketSummary(normalizedIntro)) {
    return { intro: normalizedIntro, judgment: "" };
  }
  if (normalizedIntro.length + normalizedJudgment.length <= 116) {
    const merged = normalizedIntro.replace(/[。．]\s*$/u, "");
    return {
      intro: `${merged}；${normalizedJudgment}`,
      judgment: "",
    };
  }
  return { intro: normalizedIntro, judgment: normalizedJudgment };
}

function containsDirectionalMarketSummary(text) {
  return /(偏强|偏弱|震荡|承压|突破|回落|上行|下行|偏涨|偏跌|偏空|偏多)/u.test(String(text || ""));
}

function knownMarketSourceLabels() {
  return [
    "CoinMarketCap",
    "CoinGecko",
    "Binance",
    "Crypto.com",
    "CoinCodex",
    "DigitalCoinPrice",
    "TradingView",
    "The Block",
    "新浪",
    "东方财富",
    "上证",
    "深证",
    "创业板",
  ];
}

function normalizeMarketBulletCandidate(line) {
  const bullet = normalizeBulletContent(line)
    .replace(/\*\*/g, "")
    .replace(/`/g, "")
    .replace(/\s+/g, " ")
    .trim();
  if (!bullet) return "";
  if (isFollowUpPrompt(bullet)) return "";
  if (isSourceLikeLine(line) || /https?:\/\//i.test(bullet)) return "";

  const sourceLabel = extractImplicitMarketSourceLabel(bullet);
  if (!sourceLabel) {
    return normalizeInlineSpacing(
      bullet
        .replace(/(?:页面头部(?:信息)?显示|页面(?:汇率)?数据(?:约)?|搜索(?:快照|结果)|摘要显示|摘显示)[:：]?\s*/gu, "")
        .replace(/不同站点存在抓取时点差/u, "不同来源存在抓取时点差")
        .trim(),
    );
  }

  const stripped = normalizeInlineSpacing(
    bullet
      .replace(new RegExp(`^${escapeRegExp(sourceLabel)}\\s*`, "u"), "")
      .replace(/^(?:页面头部(?:信息)?显示|页面(?:汇率)?数据(?:约)?|搜索(?:快照|结果)|转换页|摘要显示|摘显示|页面显示)[:：]?\s*/u, "")
      .trim(),
  );
  const price = extractMarketPrice(stripped);
  const pcts = extractMarketPercentages(stripped);
  const volume = extractMarketVolume(stripped);
  const range = extractMarketRange(stripped);
  const dominance = extractMarketDominance(stripped);

  const parts = [];
  if (price) parts.push(price);
  if (pcts.length) parts.push(...pcts.slice(0, 2));
  if (range) parts.push(range);
  if (volume) parts.push(volume);
  if (dominance) parts.push(dominance);

  if (parts.length) {
    return `${sourceLabel}：${parts.join("，")}`;
  }

  return `${sourceLabel}：${stripped}`.replace(/：\s*$/u, "");
}

function collectImplicitMarketSources(lines) {
  return dedupeStable(
    (lines || [])
      .map((line) => extractImplicitMarketSourceLabel(line))
      .filter(Boolean),
  );
}

function extractImplicitMarketSourceLabel(line) {
  const value = String(line || "").replace(/[`*]/g, "").trim();
  if (!value) return "";
  for (const label of knownMarketSourceLabels()) {
    if (new RegExp(`(^|\\b)${escapeRegExp(label)}(?:\\b|\\s|：|:)`, "iu").test(value)) {
      return label;
    }
  }
  return "";
}

function extractMarketPrice(text) {
  const match = String(text || "").match(/\$\s?[\d,.]+(?:\.\d+)?(?:万|k|K|M|B|T)?/u);
  return match ? normalizeInlineSpacing(match[0].replace(/\s+/g, "")) : "";
}

function extractMarketPercentages(text) {
  const value = String(text || "");
  const results = [];
  const oneHourMatch = value.match(/1小时\s*([+\-]?\d+(?:\.\d+)?%)/u);
  if (oneHourMatch) {
    results.push(`1h ${normalizePercent(oneHourMatch[1])}`);
  }
  const dayMatch = value.match(/24小时\s*([+\-]?\d+(?:\.\d+)?%)/u);
  if (dayMatch) {
    results.push(`24h ${normalizePercent(dayMatch[1])}`);
  }
  const directionalMatch = value.match(/过去\s*(?:24h|24小时)?[^。\n，,]*?(上涨|下跌)\s*([\d.]+%)/u);
  if (directionalMatch) {
    const sign = directionalMatch[1] === "上涨" ? "+" : "-";
    results.push(`24h ${sign}${directionalMatch[2]}`);
  }
  if (!results.length) {
    const genericMatch = value.match(/([+\-]\d+(?:\.\d+)?%)/);
    if (genericMatch) {
      results.push(`24h ${genericMatch[1]}`);
    }
  }
  return dedupeStable(results);
}

function normalizePercent(value) {
  const text = String(value || "").trim();
  if (!text) return "";
  if (/^[+\-]/.test(text)) return text;
  return text;
}

function extractMarketVolume(text) {
  const value = String(text || "");
  const turnoverMatch = value.match(/(?:24h|24小时)(?:成交额|成交量)[:：]?\s*([$¥]?\s?[\d,.]+(?:\.\d+)?(?:[KMBT]|亿|万)?)/iu);
  if (turnoverMatch) {
    return `24h成交额 ${normalizeInlineSpacing(turnoverMatch[1].replace(/\s+/g, ""))}`;
  }
  const marketCapMatch = value.match(/(?:市值|总市值)[:：]?\s*([$¥]?\s?[\d,.]+(?:\.\d+)?(?:[KMBT]|亿|万)?)/iu);
  if (marketCapMatch) {
    return `市值 ${normalizeInlineSpacing(marketCapMatch[1].replace(/\s+/g, ""))}`;
  }
  return "";
}

function extractMarketRange(text) {
  const match = String(text || "").match(/(?:区间|24h区间)[:：]?\s*(\$\s?[\d,.]+(?:\.\d+)?(?:万|k|K|M|B|T)?)\s*(?:-|–|—|~|～)\s*(\$\s?[\d,.]+(?:\.\d+)?(?:万|k|K|M|B|T)?)/u);
  if (!match) return "";
  const left = normalizeInlineSpacing(match[1].replace(/\s+/g, ""));
  const right = normalizeInlineSpacing(match[2].replace(/\s+/g, ""));
  return `区间 ${left}-${right}`;
}

function extractMarketDominance(text) {
  const match = String(text || "").match(/(?:占市率|Dominance)[:：]?\s*([\d.]+%)/iu);
  return match ? `市占率 ${match[1]}` : "";
}

function escapeRegExp(value) {
  return String(value || "").replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function normalizeOutlineMarkdown(text) {
  const lines = String(text || "").split("\n");
  const normalized = [];
  let orderedContext = false;

  for (let index = 0; index < lines.length; index += 1) {
    const rawLine = lines[index].replace(/\s+$/g, "");
    const trimmed = rawLine.trim();
    const nextNonEmpty = lines
      .slice(index + 1)
      .map((line) => line.trim())
      .find(Boolean) || "";

    if (!trimmed) {
      if (
        orderedContext &&
        (/^\d+\.\s+\S/.test(nextNonEmpty) ||
          /^[○•◦▪■▸▹►]\s+\S/.test(nextNonEmpty) ||
          /^[-*+]\s+\S/.test(nextNonEmpty))
      ) {
        continue;
      }
      if (normalized[normalized.length - 1] !== "") {
        normalized.push("");
      }
      orderedContext = false;
      continue;
    }

    const orderedMatch = trimmed.match(/^(\d+)\.\s+(.+)$/);
    if (orderedMatch) {
      normalized.push(`${orderedMatch[1]}. ${orderedMatch[2].trim()}`);
      orderedContext = true;
      continue;
    }

    const specialBulletMatch = trimmed.match(/^[○•◦▪■▸▹►]\s+(.+)$/);
    if (specialBulletMatch) {
      normalized.push(`${orderedContext ? "   " : ""}- ${specialBulletMatch[1].trim()}`);
      continue;
    }

    normalized.push(rawLine);
    orderedContext = false;
  }

  return normalized.join("\n");
}

const INLINE_ENTITY_PATTERN =
  /\d{4}年\d{1,2}月\d{1,2}日|\d{4}-\d{2}-\d{2}|[+\-]?\d+(?:\.\d+)?%|\b(?:[036]\d{5})\b|\b\d{4,}(?:\.\d+)?\b|\b\d+\.\d{2,}\b/gu;

function classifyInlineEntity(text) {
  const value = String(text || "");
  if (/^\d{4}年\d{1,2}月\d{1,2}日$|^\d{4}-\d{2}-\d{2}$/u.test(value)) return "date";
  if (/^[+\-]?\d+(?:\.\d+)?%$/u.test(value)) return "percent";
  if (/^(?:[036]\d{5})$/u.test(value)) return "symbol";
  return "number";
}

function tokenizeInlineEntities(text) {
  const value = String(text || "");
  if (!value) return [];
  const chunks = [];
  let lastIndex = 0;

  for (const match of value.matchAll(INLINE_ENTITY_PATTERN)) {
    const token = match[0];
    const start = match.index ?? 0;
    const end = start + token.length;
    if (start > lastIndex) {
      chunks.push({ type: "text", content: value.slice(lastIndex, start) });
    }
    chunks.push({
      type: "entity",
      content: token,
      kind: classifyInlineEntity(token),
    });
    lastIndex = end;
  }

  if (lastIndex < value.length) {
    chunks.push({ type: "text", content: value.slice(lastIndex) });
  }

  return chunks.length ? chunks : [{ type: "text", content: value }];
}

function decorateInlineEntitiesHtml(html) {
  const source = String(html || "");
  if (!source || typeof DOMParser === "undefined") return source;

  const parser = new DOMParser();
  const doc = parser.parseFromString(`<div>${source}</div>`, "text/html");
  const root = doc.body.firstElementChild;
  if (!root) return source;

  const skipTags = new Set(["A", "CODE", "PRE", "SCRIPT", "STYLE"]);

  const walk = (node) => {
    const children = Array.from(node.childNodes || []);
    for (const child of children) {
      if (child.nodeType === 3) {
        const chunks = tokenizeInlineEntities(child.nodeValue || "");
        if (!chunks.some((chunk) => chunk.type === "entity")) continue;
        const fragment = doc.createDocumentFragment();
        for (const chunk of chunks) {
          if (chunk.type === "text") {
            fragment.appendChild(doc.createTextNode(chunk.content));
            continue;
          }
          const chip = doc.createElement("span");
          chip.className = "inline-entity-chip";
          chip.setAttribute("data-entity-kind", chunk.kind);
          chip.textContent = chunk.content;
          fragment.appendChild(chip);
        }
        child.parentNode?.replaceChild(fragment, child);
        continue;
      }

      if (child.nodeType === 1) {
        const tagName = child.tagName || "";
        if (skipTags.has(tagName)) continue;
        walk(child);
      }
    }
  };

  walk(root);
  return root.innerHTML;
}

const streamingEntityChunks = computed(() => tokenizeInlineEntities(displayText.value));

function parseInlineChunks(text) {
  const regex = /\[([^\]]+)\]\(([^)]+)\)/g;
  let match;
  let lastIndex = 0;
  const chunks = [];

  while ((match = regex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      chunks.push({ type: "text", content: text.substring(lastIndex, match.index) });
    }
    chunks.push({ type: "link", label: match[1], path: match[2] });
    lastIndex = regex.lastIndex;
  }

  if (lastIndex < text.length) {
    chunks.push({ type: "text", content: text.substring(lastIndex) });
  }

  return chunks.length > 0 ? chunks : [{ type: "text", content: text }];
}

const parsedMessageChunks = computed(() => {
  const text = messageText.value;
  if (!text) return [];
  return parseInlineChunks(text);
});

const messageBlocks = computed(() => {
  const text = messageText.value;
  if (!text || renderAsCode.value) return [];

  const blocks = [];
  const lines = text.split("\n");
  let fileItems = [];

  const flushFileItems = () => {
    if (!fileItems.length) return;
    blocks.push({ type: "file-list", items: fileItems });
    fileItems = [];
  };

  const pushSpacer = () => {
    if (!blocks.length || blocks[blocks.length - 1].type === "spacer") return;
    blocks.push({ type: "spacer" });
  };

  for (const line of lines) {
    const fileMatch = line.match(/^\s*[-*]\s+\[([^\]]+)\]\(([^)]+)\)\s*$/);
    if (fileMatch) {
      fileItems.push({ label: fileMatch[1], path: fileMatch[2] });
      continue;
    }

    flushFileItems();

    if (!line.trim()) {
      pushSpacer();
      continue;
    }

    blocks.push({
      type: "text",
      chunks: parseInlineChunks(line),
    });
  }

  flushFileItems();

  return blocks.length
    ? blocks
    : [
        {
          type: "text",
          chunks: parseInlineChunks(text),
        },
      ];
});

const isCopied = ref(false);
const previewOpen = ref(false);
const previewLoading = ref(false);
const previewError = ref("");
const previewPath = ref("");
const previewContent = ref("");
const previewTruncated = ref(false);

async function handleCopy() {
  if (!messageText.value || isCopied.value) return;
  try {
    await navigator.clipboard.writeText(messageText.value);
    isCopied.value = true;
    setTimeout(() => {
      isCopied.value = false;
    }, 2000);
  } catch (err) {
    console.error("Failed to copy:", err);
  }
}

function parseFileLinkTarget(raw) {
  const value = (raw || "").trim();
  if (!value) {
    return { hostId: "server-local", path: "", line: 0 };
  }

  if (value.startsWith("remote://")) {
    try {
      const parsed = new URL(value);
      const path = decodeURIComponent(parsed.pathname || "");
      const lineMatch = parsed.hash.match(/^#L(\d+)$/i);
      return {
        hostId: parsed.host || "server-local",
        path,
        line: lineMatch ? Number(lineMatch[1]) : 0,
      };
    } catch (_err) {
      return { hostId: "server-local", path: value.replace(/^remote:\/\//, ""), line: 0 };
    }
  }

  const [pathPart, hashPart] = value.split("#", 2);
  const lineMatch = (hashPart || "").match(/^L(\d+)$/i);
  return {
    hostId: "server-local",
    path: pathPart,
    line: lineMatch ? Number(lineMatch[1]) : 0,
  };
}

function tooltipPath(raw) {
  return parseFileLinkTarget(raw).path;
}

async function openFilePreview(raw) {
  const target = parseFileLinkTarget(raw);
  if (!target.path) return;

  previewOpen.value = true;
  previewLoading.value = true;
  previewError.value = "";
  previewPath.value = target.path;
  previewContent.value = "";
  previewTruncated.value = false;

  try {
    const data = await fetchFilePreview({ hostId: target.hostId, path: target.path });
    previewPath.value = data.path || target.path;
    previewContent.value = data.content || "";
    previewTruncated.value = !!data.truncated;
  } catch (_err) {
    previewError.value = "文件预览失败";
  } finally {
    previewLoading.value = false;
  }
}

function closePreview() {
  previewOpen.value = false;
}

function autoResize(event) {
  const iframe = event?.target;
  if (!iframe) return;
  try {
    const doc = iframe.contentDocument;
    const bodyHeight = doc?.body?.scrollHeight || 0;
    const rootHeight = doc?.documentElement?.scrollHeight || 0;
    const height = Math.max(bodyHeight, rootHeight);
    if (height > 0) {
      iframe.style.height = `${height + 20}px`;
    }
  } catch (_err) {
    // sandboxed iframe may not expose its document in every browser mode.
  }
}
</script>

<template>
  <div class="message-wrapper" :class="{ 'is-user': isUser }">
    <div class="avatar assistant-avatar" v-if="!isUser">
      <BotIcon size="16" />
    </div>
    
    <div class="message-content">
      <div class="content-block relative-block assistant-thread-block assistant-document-flow" v-if="!isUser">
        <pre v-if="renderAsCode" class="message-code">{{ messageText }}</pre>
        <template v-else-if="renderAsMarkdown">
          <div
            v-if="useStreamingPlainText"
            class="message-text streaming-plain-text"
            data-testid="message-streaming-plain"
          >
            <template v-for="(chunk, idx) in streamingEntityChunks" :key="idx">
              <span v-if="chunk.type === 'text'">{{ chunk.content }}</span>
              <InlineEntityChip v-else :kind="chunk.kind" :text="chunk.content" />
            </template>
            <span class="streaming-cursor" aria-hidden="true"></span>
          </div>
          <div
            v-else
            class="message-text markdown-body"
            :class="{ 'is-streaming': isStreaming }"
            v-html="decoratedMarkdown"
          ></div>
          <div v-if="isLongMessage" class="collapse-toggle">
            <button type="button" class="expand-btn" @click="toggleCollapse">
              <template v-if="isCollapsed">
                展开全部
                <ChevronDownIcon size="14" />
              </template>
              <template v-else>
                收起
                <ChevronDownIcon size="14" class="chevron-up" />
              </template>
            </button>
          </div>
        </template>
        <div v-else class="message-text rich-message">
          <template v-for="(block, blockIdx) in messageBlocks" :key="blockIdx">
            <div v-if="block.type === 'text'" class="message-line">
              <template v-for="(chunk, idx) in block.chunks" :key="idx">
                <span v-if="chunk.type === 'text'">{{ chunk.content }}</span>
                <button
                  v-else-if="chunk.type === 'link'"
                  type="button"
                  class="file-link-text"
                  :data-path="tooltipPath(chunk.path)"
                  @click="openFilePreview(chunk.path)"
                >
                  {{ chunk.label }}
                </button>
              </template>
            </div>

            <div v-else-if="block.type === 'file-list'" class="file-list-block">
              <div v-for="item in block.items" :key="item.path" class="file-list-item">
                <button
                  type="button"
                  class="file-link-text"
                  :data-path="tooltipPath(item.path)"
                  @click="openFilePreview(item.path)"
                >
                  {{ item.label }}
                </button>
              </div>
            </div>

            <div v-else-if="block.type === 'spacer'" class="message-spacer"></div>
          </template>
        </div>
        <iframe
          v-if="mcpAppHtml"
          :srcdoc="mcpAppHtml"
          sandbox="allow-scripts"
          class="mcp-ui-card"
          data-testid="mcp-ui-card"
          @load="autoResize"
        />
        <button v-if="showCopyButton" class="copy-btn" @click="handleCopy" :class="{ copied: isCopied }" title="Copy">
          <CheckIcon v-if="isCopied" size="14" class="text-green-500" />
          <CopyIcon v-else size="14" />
          <span v-if="isCopied" class="copy-tooltip">✓ 复制成功</span>
        </button>
      </div>
      <template v-else>
        <pre v-if="renderAsCode" class="message-code">{{ messageText }}</pre>
        <div v-else class="message-text user-message-bubble">
          <template v-for="(chunk, idx) in parsedMessageChunks" :key="idx">
            <span v-if="chunk.type === 'text'">{{ chunk.content }}</span>
            <button
              v-else-if="chunk.type === 'link'"
              type="button"
              class="file-link-text"
              :data-path="tooltipPath(chunk.path)"
              @click="openFilePreview(chunk.path)"
            >
              {{ chunk.label }}
            </button>
          </template>
        </div>
      </template>
      <div class="ghost-loader" v-if="showSkeleton">
        <n-skeleton text :repeat="2" style="width: 60%" />
        <n-skeleton text style="width: 40%" />
      </div>
      <div class="ghost-loader" v-else-if="card.status === 'inProgress' && !isUser">
        <span class="streaming-cursor" />
      </div>
    </div>
    
    <div class="avatar user-avatar" v-if="isUser">
      <UserIcon size="16" />
    </div>
  </div>

  <Modal v-if="previewOpen" :title="previewPath || '文件预览'" @close="closePreview">
    <div class="preview-modal">
      <div v-if="previewLoading" class="preview-state">正在读取文件...</div>
      <div v-else-if="previewError" class="preview-error">{{ previewError }}</div>
      <template v-else>
        <pre class="preview-code">{{ previewContent }}</pre>
        <div v-if="previewTruncated" class="preview-note">文件内容过长，当前仅展示前一部分。</div>
      </template>
    </div>
  </Modal>
</template>

<style scoped>
.message-wrapper {
  display: flex;
  gap: 4px;
  max-width: 100%;
  width: 100%;
}

.message-wrapper.is-user {
  justify-content: flex-end;
}

.avatar {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: rgba(248, 250, 252, 0.98);
  border: 1px solid rgba(226, 232, 240, 0.9);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #94a3b8;
  flex-shrink: 0;
  margin-top: 2px;
}

.assistant-avatar {
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.9);
}

.user-avatar {
  background: #eef2f7;
  color: #475569;
}

.message-content {
  flex: 1;
  max-width: calc(100% - 34px);
  min-width: 0;
}

.is-user .message-content {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  max-width: min(520px, 68%);
}

.message-text {
  font-size: var(--text-body, 13.25px);
  line-height: var(--line-height-body, 1.5);
  color: #0f172a;
  white-space: pre-wrap;
  letter-spacing: 0;
}

.rich-message {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.streaming-plain-text {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.58;
}

.message-line {
  white-space: pre-wrap;
  line-height: 1.5;
}

.message-spacer {
  height: 2px;
}

.file-list-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 1px;
}

.file-list-item {
  line-height: 1.55;
}

.message-code {
  margin: 0;
  padding: 9px 12px;
  border-radius: 12px;
  border: 1px solid rgba(226, 232, 240, 0.92);
  background: rgba(248, 250, 252, 0.96);
  color: #0f172a;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.52;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.mcp-ui-card {
  width: 100%;
  border: none;
  border-radius: 12px;
  margin-top: 10px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  min-height: 200px;
  background: #fff;
}

.user-message-bubble {
  background: #f3f4f6;
  border: 1px solid rgba(226, 232, 240, 0.95);
  padding: 7px 12px;
  border-radius: 16px;
  color: #0f172a;
  display: inline-block;
  font-size: 13.25px;
  line-height: 1.5;
  box-shadow: 0 1px 1px rgba(15, 23, 42, 0.02);
}

.is-user .message-code {
  background: #f3f4f6;
  border-color: transparent;
}

.ghost-loader {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
  max-width: min(480px, 100%);
}

.streaming-cursor {
  display: inline-block;
  width: 2px;
  height: 16px;
  background: #3b82f6;
  border-radius: 1px;
  animation: blink-cursor 1s step-end infinite;
  vertical-align: text-bottom;
  margin-left: 1px;
}

@keyframes blink-cursor {
  0%, 100% { opacity: 1; }
  50% { opacity: 0; }
}

.relative-block {
  position: relative;
  display: block;
  width: min(92ch, 100%);
  max-width: 100%;
}

.assistant-document-flow {
  padding: 0;
  background: transparent;
  border: none;
  box-shadow: none;
}

.copy-btn {
  position: absolute;
  top: 0;
  right: -26px;
  background: rgba(255, 255, 255, 0.98);
  border: 1px solid rgba(226, 232, 240, 0.95);
  border-radius: 999px;
  padding: 5px;
  color: #6b7280;
  cursor: pointer;
  opacity: 0;
  transition: all 0.2s ease;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08);
}

.relative-block:hover .copy-btn,
.relative-block:focus-within .copy-btn {
  opacity: 1;
}

.copy-btn:hover {
  background: #f9fafb;
  color: #111827;
}

.copy-btn.copied {
  opacity: 1;
  border-color: #22c55e;
  background: #f0fdf4;
  color: #15803d;
}

.text-green-500 {
  color: #22c55e;
}

.copy-tooltip {
  position: absolute;
  bottom: 110%;
  right: 0;
  background: #111827;
  color: white;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
  white-space: nowrap;
  pointer-events: none;
  animation: fadeIn 0.2s ease;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

/* Collapse / Expand toggle */
.collapse-toggle {
  margin-top: 6px;
  text-align: left;
}

.expand-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 999px;
  background: #f8fafc;
  color: #2563eb;
  font-size: 12.5px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.expand-btn:hover {
  background: #eff6ff;
  border-color: #bfdbfe;
}

.chevron-up {
  transform: rotate(180deg);
}

.file-link-text {
  position: relative;
  display: inline-flex;
  align-items: center;
  padding: 0;
  background: transparent;
  border: none;
  color: #2563eb;
  font-weight: 500;
  cursor: pointer;
  text-decoration: none;
  transition: color 0.2s ease, text-decoration-color 0.2s ease;
  font: inherit;
}

.file-link-text:hover {
  color: #1d4ed8;
  text-decoration: underline;
}

.file-link-text::after {
  content: attr(data-path);
  position: absolute;
  left: 0;
  bottom: calc(100% + 8px);
  min-width: 240px;
  max-width: min(560px, 80vw);
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.96);
  color: #f8fafc;
  font-size: 12px;
  line-height: 1.5;
  white-space: normal;
  word-break: break-word;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.18);
  opacity: 0;
  pointer-events: none;
  transform: translateY(4px);
  transition: opacity 0.16s ease, transform 0.16s ease;
  z-index: 20;
}

.file-link-text:hover::after {
  opacity: 1;
  transform: translateY(0);
}

.preview-modal {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.preview-state,
.preview-error,
.preview-note {
  font-size: 13px;
  color: #64748b;
}

.preview-error {
  color: #b91c1c;
}

.preview-code {
  margin: 0;
  padding: 14px 16px;
  border-radius: 12px;
  background: #f8fafc;
  border: 1px solid #dbe3ee;
  color: #0f172a;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.6;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  max-height: 60vh;
  overflow: auto;
}

/* Markdown rendered content */
.markdown-body {
  font-size: 14.5px;
  line-height: 1.62;
  color: #111827;
  word-break: break-word;
}

.markdown-body.is-streaming :deep(p:last-child::after),
.markdown-body.is-streaming :deep(li:last-child::after),
.markdown-body.is-streaming :deep(code:last-child::after) {
  content: "";
  display: inline-block;
  width: 2px;
  height: 0.9em;
  background: #3b82f6;
  border-radius: 1px;
  margin-left: 1px;
  vertical-align: text-bottom;
  animation: blink-cursor 1s step-end infinite;
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3),
.markdown-body :deep(h4),
.markdown-body :deep(h5),
.markdown-body :deep(h6) {
  margin: 0 0 2px;
  font-weight: 600;
  line-height: 1.2;
  color: #0f172a;
}

.markdown-body :deep(h1) { font-size: 1.22em; }
.markdown-body :deep(h2) { font-size: 1.1em; }
.markdown-body :deep(h3) { font-size: 1.02em; }

.markdown-body :deep(p) {
  margin: 0 0 6px;
  line-height: 1.62;
}

.markdown-body :deep(p:last-child) {
  margin-bottom: 0;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin: 0 0 8px;
  padding-left: 18px;
}

.markdown-body :deep(li) {
  margin: 0 0 4px;
  line-height: 1.58;
}

.markdown-body :deep(li p) {
  margin: 0;
}

.markdown-body :deep(li > p:first-child) {
  display: inline;
}

.markdown-body :deep(li > p:first-child + p) {
  display: block;
  margin-top: 1px;
}

.markdown-body :deep(li > ul),
.markdown-body :deep(li > ol) {
  margin-top: 0;
  margin-bottom: 0;
}

.markdown-body :deep(ol > li::marker),
.markdown-body :deep(ul > li::marker) {
  color: #64748b;
  font-weight: 600;
}

.markdown-body :deep(code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.88em;
  background: #f1f5f9;
  padding: 1px 4px;
  border-radius: 4px;
  color: #334155;
}

.markdown-body :deep(pre) {
  margin: 3px 0;
  padding: 8px 12px;
  border-radius: 10px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  overflow-x: auto;
}

.markdown-body :deep(pre code) {
  background: transparent;
  padding: 0;
  border-radius: 0;
  font-size: 13px;
  line-height: 1.6;
  color: inherit;
}

.markdown-body :deep(blockquote) {
  margin: 4px 0;
  padding: 3px 10px;
  border-left: 3px solid #cbd5e1;
  color: #475569;
}

.markdown-body :deep(strong) {
  font-weight: 600;
}

.markdown-body :deep(table) {
  border-collapse: collapse;
  margin: 4px 0;
  font-size: 12.5px;
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  border: 1px solid #e2e8f0;
  padding: 5px 8px;
  text-align: left;
}

.markdown-body :deep(th) {
  background: #f8fafc;
  font-weight: 600;
}

.markdown-body :deep(hr) {
  border: none;
  border-top: 1px solid #e2e8f0;
  margin: 6px 0;
}

.markdown-body :deep(a) {
  color: #2563eb;
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

.markdown-body :deep(.inline-entity-chip) {
  display: inline-flex;
  align-items: center;
  margin: 0 1px;
  padding: 0 7px;
  min-height: 1.45em;
  border-radius: 999px;
  background: rgba(241, 245, 249, 0.92);
  color: #334155;
  font-size: 0.93em;
  line-height: 1.35;
  vertical-align: baseline;
}

.markdown-body :deep(.inline-entity-chip[data-entity-kind="percent"]) {
  background: rgba(226, 232, 240, 0.98);
}

.markdown-body :deep(.inline-entity-chip[data-entity-kind="symbol"]) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

@media (max-width: 900px) {
  .copy-btn {
    top: 0;
    right: 0;
  }
}
</style>
