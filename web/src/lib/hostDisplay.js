const SERVER_LOCAL_HOST_ID = "server-local";

function normalize(value) {
  return (value || "").trim();
}

function looksLikeIPv4(value) {
  return /^(?:\d{1,3}\.){3}\d{1,3}$/.test(normalize(value));
}

function looksLikeAddress(value) {
  const text = normalize(value);
  if (!text) return false;
  if (looksLikeIPv4(text)) return true;
  return /^[a-z0-9.-]+$/i.test(text) && /[.:]/.test(text);
}

export function resolveHostDisplay(hostLike = {}) {
  const id = normalize(hostLike.id || hostLike.hostId);
  const name = normalize(hostLike.name || hostLike.hostName);
  const address = normalize(hostLike.address);

  if (!id && !name && !address) return "";
  if (id === SERVER_LOCAL_HOST_ID) {
    if (!id || id === name) return name || id || SERVER_LOCAL_HOST_ID;
    return `${name} · ${id}`;
  }

  if (address) return address;
  if (looksLikeAddress(name)) return name;
  if (looksLikeAddress(id)) return id;
  return name || id;
}

export function resolveHostBadge(hostLike = {}, prefix = "目标主机 ") {
  const label = resolveHostDisplay(hostLike);
  return label ? `${prefix}${label}` : "";
}
