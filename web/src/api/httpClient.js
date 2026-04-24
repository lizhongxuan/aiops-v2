export class HttpClientError extends Error {
  constructor(message, { status = 0, url = "", payload = null, code = "", cause = null } = {}) {
    super(message);
    this.name = "HttpClientError";
    this.status = status;
    this.url = url;
    this.payload = payload;
    this.code = code;
    this.cause = cause;
  }
}

function isJsonResponse(response) {
  const header = response?.headers?.get?.("content-type") || "";
  return header.includes("application/json") || header.includes("+json");
}

async function parseResponseBody(response, url) {
  if ([204, 205, 304].includes(response?.status)) {
    return {};
  }

  const canUseJson = typeof response?.json === "function";
  const canUseText = typeof response?.text === "function";
  const shouldParseJson = isJsonResponse(response);

  if (canUseText) {
    try {
      const rawText = await response.text();
      if (!rawText) {
        return {};
      }
      if (!shouldParseJson) {
        return rawText;
      }
      return JSON.parse(rawText);
    } catch (error) {
      throw new HttpClientError("Invalid JSON response", {
        status: response?.status || 0,
        url,
        code: "invalid_json",
        cause: error,
      });
    }
  }

  if (canUseJson) {
    try {
      const payload = await response.json();
      return payload ?? {};
    } catch (error) {
      throw new HttpClientError("Invalid JSON response", {
        status: response?.status || 0,
        url,
        code: "invalid_json",
        cause: error,
      });
    }
  }

  return {};
}

function buildRequestInit(options = {}) {
  const headers = {
    ...(options.headers || {}),
  };
  const hasJsonBody = options.body !== undefined && options.body !== null && !(options.body instanceof FormData);
  if (hasJsonBody && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }
  return {
    method: options.method || "GET",
    credentials: "include",
    ...options,
    headers,
    body: hasJsonBody ? JSON.stringify(options.body) : options.body,
  };
}

export function createHttpClient({ baseUrl = "", fetchImpl = (...args) => fetch(...args) } = {}) {
  async function request(path, options = {}) {
    const url = `${baseUrl}${path}`;
    const response = await fetchImpl(url, buildRequestInit(options));
    const payload = await parseResponseBody(response, url);
    if (!response.ok) {
      throw new HttpClientError(payload?.error || payload?.message || `Request failed with status ${response.status}`, {
        status: response.status,
        url,
        payload,
      });
    }
    return payload;
  }

  return {
    request,
    get(path, options = {}) {
      return request(path, { ...options, method: "GET" });
    },
    post(path, body, options = {}) {
      return request(path, { ...options, method: "POST", body });
    },
    put(path, body, options = {}) {
      return request(path, { ...options, method: "PUT", body });
    },
    delete(path, options = {}) {
      return request(path, { ...options, method: "DELETE" });
    },
  };
}

const httpClient = createHttpClient();

export default httpClient;
