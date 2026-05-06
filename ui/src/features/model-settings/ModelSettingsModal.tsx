import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  Eye,
  EyeOff,
  LoaderCircle,
  Plus,
  ServerCog,
  Sparkles,
  X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { requestJSON } from "@/features/api/client";

type ModelSettingsModalProps = {
  onClose: () => void;
};

type ProviderRuntime = "anthropic" | "compatible" | "ollama";

type ProviderHealth = {
  http_status?: number;
  message?: string;
  ok?: boolean;
  status?: string;
};

type ProviderView = {
  api_key_preview?: string;
  base_url?: string;
  capabilities?: string[];
  default_model?: string;
  enabled: boolean;
  has_api_key?: boolean;
  health?: ProviderHealth;
  id: string;
  is_default?: boolean;
  name: string;
  provider: string;
  type?: string;
};

type ProviderDraft = {
  apiKey: string;
  apiKeyPreview: string;
  baseUrl: string;
  capabilities: string[];
  enabled: boolean;
  hasStoredApiKey: boolean;
  id: string;
  model: string;
  name: string;
  runtime: ProviderRuntime;
  type: string;
};

type NoticeState =
  | {
      kind: "error" | "success";
      message: string;
    }
  | null;

const NEW_PROVIDER_ID = "__new_provider__";

function normalizeRuntime(value?: string): ProviderRuntime {
  const runtime = value?.trim().toLowerCase() ?? "";
  if (runtime === "anthropic" || runtime.includes("claude")) return "anthropic";
  if (runtime === "ollama") return "ollama";
  return "compatible";
}

function runtimeLabel(runtime: ProviderRuntime) {
  switch (runtime) {
    case "anthropic":
      return "Anthropic 兼容协议";
    case "ollama":
      return "Ollama 本地接口";
    default:
      return "OpenAI 兼容协议";
  }
}

function runtimeModelPlaceholder(runtime: ProviderRuntime) {
  switch (runtime) {
    case "anthropic":
      return "例如 claude-3-5-sonnet";
    case "ollama":
      return "例如 llama3.2";
    default:
      return "例如 gpt-4o-mini";
  }
}

function runtimeBaseUrlPlaceholder(runtime: ProviderRuntime) {
  switch (runtime) {
    case "anthropic":
      return "请输入 API 基础地址，例如 https://api.anthropic.com/v1";
    case "ollama":
      return "请输入 API 基础地址，例如 http://127.0.0.1:11434/v1";
    default:
      return "请输入 API 基础地址，例如 https://api.openai.com/v1";
  }
}

function runtimeDescription(runtime: ProviderRuntime) {
  switch (runtime) {
    case "anthropic":
      return "适用于 Claude / Anthropic Messages API 接口。";
    case "ollama":
      return "适合本地模型，通常不需要 API Key。";
    default:
      return "适用于 OpenAI、通义千问百炼、OpenRouter、火山方舟等采用 chat/completions 的接口。";
  }
}

function runtimeToType(runtime: ProviderRuntime) {
  switch (runtime) {
    case "anthropic":
      return "anthropic";
    case "ollama":
      return "ollama";
    default:
      return "openai-compatible";
  }
}

function createEmptyDraft(): ProviderDraft {
  return {
    apiKey: "",
    apiKeyPreview: "",
    baseUrl: "",
    capabilities: [],
    enabled: true,
    hasStoredApiKey: false,
    id: "",
    model: "",
    name: "",
    runtime: "compatible",
    type: runtimeToType("compatible"),
  };
}

function providerToDraft(provider: ProviderView): ProviderDraft {
  const runtime = normalizeRuntime(provider.provider);

  return {
    apiKey: "",
    apiKeyPreview: provider.api_key_preview ?? "",
    baseUrl: provider.base_url ?? "",
    capabilities: provider.capabilities ?? [],
    enabled: provider.enabled,
    hasStoredApiKey: provider.has_api_key ?? false,
    id: provider.id,
    model: provider.default_model ?? "",
    name: provider.name,
    runtime,
    type: provider.type ?? runtimeToType(runtime),
  };
}

function buildPayload(draft: ProviderDraft) {
  return {
    api_key: draft.apiKey.trim(),
    base_url: draft.baseUrl.trim(),
    capabilities: draft.capabilities,
    default_model: draft.model.trim(),
    enabled: draft.enabled,
    id: draft.id.trim(),
    name: draft.name.trim(),
    provider: draft.runtime,
    type: runtimeToType(draft.runtime),
  };
}

function validateDraft(draft: ProviderDraft) {
  if (draft.name.trim() === "") return "请填写模型名称";
  if (draft.model.trim() === "") return "请填写调用模型";
  if (draft.runtime !== "ollama" && draft.apiKey.trim() === "" && !draft.hasStoredApiKey) {
    return "请填写 API Key";
  }
  return null;
}

function StatusPill({ provider }: { provider: ProviderView | null }) {
  if (!provider) return null;

  const status = provider.health?.status ?? (provider.enabled ? "ready" : "disabled");
  const label =
    status === "ready"
      ? "可用"
      : status === "reachable"
        ? "已连通"
        : status === "missing_key"
          ? "缺少密钥"
          : status === "disabled"
            ? "已停用"
            : "待检查";

  return (
    <span
      className={[
        "inline-flex items-center rounded-full px-3 py-1 text-xs font-medium",
        status === "ready" || status === "reachable"
          ? "bg-[#eefbf2] text-[#166534]"
          : "bg-[#f4f5f7] text-[#667085]",
      ].join(" ")}
    >
      {label}
    </span>
  );
}

function resolveTestFeedback(result: ProviderHealth | null) {
  if (!result) {
    return null;
  }

  const httpStatus = result.http_status;
  const isHealthyHttp = httpStatus === undefined || (httpStatus >= 200 && httpStatus < 300);
  const fallbackMessage = result.message?.trim() ?? "";
  const friendlyMessage = resolveProviderHealthMessage(result);

  if (result.ok && isHealthyHttp) {
    return {
      message: friendlyMessage || "连接成功，接口已响应。",
      tone: "success" as const,
    };
  }

  if (result.ok && httpStatus !== undefined) {
    return {
      message:
        friendlyMessage ||
        `服务可达，但返回了 HTTP ${httpStatus}。请确认接口地址和 API 协议类型是否正确。`,
      tone: "warning" as const,
    };
  }

  return {
    message: friendlyMessage || fallbackMessage || "连接测试失败",
    tone: "error" as const,
  };
}

function resolveProviderHealthMessage(result: ProviderHealth) {
  const raw = result.message?.trim() ?? "";
  const status = result.status?.trim().toLowerCase() ?? "";
  const httpStatus = result.http_status;

  if (status === "reachable" && result.ok) {
    return httpStatus ? `连接成功，接口已响应（HTTP ${httpStatus}）。` : "连接成功，接口已响应。";
  }
  if (status === "ready" && result.ok) {
    return raw || "配置已就绪，可以使用。";
  }
  if (status === "auth_error") {
    return httpStatus
      ? `接口已连通，但 API Key 无效或没有权限（HTTP ${httpStatus}）。`
      : "接口已连通，但 API Key 无效或没有权限。";
  }
  if (status === "endpoint_not_found") {
    return "接口地址返回 404，请检查是否多填或少填了 API 路径。";
  }
  if (status === "invalid_base_url") {
    return "接口地址格式不正确，请检查后再试。";
  }
  if (status === "missing_key") {
    return "请填写 API Key 后再测试连接。";
  }
  if (status === "disabled") {
    return "这个模型配置已停用，请启用后再测试。";
  }
  if (/^Endpoint responded with HTTP 2\d\d\.?$/i.test(raw)) {
    return httpStatus ? `连接成功，接口已响应（HTTP ${httpStatus}）。` : "连接成功，接口已响应。";
  }
  if (/^Endpoint responded with HTTP \d+\.?$/i.test(raw) && httpStatus) {
    return `接口已响应，但返回了 HTTP ${httpStatus}。`;
  }

  return raw;
}

function FieldLabel({ hint, label }: { hint?: string; label: string }) {
  return (
    <div className="mb-2 flex items-center justify-between gap-3">
      <label className="text-sm font-medium text-[#344054]">{label}</label>
      {hint ? <span className="text-xs text-[#98a2b3]">{hint}</span> : null}
    </div>
  );
}

export function ModelSettingsModal({ onClose }: ModelSettingsModalProps) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<ProviderDraft>(createEmptyDraft());
  const [notice, setNotice] = useState<NoticeState>(null);
  const [secretVisible, setSecretVisible] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<ProviderHealth | null>(null);

  const providersQuery = useQuery({
    queryKey: ["provider-profiles"],
    queryFn: () => requestJSON<ProviderView[]>("/providers"),
    staleTime: 5_000,
  });

  const saveProviderMutation = useMutation({
    mutationFn: (payload: ReturnType<typeof buildPayload>) =>
      requestJSON<ProviderView>("/providers", {
        body: JSON.stringify(payload),
        method: "POST",
      }),
  });

  const setDefaultMutation = useMutation({
    mutationFn: (providerRef: string) =>
      requestJSON<ProviderView>("/providers/default", {
        body: JSON.stringify({ provider_ref: providerRef }),
        method: "POST",
      }),
  });

  const testProviderMutation = useMutation({
    mutationFn: (payload: ReturnType<typeof buildPayload>) =>
      requestJSON<ProviderHealth>("/providers/test", {
        body: JSON.stringify(payload),
        method: "POST",
      }),
  });

  const providers = providersQuery.data ?? [];

  const selectedProvider = useMemo(
    () => providers.find((provider) => provider.id === selectedId) ?? null,
    [providers, selectedId],
  );

  const isCreating = selectedId === NEW_PROVIDER_ID;
  const requiresApiKey = draft.runtime !== "ollama";
  const isBusy =
    saveProviderMutation.isPending || setDefaultMutation.isPending || testProviderMutation.isPending;
  const testFeedback = resolveTestFeedback(testResult);

  useEffect(() => {
    if (selectedId !== null) return;

    if (providers.length > 0) {
      const currentDefault = providers.find((provider) => provider.is_default) ?? providers[0];
      setSelectedId(currentDefault.id);
      setDraft(providerToDraft(currentDefault));
      return;
    }

    if (providersQuery.isFetched || providersQuery.isError) {
      setSelectedId(NEW_PROVIDER_ID);
      setDraft(createEmptyDraft());
    }
  }, [providers, providersQuery.isError, providersQuery.isFetched, selectedId]);

  function resetFeedback() {
    setNotice(null);
    setTestResult(null);
  }

  async function handleSelectProvider(provider: ProviderView) {
    setSelectedId(provider.id);
    setDraft(providerToDraft(provider));
    setSecretVisible(false);
    resetFeedback();
  }

  function handleCreateProvider() {
    setSelectedId(NEW_PROVIDER_ID);
    setDraft(createEmptyDraft());
    setSecretVisible(false);
    resetFeedback();
  }

  function updateDraft<K extends keyof ProviderDraft>(key: K, value: ProviderDraft[K]) {
    setDraft((current) => {
      const next = { ...current, [key]: value };
      if (key === "runtime") {
        next.type = runtimeToType(value as ProviderRuntime);
      }
      return next;
    });
    setNotice(null);
    setTestResult(null);
  }

  async function refreshQueries() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["provider-profiles"] }),
      queryClient.invalidateQueries({ queryKey: ["workspace-overview"] }),
    ]);
  }

  async function handleSave(makeDefault: boolean) {
    const validationMessage = validateDraft(draft);
    if (validationMessage) {
      setNotice({ kind: "error", message: validationMessage });
      return;
    }

    setNotice(null);

    try {
      const savedProvider = await saveProviderMutation.mutateAsync(buildPayload(draft));
      setSelectedId(savedProvider.id);
      setDraft(providerToDraft(savedProvider));

      if (makeDefault) {
        const defaultProvider = await setDefaultMutation.mutateAsync(savedProvider.id);
        setDraft(providerToDraft(defaultProvider));
        await refreshQueries();
        onClose();
        return;
      }

      await refreshQueries();
      setNotice({
        kind: "success",
        message: `已保存 ${savedProvider.name}`,
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : "保存失败";
      setNotice({ kind: "error", message });
    }
  }

  async function handleTest() {
    const validationMessage = validateDraft({
      ...draft,
      model: draft.model.trim() || "placeholder-model",
    });

    if (validationMessage === "请填写调用模型") {
      setNotice({ kind: "error", message: "测试前请先填写调用模型" });
      return;
    }

    if (validationMessage === "请填写 API Key") {
      setNotice({ kind: "error", message: validationMessage });
      return;
    }

    try {
      setNotice(null);
      const result = await testProviderMutation.mutateAsync(buildPayload(draft));
      setTestResult(result);
    } catch (error) {
      const message = error instanceof Error ? error.message : "测试失败";
      setNotice({ kind: "error", message });
      setTestResult(null);
    }
  }

  return (
    <section
      aria-labelledby="model-settings-dialog-title"
      aria-modal="true"
      className="pointer-events-auto mx-4 flex h-[88vh] w-full max-w-[1180px] flex-col overflow-hidden rounded-[32px] border border-white/80 bg-white shadow-[0_36px_90px_rgba(15,23,42,0.18)] sm:mx-6 lg:h-[82vh]"
      role="dialog"
    >
      <header className="flex items-center justify-between gap-4 border-b border-[#eceff3] px-6 py-5">
        <div>
          <div className="text-sm font-medium text-[#667085]">模型设置</div>
          <h2 className="mt-1 text-[24px] font-semibold tracking-[-0.03em] text-[#111827]" id="model-settings-dialog-title">
            默认大模型
          </h2>
        </div>

        <button
          aria-label="关闭模型设置"
          className="flex h-11 w-11 items-center justify-center rounded-full text-[#667085] transition-colors duration-150 hover:bg-[#f3f4f6] hover:text-[#111827]"
          onClick={onClose}
          type="button"
        >
          <X size={22} strokeWidth={2.1} />
        </button>
      </header>

      <div className="grid min-h-0 flex-1 lg:grid-cols-[320px_minmax(0,1fr)]">
        <aside className="flex min-h-0 flex-col border-b border-[#eceff3] bg-[#fbfcfe] lg:border-b-0 lg:border-r">
          <div className="border-b border-[#eceff3] px-5 py-5">
            <button
              className="flex h-11 w-full items-center justify-center gap-2 rounded-[16px] bg-[#1f2430] px-4 text-sm font-medium text-white transition-transform duration-150 hover:-translate-y-0.5"
              onClick={handleCreateProvider}
              type="button"
            >
              <Plus size={16} strokeWidth={2.1} />
              <span>新建自定义配置</span>
            </button>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
            {providersQuery.isLoading ? (
              <div className="flex items-center gap-2 px-3 py-4 text-sm text-[#667085]">
                <LoaderCircle className="animate-spin" size={16} strokeWidth={2.1} />
                <span>正在读取模型配置...</span>
              </div>
            ) : null}

            {providersQuery.isError ? (
              <div className="px-3 py-4 text-sm leading-6 text-[#667085]">已有配置读取失败，但你仍然可以新建配置。</div>
            ) : null}

            {providers.map((provider) => {
              const active = provider.id === selectedId;

              return (
                <button
                  aria-pressed={active}
                  className={[
                    "mb-2 flex w-full items-start gap-3 rounded-[20px] px-4 py-3 text-left transition-colors duration-150",
                    active ? "bg-white text-[#111827] shadow-[0_10px_24px_rgba(15,23,42,0.06)]" : "text-[#475467] hover:bg-white",
                    isBusy ? "cursor-wait opacity-70" : "",
                  ].join(" ")}
                  disabled={isBusy}
                  key={provider.id}
                  onClick={() => void handleSelectProvider(provider)}
                  type="button"
                >
                  <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-[#f4f6fb] text-[#1f2430]">
                    {provider.is_default ? <CheckCircle2 size={18} strokeWidth={2.1} /> : <ServerCog size={18} strokeWidth={2.1} />}
                  </span>

                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <span className="truncate text-sm font-semibold text-[#111827]">{provider.name}</span>
                      {provider.is_default ? (
                        <span className="rounded-full bg-[#eef2ff] px-2.5 py-1 text-[11px] font-medium text-[#3b5bcc]">
                          默认
                        </span>
                      ) : null}
                    </span>
                    <span className="mt-1 block text-sm text-[#667085]">
                      {runtimeLabel(normalizeRuntime(provider.provider))}
                      {provider.default_model ? ` · ${provider.default_model}` : ""}
                    </span>
                    {provider.base_url ? (
                      <span className="mt-1 block truncate text-xs text-[#98a2b3]">{provider.base_url}</span>
                    ) : null}
                  </span>
                </button>
              );
            })}

            {!providersQuery.isLoading && providers.length === 0 ? (
              <div className="px-3 py-4 text-sm leading-6 text-[#667085]">还没有可用配置，先新建一个模型接入。</div>
            ) : null}
          </div>
        </aside>

        <div className="flex min-h-0 flex-col">
          <div className="border-b border-[#eceff3] px-6 py-5 lg:px-8">
            <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
              <div>
                <div className="flex items-center gap-2">
                  <div className="text-[22px] font-semibold tracking-[-0.03em] text-[#111827]">
                    {isCreating ? "自定义大模型" : draft.name || "模型配置"}
                  </div>
                  {!isCreating && selectedProvider?.is_default ? (
                    <span className="rounded-full bg-[#eef2ff] px-3 py-1 text-xs font-medium text-[#3b5bcc]">
                      当前默认
                    </span>
                  ) : null}
                  <StatusPill provider={selectedProvider} />
                </div>
                <div className="mt-2 text-sm text-[#667085]">{runtimeDescription(draft.runtime)}</div>
              </div>

              <div className="flex items-center gap-3">
                <button
                  className="chip-button px-4 py-2 text-sm text-[#475467]"
                  disabled={isBusy}
                  onClick={handleTest}
                  type="button"
                >
                  {testProviderMutation.isPending ? (
                    <LoaderCircle className="animate-spin" size={15} strokeWidth={2.1} />
                  ) : (
                    <Sparkles size={15} strokeWidth={2.1} />
                  )}
                  <span>测试连接</span>
                </button>
              </div>
            </div>

            {testFeedback ? (
              <div
                className={[
                  "mt-4 rounded-[16px] px-4 py-3 text-sm",
                  testFeedback.tone === "success"
                    ? "bg-[#eefbf2] text-[#166534]"
                    : testFeedback.tone === "warning"
                      ? "bg-[#fff7ed] text-[#b45309]"
                      : "bg-[#fff7ed] text-[#c2410c]",
                ].join(" ")}
              >
                {testFeedback.message}
              </div>
            ) : null}

            {notice ? (
              <div
                className={[
                  "mt-4 rounded-[16px] px-4 py-3 text-sm",
                  notice.kind === "success" ? "bg-[#eefbf2] text-[#166534]" : "bg-[#fff7ed] text-[#c2410c]",
                ].join(" ")}
              >
                {notice.message}
              </div>
            ) : null}
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-6 py-6 lg:px-8 lg:py-7">
            <div className="grid gap-6 xl:grid-cols-2">
              <div>
                <FieldLabel label="接口地址" />
                <input
                  className="h-12 w-full rounded-[16px] border border-[#dbe1ea] bg-white px-4 text-sm text-[#111827] outline-none transition-colors duration-150 placeholder:text-[#98a2b3] focus:border-[#98a2b3]"
                  onChange={(event) => updateDraft("baseUrl", event.target.value)}
                  placeholder={runtimeBaseUrlPlaceholder(draft.runtime)}
                  value={draft.baseUrl}
                />
              </div>

              <div>
                <FieldLabel label="API 协议类型" />
                <select
                  aria-label="API 协议类型"
                  className="h-12 w-full rounded-[16px] border border-[#dbe1ea] bg-white px-4 text-sm text-[#111827] outline-none transition-colors duration-150 focus:border-[#98a2b3]"
                  onChange={(event) => updateDraft("runtime", event.target.value as ProviderRuntime)}
                  value={draft.runtime}
                >
                  <option value="compatible">OpenAI 兼容协议</option>
                  <option value="anthropic">Anthropic 兼容协议</option>
                  <option value="ollama">Ollama 本地接口</option>
                </select>
              </div>

              <div className="xl:col-span-2">
                <FieldLabel hint={requiresApiKey ? "必填" : "本地模型通常不需要"} label="API Key" />
                <div className="relative">
                  <input
                    className="h-12 w-full rounded-[16px] border border-[#dbe1ea] bg-white px-4 pr-12 text-sm text-[#111827] outline-none transition-colors duration-150 placeholder:text-[#98a2b3] focus:border-[#98a2b3]"
                    onChange={(event) => updateDraft("apiKey", event.target.value)}
                    placeholder={requiresApiKey ? "请输入 API Key" : "如有需要可填写"}
                    type={secretVisible ? "text" : "password"}
                    value={draft.apiKey}
                  />
                  <button
                    aria-label={secretVisible ? "隐藏 API Key" : "显示 API Key"}
                    className="absolute right-2 top-1/2 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full text-[#98a2b3] transition-colors duration-150 hover:bg-[#f4f5f7] hover:text-[#344054]"
                    onClick={() => setSecretVisible((current) => !current)}
                    type="button"
                  >
                    {secretVisible ? <EyeOff size={16} strokeWidth={2.1} /> : <Eye size={16} strokeWidth={2.1} />}
                  </button>
                </div>

                {draft.hasStoredApiKey && draft.apiKeyPreview && draft.apiKey.trim() === "" ? (
                  <div className="mt-2 text-xs text-[#667085]">已保存密钥：{draft.apiKeyPreview}</div>
                ) : null}
              </div>

              <div>
                <FieldLabel label="模型名称" />
                <input
                  className="h-12 w-full rounded-[16px] border border-[#dbe1ea] bg-white px-4 text-sm text-[#111827] outline-none transition-colors duration-150 placeholder:text-[#98a2b3] focus:border-[#98a2b3]"
                  onChange={(event) => updateDraft("name", event.target.value)}
                  placeholder="请输入模型的显示名称"
                  value={draft.name}
                />
              </div>

              <div>
                <FieldLabel hint="实际发送给 API 的模型名" label="调用模型" />
                <input
                  className="h-12 w-full rounded-[16px] border border-[#dbe1ea] bg-white px-4 text-sm text-[#111827] outline-none transition-colors duration-150 placeholder:text-[#98a2b3] focus:border-[#98a2b3]"
                  onChange={(event) => updateDraft("model", event.target.value)}
                  placeholder={runtimeModelPlaceholder(draft.runtime)}
                  value={draft.model}
                />
              </div>
            </div>
          </div>

          <footer className="flex flex-col gap-3 border-t border-[#eceff3] px-6 py-5 sm:flex-row sm:items-center sm:justify-end lg:px-8">
            <button
              className="chip-button justify-center px-5 py-3 text-sm text-[#475467]"
              disabled={isBusy}
              onClick={onClose}
              type="button"
            >
              取消
            </button>

            <button
              className="chip-button justify-center px-5 py-3 text-sm text-[#475467] disabled:opacity-60"
              disabled={isBusy}
              onClick={() => void handleSave(false)}
              type="button"
            >
              {saveProviderMutation.isPending && !setDefaultMutation.isPending ? (
                <LoaderCircle className="animate-spin" size={15} strokeWidth={2.1} />
              ) : null}
              <span>保存配置</span>
            </button>

            <button
              className="flex items-center justify-center gap-2 rounded-full bg-[#1f2430] px-5 py-3 text-sm font-medium text-white transition-transform duration-150 hover:-translate-y-0.5 disabled:opacity-60"
              disabled={isBusy}
              onClick={() => void handleSave(true)}
              type="button"
            >
              {setDefaultMutation.isPending ? (
                <LoaderCircle className="animate-spin" size={15} strokeWidth={2.1} />
              ) : (
                <CheckCircle2 size={16} strokeWidth={2.1} />
              )}
              <span>{isCreating ? "保存并设为默认" : "设为默认模型"}</span>
            </button>
          </footer>
        </div>
      </div>
    </section>
  );
}
