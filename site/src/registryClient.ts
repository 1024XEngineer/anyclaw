import { type MarketItem, type MarketKind, type MarketVersion } from "./data";

const REGISTRY_BASE_URL = "/v1";

type RegistryCompatibility = {
  anyclaw_min?: string;
  os?: string[];
  arch?: string[];
};

type RegistryArtifact = {
  id: string;
  kind: string;
  name: string;
  summary?: string;
  description_md?: string;
  version?: string;
  latest_version?: string;
  source?: string;
  publisher?: string;
  risk_level?: string;
  trust_level?: string;
  permissions?: string[];
  compatibility?: RegistryCompatibility;
  dependencies?: Array<{ id: string; version_range?: string }>;
  size_bytes?: number;
  checksum_sha256?: string;
  icon_url?: string;
  tags?: string[];
  hit_signals?: string[];
  score?: number;
  updated_at?: string;
  manifest_summary?: Record<string, string>;
};

type RegistryListEnvelope = {
  data?: {
    items?: RegistryArtifact[];
    total?: number;
    limit?: number;
    offset?: number;
  };
  error?: {
    code?: string;
    message?: string;
    detail?: string;
  };
};

type RegistryVersion = {
  version: string;
  released_at?: string;
  changelog_md?: string;
  permissions_diff?: string[];
  size_bytes?: number;
  deprecated?: boolean;
};

type RegistryVersionsEnvelope = {
  data?: {
    items?: RegistryVersion[];
    total?: number;
  };
  error?: {
    code?: string;
    message?: string;
    detail?: string;
  };
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

export type RegistryListResult = {
  items: MarketItem[];
  total: number;
  endpoint: string;
};

export type RegistryDetailResult = {
  item: MarketItem | null;
  endpoint: string;
};

export type RegistryArtifactFilters = {
  arch?: string;
  kind?: MarketKind | "all";
  os?: string;
  permission?: string;
  publisher?: string;
  q?: string;
  risk?: string;
  sort?: string;
  tag?: string;
  trust?: string;
};

export async function listRegistryArtifacts(filters: RegistryArtifactFilters = {}, signal?: AbortSignal): Promise<RegistryListResult> {
  const params = new URLSearchParams();
  params.set("limit", "100");
  if (filters.kind && filters.kind !== "all") params.set("kind", filters.kind);
  if (filters.q?.trim()) params.set("q", filters.q.trim());
  if (filters.risk) params.set("risk", filters.risk);
  if (filters.trust) params.set("trust", filters.trust);
  if (filters.tag?.trim()) params.set("tag", filters.tag.trim());
  if (filters.permission?.trim()) params.set("permission", filters.permission.trim());
  if (filters.publisher?.trim()) params.set("publisher", filters.publisher.trim());
  if (filters.os?.trim()) params.set("os", filters.os.trim());
  if (filters.arch?.trim()) params.set("arch", filters.arch.trim());
  if (filters.sort && filters.sort !== "score") params.set("sort", filters.sort);
  const endpoint = `${REGISTRY_BASE_URL}/artifacts?${params.toString()}`;
  const response = await fetch(endpoint, {
    headers: { Accept: "application/json" },
    signal
  });

  const payload = (await response.json().catch(() => ({}))) as RegistryListEnvelope;
  if (!response.ok) {
    const message = payload.error?.message || `Registry request failed with HTTP ${response.status}`;
    throw new Error(message);
  }

  const items = (payload.data?.items ?? []).map(normalizeArtifact).filter((item): item is MarketItem => item !== null);
  return {
    items,
    total: payload.data?.total ?? items.length,
    endpoint
  };
}

export async function getRegistryArtifact(id: string, signal?: AbortSignal): Promise<RegistryDetailResult> {
  const endpoint = `${REGISTRY_BASE_URL}/artifacts/${encodeURIComponent(id)}`;
  const response = await fetch(endpoint, {
    headers: { Accept: "application/json" },
    signal
  });
  const payload = (await response.json().catch(() => ({}))) as { data?: RegistryArtifact; error?: { message?: string } };
  if (!response.ok) {
    const message = payload.error?.message || `Registry artifact request failed with HTTP ${response.status}`;
    throw new Error(message);
  }
  return {
    endpoint,
    item: payload.data ? normalizeArtifact(payload.data) : null
  };
}

export async function listRegistryArtifactVersions(id: string, signal?: AbortSignal): Promise<MarketVersion[]> {
  const endpoint = `${REGISTRY_BASE_URL}/artifacts/${encodeURIComponent(id)}/versions`;
  const response = await fetch(endpoint, {
    headers: { Accept: "application/json" },
    signal
  });
  const payload = (await response.json().catch(() => ({}))) as RegistryVersionsEnvelope;
  if (!response.ok) {
    const message = payload.error?.message || `Registry versions request failed with HTTP ${response.status}`;
    throw new Error(message);
  }
  return (payload.data?.items ?? []).map((item) => ({
    changelog: item.changelog_md,
    deprecated: item.deprecated,
    permissionsDiff: item.permissions_diff ?? [],
    releasedAt: item.released_at,
    sizeBytes: item.size_bytes,
    version: item.version
  }));
}

function normalizeArtifact(item: RegistryArtifact): MarketItem | null {
  if (!isMarketKind(item.kind)) {
    return null;
  }

  return {
    id: item.id,
    name: item.name || item.id,
    kind: item.kind,
    version: item.latest_version || item.version || "未知",
    summary: item.summary || item.description_md || "Registry 能力",
    source: item.source || "registry",
    publisher: item.publisher || "未知",
    risk: (item.risk_level || "unknown").trim().toLowerCase(),
    riskLabel: localizeRisk(item.risk_level),
    trust: (item.trust_level || "unknown").trim().toLowerCase(),
    trustLabel: localizeTrust(item.trust_level),
    permissions: item.permissions ?? [],
    tags: item.tags ?? [],
    hitSignals: item.hit_signals ?? [],
    compatibility: item.compatibility,
    sizeBytes: item.size_bytes,
    score: item.score,
    updatedAt: item.updated_at
  };
}

function isMarketKind(value: string): value is MarketKind {
  return value === "agent" || value === "skill" || value === "cli";
}
