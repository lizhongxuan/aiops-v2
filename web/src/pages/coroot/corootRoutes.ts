export type CorootProductPathInput = {
  projectId: string;
  view?: string;
  id?: string;
  report?: string;
  query?: Record<string, unknown>;
};

export type CorootRouteMessage = {
  type: "aiops.coroot.route.v1";
  projectId: string;
  view?: string;
  id?: string;
  report?: string;
  query?: Record<string, unknown>;
};

function encodeSegment(value: string) {
  return encodeURIComponent(value);
}

function appendQuery(path: string, query?: Record<string, unknown>) {
  const params = new URLSearchParams();
  Object.entries(query || {}).forEach(([key, value]) => {
    if (value === undefined || value === null) return;
    if (Array.isArray(value)) {
      value.forEach((item) => params.append(key, String(item)));
      return;
    }
    params.append(key, String(value));
  });
  const queryString = params.toString();
  return queryString ? `${path}?${queryString}` : path;
}

export function toCorootProductPath({ projectId, view = "applications", id, report, query }: CorootProductPathInput) {
  const segments = ["/coroot/p", projectId, view, id, report].filter((segment): segment is string => Boolean(segment));
  return appendQuery(segments.map((segment, index) => (index === 0 ? segment : encodeSegment(segment))).join("/"), query);
}

export function toCorootIframePath(location: { pathname: string; search: string }) {
  const innerPath = location.pathname.replace(/^\/coroot/, "/_coroot");
  const normalized = innerPath.match(/^\/_coroot\/p\/[^/]+$/) ? `${innerPath}/applications` : innerPath;
  const params = new URLSearchParams(location.search);
  const next = new URLSearchParams();
  next.set("embed", "1");
  params.forEach((value, key) => {
    if (key !== "embed") next.append(key, value);
  });
  return `${normalized}?${next.toString()}`;
}

export function fromCorootRouteMessage(message: unknown) {
  if (!message || typeof message !== "object") return null;
  const payload = message as Partial<CorootRouteMessage>;
  if (payload.type !== "aiops.coroot.route.v1") return null;
  if (!payload.projectId || typeof payload.projectId !== "string") return null;

  return toCorootProductPath({
    projectId: payload.projectId,
    view: typeof payload.view === "string" && payload.view ? payload.view : "applications",
    id: typeof payload.id === "string" && payload.id ? payload.id : undefined,
    report: typeof payload.report === "string" && payload.report ? payload.report : undefined,
    query: payload.query && typeof payload.query === "object" ? payload.query : undefined,
  });
}

export const fromCorootMessage = fromCorootRouteMessage;
