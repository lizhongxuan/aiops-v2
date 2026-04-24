import { compactText } from "./workspaceViewModel";
import {
  isMcpBundlePayload,
  isMcpUiCardPayload,
  normalizeMcpBundle,
  normalizeMcpFreshness,
  normalizeMcpPayloadErrors,
  normalizeMcpPayloadSource,
  normalizeMcpScope,
  normalizeMcpUiActions,
  normalizeMcpUiCard,
} from "./mcpUiCardModel";

const WRAPPER_KEYS = [
  "payload",
  "result",
  "data",
  "detail",
  "response",
  "body",
  "content",
  "ui",
];

const CARD_KEYS = [
  "mcpUi",
  "mcp_ui",
  "mcpCard",
  "card",
];

const BUNDLE_KEYS = [
  "mcpBundle",
  "mcp_bundle",
  "bundle",
];

const CARD_COLLECTION_KEYS = [
  "cards",
  "uiCards",
  "ui_cards",
  "mcpUiCards",
  "mcp_ui_cards",
];

const BUNDLE_COLLECTION_KEYS = [
  "bundles",
  "mcpBundles",
  "mcp_bundles",
];

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function mergeObjects(base, extra) {
  const baseObject = asObject(base);
  const extraObject = asObject(extra);
  if (!Object.keys(baseObject).length) return extraObject;
  if (!Object.keys(extraObject).length) return baseObject;
  return {
    ...baseObject,
    ...extraObject,
  };
}

function normalizeErrorList(values, source = "") {
  return normalizeMcpPayloadErrors(values)
    .map((error) => ({
      ...error,
      source: compactText(error.source || source),
    }))
    .filter((error, index, array) => {
      const key = `${error.code}|${error.message}|${error.source}`;
      return array.findIndex((item) => `${item.code}|${item.message}|${item.source}` === key) === index;
    });
}

function readEnvelopeContext(input = {}, defaults = {}, options = {}) {
  const source = asObject(input);
  const normalizedDefaults = asObject(defaults);
  const useDefaults = options.useDefaults !== false;
  const sourceValue =
    source.source ||
    source.origin ||
    source.channel ||
    source.transport ||
    source.surfaceSource ||
    source.meta?.source ||
    source.context?.source ||
    (useDefaults ? normalizedDefaults.source || "mcp" : "");
  const scopeValue =
    source.scope ||
    source.scope_hint ||
    source.meta?.scope ||
    source.context?.scope ||
    (useDefaults ? normalizedDefaults.scope : null);
  const freshnessValue =
    source.freshness ||
    source.meta?.freshness ||
    (useDefaults ? normalizedDefaults.freshness : null);
  const actionsValue =
    source.actions ||
    source.availableActions ||
    source.meta?.actions ||
    (useDefaults ? normalizedDefaults.actions : null);
  const defaultErrors = useDefaults ? normalizedDefaults.errors : null;
  const rawErrors = [
    source.error,
    ...(Array.isArray(source.errors) ? source.errors : source.errors ? [source.errors] : []),
    source.meta?.error,
    ...(Array.isArray(source.meta?.errors) ? source.meta.errors : source.meta?.errors ? [source.meta.errors] : []),
    ...(Array.isArray(defaultErrors) ? defaultErrors : defaultErrors ? [defaultErrors] : []),
  ].filter(Boolean);

  return {
    source: sourceValue ? normalizeMcpPayloadSource(sourceValue) : "",
    mcpServer: compactText(
      source.mcpServer ||
        source.mcp_server ||
        source.server ||
        source.serverName ||
        source.server_name ||
        source.meta?.mcpServer ||
        source.meta?.mcp_server ||
        (useDefaults ? normalizedDefaults.mcpServer : ""),
    ),
    scope: scopeValue ? normalizeMcpScope(scopeValue) : {},
    freshness: freshnessValue ? normalizeMcpFreshness(freshnessValue) : {},
    actions: actionsValue ? normalizeMcpUiActions(actionsValue) : [],
    errors: rawErrors.length ? normalizeErrorList(rawErrors, sourceValue ? normalizeMcpPayloadSource(sourceValue) : "") : [],
    placement: compactText(source.placement || source.meta?.placement || (useDefaults ? normalizedDefaults.placement : "")),
  };
}

function mergeEnvelopeContext(base = {}, extra = {}) {
  const left = asObject(base);
  const right = asObject(extra);
  return {
    source: compactText(right.source || left.source || "mcp"),
    mcpServer: compactText(right.mcpServer || left.mcpServer || ""),
    scope: normalizeMcpScope(mergeObjects(left.scope, right.scope)),
    freshness: normalizeMcpFreshness(mergeObjects(left.freshness, right.freshness)),
    actions: right.actions?.length ? right.actions : left.actions || [],
    errors: normalizeErrorList([...(left.errors || []), ...(right.errors || [])], compactText(right.source || left.source || "mcp")),
    placement: compactText(right.placement || left.placement || ""),
  };
}

function addItem(bucket, entry) {
  const key = `${entry.kind}:${entry.id}:${entry.placement}:${entry.sourceCardId || ""}`;
  if (bucket.keys.has(key)) return;
  bucket.keys.add(key);
  bucket.items.push(entry);
}

function cardDefaultsFromContext(context = {}, defaults = {}) {
  return {
    ...defaults,
    source: compactText(defaults.source || context.source || "mcp"),
    mcpServer: compactText(defaults.mcpServer || context.mcpServer),
    scope: mergeObjects(context.scope, defaults.scope),
    freshness: mergeObjects(context.freshness, defaults.freshness),
    actions: defaults.actions?.length ? defaults.actions : context.actions || [],
    errors: [...(context.errors || []), ...(defaults.errors || [])],
    placement: compactText(defaults.placement || context.placement),
  };
}

function buildCardEntry(payload = {}, context = {}, defaults = {}, index = 0) {
  const normalizedDefaults = cardDefaultsFromContext(context, defaults);
  const model = normalizeMcpUiCard(payload, {
    ...normalizedDefaults,
    id: compactText(payload?.id || normalizedDefaults.id || `mcp-ui-card-${index + 1}`),
  });
  const errors = normalizeErrorList(
    [
      ...(context.errors || []),
      ...(normalizedDefaults.errors || []),
      model.error,
      ...(payload?.errors || []),
      payload?.error,
    ],
    model.source,
  );

  return {
    id: model.id,
    kind: "mcp_ui_card",
    placement: model.placement,
    source: normalizeMcpPayloadSource(model.source),
    mcpServer: compactText(model.mcpServer || normalizedDefaults.mcpServer),
    freshness: model.freshness,
    scope: model.scope,
    errors,
    sourceCardId: compactText(defaults.sourceCardId),
    model: {
      ...model,
      source: normalizeMcpPayloadSource(model.source),
      error: compactText(model.error || errors[0]?.message || ""),
      errors,
    },
  };
}

function buildBundleEntry(payload = {}, context = {}, defaults = {}, index = 0) {
  const normalizedDefaults = cardDefaultsFromContext(context, defaults);
  const model = normalizeMcpBundle(payload, {
    ...normalizedDefaults,
    bundleId: compactText(payload?.bundleId || payload?.bundle_id || normalizedDefaults.bundleId || `mcp-bundle-${index + 1}`),
  });
  const placement = compactText(payload?.placement || normalizedDefaults.placement || "inline_final") || "inline_final";
  const errors = normalizeErrorList(
    [
      ...(context.errors || []),
      ...(normalizedDefaults.errors || []),
      model.error,
      ...(payload?.errors || []),
      payload?.error,
    ],
    model.source,
  );

  return {
    id: model.bundleId,
    kind: "mcp_bundle",
    placement,
    source: normalizeMcpPayloadSource(model.source),
    mcpServer: compactText(model.mcpServer || normalizedDefaults.mcpServer),
    freshness: model.freshness,
    scope: model.scope,
    errors,
    sourceCardId: compactText(defaults.sourceCardId),
    model: {
      ...model,
      source: normalizeMcpPayloadSource(model.source),
      error: compactText(model.error || errors[0]?.message || ""),
      errors,
    },
  };
}

function looksLikeDirectMcpUiCard(node) {
  if (!isMcpUiCardPayload(node)) return false;
  return Boolean(
    node?.uiKind ||
      node?.ui_kind ||
      node?.visual ||
      node?.title ||
      node?.name ||
      node?.summary ||
      node?.description ||
      node?.empty,
  );
}

function looksLikeDirectMcpBundle(node) {
  if (!isMcpBundlePayload(node)) return false;
  return Boolean(
    node?.bundleKind ||
      node?.bundle_kind ||
      node?.subject ||
      node?.sections ||
      node?.summary ||
      node?.rootCause ||
      node?.root_cause,
  );
}

function visitNode(node, context, defaults, bucket, state) {
  if (!node || typeof node !== "object") return;
  if (state.nodes.has(node)) return;
  state.nodes.add(node);

  const nextContext = mergeEnvelopeContext(context, readEnvelopeContext(node, defaults, { useDefaults: false }));

  if (looksLikeDirectMcpBundle(node)) {
    addItem(bucket, buildBundleEntry(node, nextContext, defaults, bucket.items.length));
  } else if (looksLikeDirectMcpUiCard(node)) {
    addItem(bucket, buildCardEntry(node, nextContext, defaults, bucket.items.length));
  }

  for (const key of CARD_KEYS) {
    const value = node[key];
    if (!value || typeof value !== "object") continue;
    if (looksLikeDirectMcpUiCard(value)) {
      addItem(bucket, buildCardEntry(value, nextContext, defaults, bucket.items.length));
      continue;
    }
    visitNode(value, nextContext, defaults, bucket, state);
  }

  for (const key of BUNDLE_KEYS) {
    const value = node[key];
    if (!value || typeof value !== "object") continue;
    if (looksLikeDirectMcpBundle(value)) {
      addItem(bucket, buildBundleEntry(value, nextContext, defaults, bucket.items.length));
      continue;
    }
    visitNode(value, nextContext, defaults, bucket, state);
  }

  for (const key of CARD_COLLECTION_KEYS) {
    for (const value of asArray(node[key])) {
      if (looksLikeDirectMcpUiCard(value)) {
        addItem(bucket, buildCardEntry(value, nextContext, defaults, bucket.items.length));
      } else {
        visitNode(value, nextContext, defaults, bucket, state);
      }
    }
  }

  for (const key of BUNDLE_COLLECTION_KEYS) {
    for (const value of asArray(node[key])) {
      if (looksLikeDirectMcpBundle(value)) {
        addItem(bucket, buildBundleEntry(value, nextContext, defaults, bucket.items.length));
      } else {
        visitNode(value, nextContext, defaults, bucket, state);
      }
    }
  }

  for (const key of WRAPPER_KEYS) {
    const value = node[key];
    if (Array.isArray(value)) {
      value.forEach((item) => visitNode(item, nextContext, defaults, bucket, state));
      continue;
    }
    visitNode(value, nextContext, defaults, bucket, state);
  }
}

export function adaptMcpUiPayload(input = {}, defaults = {}) {
  const initialContext = readEnvelopeContext(input, defaults, { useDefaults: true });
  const bucket = {
    keys: new Set(),
    items: [],
  };
  visitNode(input, initialContext, defaults, bucket, {
    nodes: new WeakSet(),
  });

  const cards = bucket.items
    .filter((item) => item.kind === "mcp_ui_card")
    .map((item) => item.model);
  const bundles = bucket.items
    .filter((item) => item.kind === "mcp_bundle")
    .map((item) => item.model);

  return {
    source: initialContext.source,
    mcpServer: initialContext.mcpServer,
    freshness: initialContext.freshness,
    scope: initialContext.scope,
    actions: initialContext.actions,
    errors: initialContext.errors,
    cards,
    bundles,
    items: bucket.items,
  };
}

export function adaptMcpUiPayloadFromCard(card = {}, index = 0) {
  return adaptMcpUiPayload(card, {
    sourceCardId: compactText(card?.id),
    id: compactText(card?.id || `mcp-ui-card-${index + 1}`),
    bundleId: compactText(card?.id || `mcp-bundle-${index + 1}`),
    placement: compactText(card?.placement),
    source: card?.source,
    mcpServer: card?.mcpServer || card?.mcp_server,
    scope: card?.scope,
    freshness: card?.freshness,
    errors: [
      card?.error,
      ...(Array.isArray(card?.errors) ? card.errors : card?.errors ? [card.errors] : []),
    ],
  });
}
