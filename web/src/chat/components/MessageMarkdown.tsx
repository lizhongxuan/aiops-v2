import MarkdownIt from "markdown-it";
import type { MouseEvent } from "react";

type MessageMarkdownProps = {
  text: string;
};

const markdown = new MarkdownIt({
  breaks: true,
  html: false,
  linkify: true,
});

markdown.renderer.rules.link_open = (tokens, index, options, _env, self) => {
  const token = tokens[index];
  const href = token.attrGet("href") || "";
  if (href) {
    token.attrSet("data-copy-url", href);
    token.attrSet("title", `点击复制：${href}`);
  }
  return self.renderToken(tokens, index, options);
};

export function MessageMarkdown({ text }: MessageMarkdownProps) {
  const trimmed = normalizeFinalAnswerMarkdown(normalizeReadableTimestamps(text.trim()));
  if (!trimmed) {
    return null;
  }
  return (
    <div
      className="aiops-message-markdown space-y-1.5 overflow-x-auto break-words [&_a]:cursor-copy [&_a]:font-medium [&_a]:text-blue-600 [&_a]:no-underline hover:[&_a]:text-blue-700 [&_blockquote]:border-l-2 [&_blockquote]:border-slate-300 [&_blockquote]:pl-3 [&_blockquote]:text-slate-700 [&_code]:rounded [&_code]:bg-slate-100 [&_code]:px-1 [&_code]:py-0.5 [&_em]:italic [&_li]:my-0.5 [&_li>p]:m-0 [&_ol]:list-decimal [&_ol]:pl-5 [&_ol_ul]:mt-0.5 [&_ol_ul]:pl-5 [&_p]:whitespace-pre-wrap [&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:bg-slate-950 [&_pre]:p-3 [&_pre]:text-slate-50 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_strong]:font-semibold [&_table]:my-2 [&_table]:w-full [&_table]:min-w-[560px] [&_table]:border-collapse [&_table]:text-sm [&_tbody_tr:nth-child(even)]:bg-slate-50/70 [&_td]:border [&_td]:border-slate-200 [&_td]:px-3 [&_td]:py-2 [&_td]:align-top [&_td]:text-slate-700 [&_th]:border [&_th]:border-slate-200 [&_th]:bg-slate-50 [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:font-semibold [&_th]:text-slate-700 [&_ul]:list-disc [&_ul]:pl-5 [&_ul_ul]:mt-0.5 [&_ul_ul]:pl-5"
      onClick={copyLinkInsteadOfNavigating}
      dangerouslySetInnerHTML={{ __html: markdown.render(trimmed) }}
    />
  );
}

function normalizeFinalAnswerMarkdown(value: string) {
  return normalizeLooseNestedListLabels(
    normalizeDetachedSourceLinks(
      normalizeStandaloneSectionLabels(stripRoutingMetadata(value)),
    ),
  );
}

function stripRoutingMetadata(value: string) {
  return value
    .replace(/(^|\n)\s*\{[^{}\n]*"route"\s*:\s*"[^"]*"[^{}\n]*\}\s*(?=\n|$)/g, "$1")
    .trim();
}

function normalizeStandaloneSectionLabels(value: string) {
  const labels = [
    "根因",
    "Root Cause",
    "证据",
    "关键证据",
    "支持证据",
    "反向证据",
    "缺失证据",
    "影响面",
    "下一步",
    "最小风险下一步",
    "需要审批的动作",
    "结论",
    "置信度",
    "建议",
    "处理结果",
    "当前状态",
    "风险",
    "原因",
  ];
  const labelPattern = labels.map(escapeRegExp).join("|");
  const standaloneLabelPattern = new RegExp(`^\\s*(?:#{1,6}\\s*)?(?:\\*\\*)?(${labelPattern})\\s*[：:]\\s*(?:\\*\\*)?\\s*$`, "i");
  const sectionStartPattern = new RegExp(`^\\s*(?:#{1,6}\\s*)?(?:\\*\\*)?(${labelPattern})\\s*[：:]`, "i");
  const lines = value.replace(/\r\n/g, "\n").split("\n");
  const output: string[] = [];
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const matched = line.match(standaloneLabelPattern);
    if (!matched) {
      output.push(line);
      continue;
    }
    let nextIndex = index + 1;
    while (nextIndex < lines.length && !lines[nextIndex].trim()) {
      nextIndex += 1;
    }
    const nextLine = lines[nextIndex] || "";
    const nextIsSection = sectionStartPattern.test(nextLine);
    if (!nextLine.trim() || nextIsSection || /^(\s*[-*+]|\s*\d+[.)])\s+/.test(nextLine)) {
      output.push(`**${matched[1]}：**`);
      continue;
    }
    if (output.length && output[output.length - 1].trim()) {
      output.push("");
    }
    output.push(`**${matched[1]}：** ${nextLine.trim()}`);
    output.push("");
    index = nextIndex;
  }
  return output.join("\n").replace(/\n{3,}/g, "\n\n").trim();
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function normalizeDetachedSourceLinks(value: string) {
  return value.replace(
    /(^|[\n\r])(\s*(?:来源|参考来源|数据来源|资料来源)\s*[：:])\s*(?:\r?\n){1,3}\s*((?:https?:\/\/|www\.)\S+)/g,
    "$1$2 $3",
  );
}

function normalizeLooseNestedListLabels(value: string) {
  const lines = value.split(/\r?\n/);
  const output: string[] = [];
  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const current = line.match(/^(\s*)-\s+([^：:，,。；;]{1,24})\s*$/);
    const next = lines[index + 1] || "";
    if (current && /^(\s{2,}|\t+)-\s+\S/.test(next)) {
      output.push(`${current[1]}- **${current[2].trim()}**`);
      continue;
    }
    output.push(line);
  }
  return output.join("\n");
}

function copyLinkInsteadOfNavigating(event: MouseEvent<HTMLDivElement>) {
  const target = event.target instanceof Element
    ? event.target.closest("a[data-copy-url]")
    : null;
  if (!target) {
    return;
  }
  event.preventDefault();
  const url = target.getAttribute("data-copy-url") || target.getAttribute("href") || "";
  if (url) {
    copyTextBySelection(url);
    void navigator.clipboard?.writeText(url).catch(() => undefined);
  }
}

function copyTextBySelection(value: string) {
  let handled = false;
  const handleCopy = (event: ClipboardEvent) => {
    event.clipboardData?.setData("text/plain", value);
    event.preventDefault();
    handled = true;
  };
  document.addEventListener("copy", handleCopy, { once: true });
  document.execCommand?.("copy");
  document.removeEventListener("copy", handleCopy);
  if (handled) {
    return;
  }

  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand?.("copy");
  textarea.remove();
}

function normalizeReadableTimestamps(value: string) {
  return value.replace(
    /(^|[\n\r])([^:\n：]{0,40}(?:时间|time|timestamp|updated_at)[^:\n：]{0,20}[：:])\s*Unix\s*`?([0-9]{10}|[0-9]{13})\b`?/gim,
    (match, prefix: string, label: string, rawTimestamp: string) => {
      const readable = formatUnixTimestamp(rawTimestamp);
      if (!readable) return match;
      const spacer = label.endsWith("：") ? "" : " ";
      return `${prefix}${label}${spacer}Unix ${readable}`;
    },
  ).replace(
    /(\b(?:last_)?updated_at\b|\bcreated_at\b|\bcompleted_at\b|\bstarted_at\b|\btimestamp\b|时间戳|数据源返回时间戳)\s*([=:：])\s*([0-9]{10}|[0-9]{13})\b/g,
    (match, label: string, separator: string, rawTimestamp: string) => {
      const readable = formatUnixTimestamp(rawTimestamp);
      if (!readable) return match;
      return separator === ":" ? `${label}: ${readable}` : `${label}${separator}${readable}`;
    },
  ).replace(
    /((?:Unix\s*)?时间戳)\s*`?([0-9]{10}|[0-9]{13})\b`?/g,
    (match, label: string, rawTimestamp: string) => {
      const readable = formatUnixTimestamp(rawTimestamp);
      if (!readable) return match;
      return `${label} ${readable}`;
    },
  );
}

function formatUnixTimestamp(rawTimestamp: string) {
  const timestamp = Number(rawTimestamp);
  if (!Number.isFinite(timestamp)) {
    return "";
  }
  const millis = rawTimestamp.length === 13 ? timestamp : timestamp * 1000;
  const date = new Date(millis);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return formatShanghaiTime(date);
}

function formatShanghaiTime(date: Date) {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).formatToParts(date);
  const get = (type: string) => parts.find((part) => part.type === type)?.value || "00";
  return `${get("year")}-${get("month")}-${get("day")} ${get("hour")}:${get("minute")}:${get("second")} GMT+8`;
}
