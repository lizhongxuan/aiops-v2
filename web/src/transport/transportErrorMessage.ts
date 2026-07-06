const NETWORK_ERROR_MESSAGE = "网络异常,请检查后重试";
const SERVICE_ERROR_MESSAGE = "服务异常,请稍后重试";
const REQUEST_ERROR_MESSAGE = "请求异常,请稍后重试";
const GENERIC_ERROR_MESSAGE = "请求失败,请稍后重试";
const AUTH_ERROR_MESSAGE = "权限不足,请检查授权后重试";
const CANCELED_MESSAGE = "请求已取消";
const MODEL_CONNECTION_TIMEOUT_MESSAGE = "模型服务连接超时，未能建立连接。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。";
const MODEL_RESPONSE_TIMEOUT_MESSAGE = "模型响应超时，模型服务较长时间没有返回完整结果。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。";

const HAN_CHARACTER_PATTERN = /[\u3400-\u9fff]/;

export function toUserFacingTransportErrorMessage(error: unknown): string {
  const raw = extractErrorMessage(error).trim();
  if (!raw) {
    return GENERIC_ERROR_MESSAGE;
  }

  const extracted = extractJsonMessage(raw).trim();
  if (extracted && extracted !== raw) {
    return toUserFacingTransportErrorMessage(extracted);
  }
  const lower = raw.toLowerCase();
  const modelTimeoutMessage = modelTimeoutErrorMessage(raw, lower);
  if (modelTimeoutMessage) {
    return modelTimeoutMessage;
  }
  if (HAN_CHARACTER_PATTERN.test(raw)) {
    return raw;
  }

  if (isCanceledError(lower)) {
    return CANCELED_MESSAGE;
  }
  if (isNetworkError(lower)) {
    return NETWORK_ERROR_MESSAGE;
  }
  if (isAuthError(lower)) {
    return AUTH_ERROR_MESSAGE;
  }
  if (isServiceError(lower)) {
    return SERVICE_ERROR_MESSAGE;
  }
  if (isRequestError(lower)) {
    return REQUEST_ERROR_MESSAGE;
  }
  return GENERIC_ERROR_MESSAGE;
}

export function isRawTransportErrorMessage(error: unknown): boolean {
  const raw = extractErrorMessage(error).trim();
  if (!raw) {
    return false;
  }
  const extracted = extractJsonMessage(raw).trim();
  if (extracted && extracted !== raw) {
    return isRawTransportErrorMessage(extracted);
  }
  const lower = raw.toLowerCase();
  if (modelTimeoutErrorMessage(raw, lower)) {
    return true;
  }
  if (HAN_CHARACTER_PATTERN.test(raw)) {
    return false;
  }
  return isCanceledError(lower) || isNetworkError(lower) || isAuthError(lower) || isServiceError(lower) || isRequestError(lower);
}

function extractErrorMessage(error: unknown): string {
  if (typeof error === "string") {
    return error;
  }
  if (error instanceof Error) {
    return error.message;
  }
  if (error && typeof error === "object" && "message" in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === "string") {
      return message;
    }
  }
  return "";
}

function extractJsonMessage(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed.startsWith("{")) {
    return raw;
  }
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    if (parsed && typeof parsed === "object") {
      const record = parsed as Record<string, unknown>;
      if (typeof record.message === "string") {
        return record.message;
      }
      if (record.error && typeof record.error === "object") {
        const nested = record.error as Record<string, unknown>;
        if (typeof nested.message === "string") {
          return nested.message;
        }
      }
    }
  } catch {
    return raw;
  }
  return raw;
}

function isCanceledError(lower: string): boolean {
  return lower.includes("aborterror") || lower.includes("aborted") || lower.includes("canceled") || lower.includes("cancelled");
}

function modelTimeoutErrorMessage(raw: string, lower: string): string {
  if (!hasAny(lower, ["timeout", "timed out", "超时"])) {
    return "";
  }
  if (
    hasAny(lower, [
      "模型请求超时",
      "模型服务连接超时",
      "tls handshake timeout",
      "chat/completions",
      "llm 地址",
      "llm address",
    ])
  ) {
    return MODEL_CONNECTION_TIMEOUT_MESSAGE;
  }
  if (raw.includes("模型响应超时")) {
    return MODEL_RESPONSE_TIMEOUT_MESSAGE;
  }
  return "";
}

function isNetworkError(lower: string): boolean {
  return (
    lower.includes("network error") ||
    lower.includes("networkerror") ||
    lower.includes("failed to fetch") ||
    lower.includes("fetch failed") ||
    lower.includes("load failed") ||
    lower.includes("connection refused") ||
    lower.includes("connect: connection refused") ||
    lower.includes("econnrefused") ||
    lower.includes("econnreset") ||
    lower.includes("etimedout") ||
    lower.includes("timed out") ||
    lower.includes("timeout") ||
    lower.includes("no such host") ||
    lower.includes("enotfound") ||
    lower.includes("dns")
  );
}

function hasAny(value: string, needles: string[]) {
  return needles.some((needle) => value.includes(needle));
}

function isAuthError(lower: string): boolean {
  return (
    lower.includes("401") ||
    lower.includes("403") ||
    lower.includes("unauthorized") ||
    lower.includes("forbidden") ||
    lower.includes("permission denied")
  );
}

function isServiceError(lower: string): boolean {
  return (
    lower.includes("500") ||
    lower.includes("502") ||
    lower.includes("503") ||
    lower.includes("504") ||
    lower.includes("internal server error") ||
    lower.includes("bad gateway") ||
    lower.includes("service unavailable") ||
    lower.includes("backend unavailable") ||
    lower.includes("gateway timeout") ||
    lower.includes("upstream") ||
    lower.includes("provider error")
  );
}

function isRequestError(lower: string): boolean {
  return (
    lower.includes("400") ||
    lower.includes("404") ||
    lower.includes("409") ||
    lower.includes("422") ||
    lower.includes("bad request") ||
    lower.includes("not found") ||
    lower.includes("conflict") ||
    lower.includes("unprocessable") ||
    lower.includes("reasoning_content") ||
    lower.includes("status code")
  );
}
