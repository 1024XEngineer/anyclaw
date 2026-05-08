import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";

import { requestJSON } from "@/features/api/client";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

export type MarketKind = "agent" | "skill" | "cli";
export type MarketSource = "local" | "cloud";
export type MarketSort = "score" | "updated" | "name";

export type MarketFilters = {
  arch: string;
  os: string;
  permission: string;
  publisher: string;
  risk: string;
  sort: MarketSort;
  tag: string;
  trust: string;
};

export type MarketDirectoryEntry = {
  chips: string[];
  footnote: string;
  id: string;
  kind: MarketKind;
  name: string;
  owner: string;
  rawStatus: string;
  source: MarketSource;
  status: string;
  summary: string;
};

export type MarketArtifactDetail = {
  capabilities?: string[];
  category?: string;
  compatibility?: {
    anyclaw_min?: string;
    arch?: string[];
    os?: string[];
  };
  dependencies?: Array<{
    id: string;
    version_range?: string;
  }>;
  description?: string;
  display_name?: string;
  hit_signals?: string[];
  id: string;
  install_hint?: string;
  kind: MarketKind;
  latest_version?: string;
  metadata?: Record<string, string>;
  name: string;
  owner?: string;
  permissions?: string[];
  risk_level?: string;
  score?: number;
  source?: MarketSource;
  source_id?: string;
  status?: string;
  tags?: string[];
  trust_level?: string;
  version?: string;
};

export type MarketVersion = {
  changelog_md?: string;
  deprecated?: boolean;
  permissions_diff?: string[];
  released_at?: string;
  size_bytes?: number;
  version: string;
};

export type MarketBinding = {
  artifact_id: string;
  created_at: string;
  id: string;
  kind: MarketKind;
  state: string;
  target_id: string;
  target_type: "main_agent" | "persistent_subagent" | "workspace" | "runtime_global";
  version: string;
};

export type MarketInstallJob = {
  artifact_id: string;
  checksum_sha256?: string;
  decision?: {
    decision: "auto" | "ask" | "block";
    high_risk_permissions?: string[];
    permissions?: string[];
    reason?: string;
    reasons?: string[];
    requires_risk_acknowledgement?: boolean;
    requires_user_confirmation?: boolean;
    risk_level?: string;
    trust_level?: string;
  };
  error?: string;
  id: string;
  progress_index?: number;
  progress_step?: string;
  progress_total?: number;
  receipt_id?: string;
  state: string;
  version?: string;
};

export type MarketUninstallResult = {
  artifact_id: string;
  previous_version?: string;
  receipt_id: string;
  removed_bindings: string[];
  removed_path?: string;
  undo_available_seconds?: number;
  uninstalled_at: string;
};

type MarketArtifactsResponse = {
  data?: {
    items?: MarketArtifactDetail[];
    total?: number;
  };
  meta?: {
    cloud_error?: string;
  };
};

type MarketArtifactsQueryResult = {
  cloudError: string;
  entries: MarketDirectoryEntry[];
};

type MarketArtifactDetailResponse = {
  data?: MarketArtifactDetail;
};

type MarketVersionsResponse = {
  data?: {
    items?: MarketVersion[];
    total?: number;
  };
};

type MarketBindingsResponse = {
  data?: {
    items?: MarketBinding[];
    total?: number;
  };
};

type MarketInstallResponse = {
  job?: MarketInstallJob;
  job_id?: string;
  reused?: boolean;
};

type MarketUninstallResponse = {
  data?: MarketUninstallResult;
};

type MarketJobResponse = {
  data?: MarketInstallJob;
};

const STATUS_LABELS: Record<string, string> = {
  active: "已启用",
  available: "可安装",
  bound: "已绑定",
  disabled: "已禁用",
  error: "错误",
  failed: "失败",
  installed: "已安装",
  installing: "安装中",
  quarantined: "已隔离",
  rolled_back: "已回滚",
  succeeded: "成功",
};

const RISK_LABELS: Record<string, string> = {
  high: "高",
  low: "低",
  medium: "中",
  unknown: "未知",
};

const TRUST_LABELS: Record<string, string> = {
  community: "社区",
  unknown: "未知",
  unverified: "未验证",
  verified: "已验证",
};

function localizeRisk(value: string | undefined) {
  const key = (value ?? "").trim().toLowerCase();
  return RISK_LABELS[key] ?? value ?? "未知";
}

function localizeTrust(value: string | undefined) {
  const key = (value ?? "").trim().toLowerCase();
  return TRUST_LABELS[key] ?? value ?? "未知";
}

function localizeArtifactMetadata(artifact: MarketArtifactDetail): MarketArtifactDetail {
  return {
    ...artifact,
    risk_level: localizeRisk(artifact.risk_level),
    trust_level: localizeTrust(artifact.trust_level),
  };
}

function normalizeKind(value: string | null): MarketKind {
  if (value === "skill") return "skill";
  if (value === "cli") return "cli";
  return "agent";
}

function normalizeSource(value: string | null): MarketSource {
  return value === "local" ? "local" : "cloud";
}

function normalizeSort(value: string | null): MarketSort {
  if (value === "updated" || value === "name") return value;
  return "score";
}

function kindLabel(kind: MarketKind) {
  if (kind === "skill") return "技能";
  if (kind === "cli") return "命令行";
  return "代理";
}

function sourceLabel(source: MarketSource) {
  return source === "local" ? "本地" : "云端";
}

function displayStatus(value: string | undefined) {
  const key = (value ?? "").trim().toLowerCase();
  return STATUS_LABELS[key] ?? value ?? "未知";
}

function normalizeArtifact(artifact: MarketArtifactDetail): MarketDirectoryEntry {
  const metadata = artifact.metadata ?? {};
  const rawStatus = (artifact.status ?? "").trim().toLowerCase();
  const chips = [
    artifact.version ? `v${artifact.version}` : "",
    artifact.category,
    ...(artifact.tags ?? []).slice(0, 3),
    ...(artifact.permissions ?? []).slice(0, 2),
  ].filter(Boolean) as string[];

  return {
    chips,
    footnote: artifact.install_hint || metadata.working_dir || metadata.entrypoint || artifact.source_id || "",
    id: artifact.id,
    kind: artifact.kind,
    name: artifact.display_name || artifact.name,
    owner: artifact.owner || artifact.source_id || artifact.source || "本地",
    rawStatus,
    source: artifact.source ?? "local",
    status: displayStatus(artifact.status),
    summary: artifact.description || artifact.capabilities?.join(" / ") || "暂无描述",
  };
}

function terminalJobState(state: string | undefined) {
  return ["succeeded", "failed", "canceled", "rolled_back", "interrupted"].includes((state ?? "").toLowerCase());
}

function installAllowed(entry: MarketDirectoryEntry | null) {
  return Boolean(entry && entry.source === "cloud" && !["installed", "bound", "active", "installing"].includes(entry.rawStatus));
}

async function loadMarketArtifacts(kind: MarketKind, source: MarketSource, query: string, filters: MarketFilters): Promise<MarketArtifactsQueryResult> {
  const params = new URLSearchParams();
  params.set("kind", kind);
  params.set("source", source);
  params.set("limit", "100");
  if (query.trim() !== "") params.set("q", query.trim());
  if (filters.risk) params.set("risk", filters.risk);
  if (filters.trust) params.set("trust", filters.trust);
  if (filters.tag) params.set("tag", filters.tag);
  if (filters.permission) params.set("permission", filters.permission);
  if (filters.publisher) params.set("publisher", filters.publisher);
  if (filters.os) params.set("os", filters.os);
  if (filters.arch) params.set("arch", filters.arch);
  if (filters.sort !== "score") params.set("sort", filters.sort);

  const payload = await requestJSON<MarketArtifactsResponse>(`/market/artifacts?${params.toString()}`);
  return {
    cloudError: payload.meta?.cloud_error ?? "",
    entries: (payload.data?.items ?? []).map(normalizeArtifact),
  };
}

export function useMarketDirectory() {
  const queryClient = useQueryClient();
  const { data } = useWorkspaceOverview();
  const [searchParams, setSearchParams] = useSearchParams();
  const [activeJobId, setActiveJobId] = useState<string | null>(null);
  const [actionError, setActionError] = useState("");
  const [lastUninstall, setLastUninstall] = useState<MarketUninstallResult | null>(null);

  const kind = normalizeKind(searchParams.get("kind"));
  const source = normalizeSource(searchParams.get("source"));
  const query = (searchParams.get("q") ?? "").trim();
  const filters: MarketFilters = {
    arch: (searchParams.get("arch") ?? "").trim(),
    os: (searchParams.get("os") ?? "").trim(),
    permission: (searchParams.get("permission") ?? "").trim(),
    publisher: (searchParams.get("publisher") ?? "").trim(),
    risk: (searchParams.get("risk") ?? "").trim(),
    sort: normalizeSort(searchParams.get("sort")),
    tag: (searchParams.get("tag") ?? "").trim(),
    trust: (searchParams.get("trust") ?? "").trim(),
  };

  const artifactsQuery = useQuery({
    queryKey: ["market", "artifacts", kind, source, query, filters],
    queryFn: () => loadMarketArtifacts(kind, source, query, filters),
    placeholderData: { cloudError: "", entries: [] } satisfies MarketArtifactsQueryResult,
    staleTime: 5000,
  });

  const entries = artifactsQuery.data?.entries ?? [];
  const selectedParam = searchParams.get("selected");
  const selectedId = entries.some((entry) => entry.id === selectedParam) ? selectedParam : (entries[0]?.id ?? null);
  const selectedEntry = entries.find((entry) => entry.id === selectedId) ?? null;

  const detailQuery = useQuery({
    enabled: Boolean(selectedId),
    queryKey: ["market", "artifact-detail", selectedId, source],
    queryFn: async () => {
      const params = new URLSearchParams();
      params.set("source", source);
      const payload = await requestJSON<MarketArtifactDetailResponse>(`/market/artifacts/${selectedId ?? ""}?${params.toString()}`);
      return payload.data ? localizeArtifactMetadata(payload.data) : null;
    },
    staleTime: 5000,
  });

  const versionsQuery = useQuery({
    enabled: Boolean(selectedId),
    queryKey: ["market", "artifact-versions", selectedId, source],
    queryFn: async () => {
      const params = new URLSearchParams();
      params.set("source", source);
      const payload = await requestJSON<MarketVersionsResponse>(`/market/artifacts/${selectedId ?? ""}/versions?${params.toString()}`);
      return payload.data?.items ?? [];
    },
    staleTime: 5000,
  });

  const bindingsQuery = useQuery({
    queryKey: ["market", "bindings"],
    queryFn: async () => {
      const payload = await requestJSON<MarketBindingsResponse>("/market/bindings");
      return payload.data?.items ?? [];
    },
    staleTime: 5000,
  });

  const jobQuery = useQuery({
    enabled: Boolean(activeJobId),
    queryKey: ["market", "jobs", activeJobId],
    queryFn: async () => {
      const payload = await requestJSON<MarketJobResponse>(`/market/jobs/${activeJobId ?? ""}`);
      return payload.data ?? null;
    },
    refetchInterval: (queryInfo) => (terminalJobState(queryInfo.state.data?.state) ? false : 1200),
  });

  useEffect(() => {
    if (!terminalJobState(jobQuery.data?.state)) return;
    void queryClient.invalidateQueries({ queryKey: ["market", "artifacts"] });
    void queryClient.invalidateQueries({ queryKey: ["market", "artifact-detail"] });
    void queryClient.invalidateQueries({ queryKey: ["market", "bindings"] });
  }, [jobQuery.data?.state, queryClient]);

  const installMutation = useMutation({
    mutationFn: async (artifactId: string) => {
      const payload = await requestJSON<MarketInstallResponse>("/market/install", {
        body: JSON.stringify({ artifact_id: artifactId, risk_acknowledged: true, user_confirmed: true }),
        headers: {
          "Idempotency-Key": `ui-${artifactId}-${Date.now()}`,
        },
        method: "POST",
      });
      return payload;
    },
    onError: (error) => setActionError(error instanceof Error ? error.message : "安装失败"),
    onSuccess: (payload) => {
      setActionError("");
      setLastUninstall(null);
      if (payload.job_id) setActiveJobId(payload.job_id);
    },
  });

  const bindMutation = useMutation({
    mutationFn: async (targetType: MarketBinding["target_type"]) => {
      if (!selectedId) throw new Error("未选择能力");
      return requestJSON<{ data?: MarketBinding }>("/market/bindings", {
        body: JSON.stringify({ artifact_id: selectedId, target_type: targetType }),
        method: "POST",
      });
    },
    onError: (error) => setActionError(error instanceof Error ? error.message : "绑定失败"),
    onSuccess: async () => {
      setActionError("");
      setLastUninstall(null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["market", "artifacts"] }),
        queryClient.invalidateQueries({ queryKey: ["market", "artifact-detail"] }),
        queryClient.invalidateQueries({ queryKey: ["market", "bindings"] }),
      ]);
    },
  });

  const upgradeMutation = useMutation({
    mutationFn: async (version: string) => {
      if (!selectedId) throw new Error("未选择能力");
      const payload = await requestJSON<MarketInstallResponse>("/market/upgrade", {
        body: JSON.stringify({ artifact_id: selectedId, risk_acknowledged: true, user_confirmed: true, version_constraint: version }),
        headers: {
          "Idempotency-Key": `ui-upgrade-${selectedId}-${version}-${Date.now()}`,
        },
        method: "POST",
      });
      return payload;
    },
    onError: (error) => setActionError(error instanceof Error ? error.message : "升级失败"),
    onSuccess: (payload) => {
      setActionError("");
      setLastUninstall(null);
      if (payload.job_id) setActiveJobId(payload.job_id);
    },
  });

  const uninstallMutation = useMutation({
    mutationFn: async () => {
      if (!selectedId) throw new Error("未选择能力");
      return requestJSON<MarketUninstallResponse>("/market/uninstall", {
        body: JSON.stringify({ artifact_id: selectedId }),
        method: "POST",
      });
    },
    onError: (error) => setActionError(error instanceof Error ? error.message : "卸载失败"),
    onSuccess: async (payload) => {
      setActionError("");
      setLastUninstall(payload.data ?? null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["market", "artifacts"] }),
        queryClient.invalidateQueries({ queryKey: ["market", "artifact-detail"] }),
        queryClient.invalidateQueries({ queryKey: ["market", "bindings"] }),
      ]);
    },
  });

  function patchParams(
    patch: Partial<{
      arch: string;
      kind: MarketKind;
      os: string;
      permission: string;
      publisher: string;
      q: string;
      risk: string;
      selected: string | null;
      sort: MarketSort;
      source: MarketSource;
      tag: string;
      trust: string;
    }>,
  ) {
    const next = new URLSearchParams(searchParams);

    if (patch.kind !== undefined) {
      next.set("kind", patch.kind);
      next.delete("selected");
    }

    if (patch.source !== undefined) {
      next.set("source", patch.source);
      next.delete("selected");
    }

    if (patch.q !== undefined) {
      const value = patch.q.trim();
      if (value === "") next.delete("q");
      else next.set("q", value);
      next.delete("selected");
    }

    for (const key of ["risk", "trust", "tag", "permission", "publisher", "os", "arch"] as const) {
      if (patch[key] === undefined) continue;
      const value = patch[key].trim();
      if (value === "") next.delete(key);
      else next.set(key, value);
      next.delete("selected");
    }

    if (patch.sort !== undefined) {
      if (patch.sort === "score") next.delete("sort");
      else next.set("sort", patch.sort);
      next.delete("selected");
    }

    if (patch.selected !== undefined) {
      if (patch.selected) next.set("selected", patch.selected);
      else next.delete("selected");
    }

    setSearchParams(next, { replace: true });
  }

  return {
    actionError,
    bindArtifact: bindMutation.mutate,
    bindings: bindingsQuery.data ?? [],
    canBind: Boolean(selectedEntry && ["installed", "bound", "active"].includes(selectedEntry.rawStatus)),
    canInstall: installAllowed(selectedEntry),
    cloudPanels: [],
    counts: {
      cloudAgents: source === "cloud" && kind === "agent" ? entries.length : 0,
      cloudSkills: source === "cloud" && kind === "skill" ? entries.length : 0,
      localAgents: source === "local" && kind === "agent" ? entries.length : 0,
      localCLIs: source === "local" && kind === "cli" ? entries.length : 0,
      localSkills: source === "local" && kind === "skill" ? entries.length : 0,
    },
    data,
    detail: detailQuery.data ?? null,
    errorMessage: artifactsQuery.error instanceof Error ? artifactsQuery.error.message : (artifactsQuery.data?.cloudError ?? ""),
    installArtifact: () => {
      if (selectedId) installMutation.mutate(selectedId);
    },
    installJob: jobQuery.data ?? null,
    installPending: installMutation.isPending,
    isFetching: artifactsQuery.isFetching,
    isLoading: artifactsQuery.isLoading,
    filters,
    kind,
    kindLabel: kindLabel(kind),
    lastUninstall,
    localEntries: entries,
    query,
    refetch: artifactsQuery.refetch,
    selectedEntry,
    selectedId,
    setFilter: (key: keyof MarketFilters, value: string) => patchParams({ [key]: value } as Partial<MarketFilters>),
    setKind: (nextKind: MarketKind) => patchParams({ kind: nextKind }),
    setQuery: (nextQuery: string) => patchParams({ q: nextQuery }),
    setSelected: (nextSelected: string | null) => patchParams({ selected: nextSelected }),
    setSource: (nextSource: MarketSource) => patchParams({ source: nextSource }),
    source,
    sourceLabel: sourceLabel(source),
    uninstallArtifact: uninstallMutation.mutate,
    uninstallPending: uninstallMutation.isPending,
    upgradeArtifact: upgradeMutation.mutate,
    upgradePending: upgradeMutation.isPending,
    versions: versionsQuery.data ?? [],
  };
}
