function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value ?? "").trim();
}

function firstText(...values) {
  for (const value of values) {
    const text = compactText(value);
    if (text) return text;
  }
  return "";
}

function toNumber(value) {
  if (value === null || value === undefined || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function hasTextLikeContent(value) {
  return Boolean(compactText(value));
}

export const TOOL_DISPLAY_KIND_LABELS = {
  text: "文本",
  kv_list: "键值",
  command: "命令",
  file_preview: "文件预览",
  file_diff_summary: "差异摘要",
  search_queries: "查询",
  link_list: "链接",
  result_stats: "结果统计",
  warning: "警告",
};

export function toolDisplayKindLabel(kind = "") {
  return TOOL_DISPLAY_KIND_LABELS[compactText(kind)] || compactText(kind) || "详情";
}

export function normalizeToolDisplayItem(item = {}, index = 0) {
  if (!item || typeof item !== "object" || Array.isArray(item)) {
    const text = compactText(item);
    return {
      id: `item-${index}`,
      label: "",
      value: text,
      text,
      query: text,
      url: "",
      path: "",
      added: null,
      removed: null,
      tone: "",
      raw: item,
    };
  }

  const label = firstText(item.label, item.key, item.name, item.title, item.field);
  const value = firstText(item.value, item.text, item.content, item.summary, item.body, item.result, item.description);
  const query = firstText(item.query, item.search, item.keyword, item.term, value);
  const url = firstText(item.url, item.href, item.link);
  const path = firstText(item.path, item.filePath, item.file, item.filename);
  const added = toNumber(item.added ?? item.insertions ?? item.plus);
  const removed = toNumber(item.removed ?? item.deletions ?? item.minus);
  const tone = firstText(item.tone, item.status);

  return {
    id: firstText(item.id, label, path, query, url, `item-${index}`),
    label,
    value,
    text: value,
    query,
    url,
    path,
    added,
    removed,
    tone,
    raw: item,
  };
}

function hasRenderableItem(item = {}) {
  return Boolean(
    hasTextLikeContent(item.label) ||
      hasTextLikeContent(item.value) ||
      hasTextLikeContent(item.text) ||
      hasTextLikeContent(item.query) ||
      hasTextLikeContent(item.url) ||
      hasTextLikeContent(item.path) ||
      item.added !== null ||
      item.removed !== null,
  );
}

export function normalizeToolDisplayBlock(block = {}, index = 0) {
  const source = asObject(block);
  const kind = compactText(source.kind || source.type).toLowerCase();
  const title = firstText(source.title, source.label, source.name, source.header);
  const text = firstText(source.text, source.summary, source.content);
  const items = asArray(source.items || source.entries || source.rows || source.fields).map((item, itemIndex) =>
    normalizeToolDisplayItem(item, itemIndex),
  );
  const metadata = asObject(source.metadata || source.meta || source.extra);
  const blockModel = {
    id: firstText(source.id, `${kind || "block"}-${index}`),
    kind,
    title,
    text,
    items,
    metadata,
    raw: source,
  };
  blockModel.hasContent = Boolean(
    blockModel.title ||
      blockModel.text ||
      blockModel.items.some(hasRenderableItem) ||
      Object.keys(blockModel.metadata).length,
  );
  return blockModel;
}

function normalizeToolDisplayFinalCard(card = {}) {
  const source = asObject(card);
  if (!Object.keys(source).length) return null;
  return {
    cardId: firstText(source.cardId, source.card_id, source.id),
    cardType: firstText(source.cardType, source.card_type, source.type),
    title: compactText(source.title),
    text: compactText(source.text),
    summary: compactText(source.summary),
    status: compactText(source.status),
    command: compactText(source.command),
    cwd: compactText(source.cwd),
    hostId: compactText(source.hostId, source.host_id),
    hostName: compactText(source.hostName, source.host_name),
    detail: asObject(source.detail),
    createdAt: compactText(source.createdAt, source.created_at),
    updatedAt: compactText(source.updatedAt, source.updated_at),
    raw: source,
  };
}

export function normalizeToolDisplayPayload(value = {}) {
  const source = asObject(value);
  if (!Object.keys(source).length) return null;

  const blocks = asArray(source.blocks || source.Blocks).map((block, index) => normalizeToolDisplayBlock(block, index));
  const summary = compactText(source.summary);
  const activity = compactText(source.activity);
  const finalCard = normalizeToolDisplayFinalCard(source.finalCard || source.final_card);
  const metadata = asObject(source.metadata || source.meta);
  const normalized = {
    summary,
    activity,
    blocks,
    finalCard,
    skipCards: Boolean(source.skipCards ?? source.skip_cards),
    metadata,
    raw: source,
  };

  const hasContent =
    Boolean(summary || activity || blocks.length || finalCard || Object.keys(metadata).length) ||
    blocks.some((block) => block.hasContent);
  return hasContent ? normalized : null;
}

