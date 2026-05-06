const DEFAULT_GATEWAY_ORIGIN = "http://127.0.0.1:18789";

type ImportMetaWithEnv = ImportMeta & {
  env?: Record<string, string | undefined>;
};

function trimTrailingSlash(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function safeJsonParse(value: string) {
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return null;
  }
}

function configuredGatewayOrigin() {
  const globalOrigin =
    typeof globalThis !== "undefined"
      ? (globalThis as { __ANYCLAW_API_ORIGIN__?: string }).__ANYCLAW_API_ORIGIN__
      : undefined;
  const metaOrigin =
    typeof document !== "undefined"
      ? document.querySelector<HTMLMetaElement>('meta[name="anyclaw-api-origin"]')?.content
      : undefined;
  const envOrigin = (import.meta as ImportMetaWithEnv).env?.VITE_ANYCLAW_API_ORIGIN;
  const origin = trimTrailingSlash(globalOrigin || metaOrigin || envOrigin || DEFAULT_GATEWAY_ORIGIN);

  try {
    return new URL(origin).origin;
  } catch {
    return DEFAULT_GATEWAY_ORIGIN;
  }
}

function inputURL(input: RequestInfo | URL) {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.toString();
  if (typeof Request !== "undefined" && input instanceof Request) return input.url;
  return "";
}

function isRootRelativePath(value: string) {
  return value.startsWith("/") && !value.startsWith("//");
}

function canUseSameOriginAPI() {
  if (typeof window === "undefined") return true;
  return window.location.protocol === "http:" || window.location.protocol === "https:";
}

function currentOriginLooksLikeGateway(gatewayOrigin: string) {
  if (typeof window === "undefined") return true;
  if (!canUseSameOriginAPI()) return false;
  if (window.location.origin === gatewayOrigin) return true;

  try {
    const gatewayURL = new URL(gatewayOrigin);
    return window.location.port !== "" && window.location.port === gatewayURL.port;
  } catch {
    return false;
  }
}

function gatewayURL(path: string) {
  return new URL(path, configuredGatewayOrigin()).toString();
}

function resolveAPIInput(input: RequestInfo | URL): RequestInfo | URL {
  const url = inputURL(input);
  if (!isRootRelativePath(url) || canUseSameOriginAPI()) {
    return input;
  }
  return gatewayURL(url);
}

function canRetryViaGateway(input: RequestInfo | URL) {
  const url = inputURL(input);
  return isRootRelativePath(url) && !currentOriginLooksLikeGateway(configuredGatewayOrigin());
}

function shouldRetryResponseViaGateway(input: RequestInfo | URL, response: Response) {
  if (!canRetryViaGateway(input)) {
    return false;
  }
  if (response.status === 404 || response.status === 405) {
    return true;
  }

  const contentType = response.headers.get("Content-Type") ?? "";
  return response.ok && contentType.toLowerCase().includes("text/html");
}

function gatewayNetworkError(input: RequestInfo | URL, cause: unknown) {
  const target = isRootRelativePath(inputURL(input)) ? gatewayURL(inputURL(input)) : inputURL(input);
  const detail = cause instanceof Error && cause.message ? ` 原始错误：${cause.message}` : "";
  return new Error(`无法连接本地 AnyClaw 网关（${target}）。请确认桌面端或 gateway 已启动。${detail}`);
}

export async function apiFetch(input: RequestInfo | URL, init?: RequestInit) {
  try {
    const response = await fetch(resolveAPIInput(input), init);
    if (shouldRetryResponseViaGateway(input, response)) {
      return await fetch(gatewayURL(inputURL(input)), init);
    }
    return response;
  } catch (error) {
    if (!canRetryViaGateway(input)) {
      throw gatewayNetworkError(input, error);
    }

    try {
      return await fetch(gatewayURL(inputURL(input)), init);
    } catch (retryError) {
      throw gatewayNetworkError(input, retryError);
    }
  }
}

export async function requestJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const response = await apiFetch(input, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : null),
      ...(init?.headers ?? {}),
    },
  });

  const raw = await response.text();
  const payload = raw ? safeJsonParse(raw) : null;

  if (!response.ok) {
    const message =
      payload && typeof payload === "object" && "error" in payload && typeof payload.error === "string"
        ? payload.error
        : raw.trim() || `请求失败 (${response.status})`;
    throw new Error(message);
  }

  return payload as T;
}

export async function fetchJSONOrNull<T>(input: RequestInfo | URL): Promise<T | null> {
  try {
    const response = await apiFetch(input, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return null;
    return (await response.json()) as T;
  } catch {
    return null;
  }
}
