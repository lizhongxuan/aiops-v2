import type { AnswerSection, AnswerSectionKind } from "./types";

const LABEL_TO_KIND: Array<[RegExp, AnswerSectionKind, string]> = [
  [/^(根因|原因|结论|初步判断)$/i, "root_cause", "根因"],
  [/^(证据|直接证据|已观测|观测事实)$/i, "evidence", "证据"],
  [/^(影响面|影响|风险)$/i, "impact", "影响面"],
  [/^(下一步|建议|处置建议|处理建议)$/i, "next_steps", "下一步"],
  [/^(摘要|总结)$/i, "summary", "摘要"],
];

export function parseAnswerSections(markdown: string): AnswerSection[] {
  const lines = markdown.replace(/\r\n/g, "\n").split("\n");
  const sections: AnswerSection[] = [];
  let current: { kind: AnswerSectionKind; title?: string; lines: string[] } | null = null;

  const flush = () => {
    if (!current) {
      return;
    }
    const body = current.lines.join("\n").trim();
    if (!body && !current.title) {
      current = null;
      return;
    }
    sections.push({
      id: `${current.kind}-${sections.length}`,
      kind: current.kind,
      title: current.title,
      markdown: body,
      order: sections.length,
    });
    current = null;
  };

  for (const line of lines) {
    const match = matchSectionLabel(line);
    if (match) {
      flush();
      current = {
        kind: match.kind,
        title: match.title,
        lines: match.remainder ? [match.remainder] : [],
      };
      continue;
    }
    if (!current) {
      current = { kind: "unknown", lines: [] };
    }
    current.lines.push(line);
  }
  flush();

  return sections.length ? mergeAdjacentUnknownSections(sections) : [];
}

function matchSectionLabel(line: string): { kind: AnswerSectionKind; title: string; remainder: string } | null {
  const trimmed = line.trim();
  const normalized = trimmed
    .replace(/^#{1,6}\s*/, "")
    .replace(/^\*\*(.+?)\*\*\s*$/, "$1")
    .replace(/^\*\*(.+?[：:])\*\*\s*(.*)$/, "$1 $2")
    .trim();
  const labelMatch = normalized.match(/^(.{1,16}?)[：:]\s*(.*)$/);
  if (!labelMatch) {
    return null;
  }
  const label = labelMatch[1].trim();
  const remainder = labelMatch[2].trim();
  for (const [pattern, kind, title] of LABEL_TO_KIND) {
    if (pattern.test(label)) {
      return { kind, title, remainder };
    }
  }
  return null;
}

function mergeAdjacentUnknownSections(sections: AnswerSection[]): AnswerSection[] {
  const output: AnswerSection[] = [];
  for (const section of sections) {
    const previous = output[output.length - 1];
    if (previous?.kind === "unknown" && section.kind === "unknown") {
      previous.markdown = [previous.markdown, section.markdown].filter(Boolean).join("\n").trim();
      continue;
    }
    output.push({ ...section, order: output.length, id: `${section.kind}-${output.length}` });
  }
  return output;
}
