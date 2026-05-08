import { type MarketItem, type MarketKind, type MarketVersion } from "./data";

const REGISTRY_BASE_URL = "/v1";

type Envelope<T> = {
  data?: T;
  error?: {
    code?: string;
    detail?: string;
    message?: string;
  };
};

type RegistryArtifact = {
  compatibility?: {
    anyclaw_min?: string;
    arch?: string[];
    os?: string[];
  };
  hit_signals?: string[];
  id: string;
  kind: string;
  latest_version?: string;
  name: string;
  permissions?: string[];
  publisher?: string;
  risk_level?: string;
  score?: number;
  size_bytes?: number;
  source?: string;
  summary?: string;
  tags?: string[];
  trust_level?: string;
  updated_at?: string;
  version?: string;
};

export type AdminPublisherToken = {
  id: string;
  publisher_id: string;
  token?: string;
  created_at: string;
  revoked_at?: string;
};

export type AdminAuditEvent = {
  id?: number;
  event_type: string;
  artifact_id?: string;
  version?: string;
  detail?: Record<string, unknown>;
  created_at?: string;
};

export type AdminDownloadStat = {
  artifact_id: string;
  version: string;
  count: number;
  last_at?: string;
};

export type AdminArtifactForm = {
  arch: string;
  changelog: string;
  description: string;
  hitSignals: string;
  id: string;
  kind: "agent" | "skill" | "cli";
  name: string;
  os: string;
  permissions: string;
  publisher: string;
  risk: string;
  summary: string;
  tags: string;
  trust: string;
  version: string;
};

export async function listAdminArtifacts(token: string): Promise<MarketItem[]> {
  return adminJSON<{ items?: RegistryArtifact[] }>("/artifacts?limit=100&sort=updated", token).then((result) =>
    (result.items ?? []).map(normalizeArtifact).filter((item): item is MarketItem => item !== null)
  );
}

export async function listAdminVersions(id: string, token: string): Promise<MarketVersion[]> {
  return adminJSON<{ items?: MarketVersion[] }>(`/artifacts/${encodeURIComponent(id)}/versions`, token).then((result) => result.items ?? []);
}

export async function createPublisherToken(publisherID: string, token: string): Promise<AdminPublisherToken> {
  return adminJSON<AdminPublisherToken>("/admin/tokens", token, {
    body: JSON.stringify({ publisher_id: publisherID }),
    method: "POST"
  });
}

export async function listPublisherTokens(token: string): Promise<AdminPublisherToken[]> {
  return adminJSON<{ items?: AdminPublisherToken[] }>("/admin/tokens?limit=100", token).then((result) => result.items ?? []);
}

export async function revokePublisherToken(tokenID: string, token: string): Promise<void> {
  await adminJSON(`/admin/tokens/${encodeURIComponent(tokenID)}/revoke`, token, { method: "POST" });
}

export async function publishArtifact(form: AdminArtifactForm, token: string): Promise<MarketItem> {
  const artifact = {
    id: form.id.trim(),
    kind: form.kind,
    name: form.name.trim(),
    summary: form.summary.trim(),
    description_md: form.description.trim(),
    latest_version: form.version.trim(),
    source: "anyclaw-cloud",
    publisher: form.publisher.trim(),
    risk_level: form.risk,
    trust_level: form.trust,
    permissions: splitCSV(form.permissions),
    compatibility: {
      anyclaw_min: "0.1.0",
      os: splitCSV(form.os),
      arch: splitCSV(form.arch)
    },
    tags: splitCSV(form.tags),
    hit_signals: splitCSV(form.hitSignals),
    score: 0.8
  };
  const version = {
    artifact_id: artifact.id,
    version: form.version.trim(),
    changelog_md: form.changelog.trim(),
    compatibility: artifact.compatibility,
    permissions: artifact.permissions,
    permissions_diff: artifact.permissions
  };
  return adminJSON<MarketItem>("/publish", token, {
    body: JSON.stringify({ artifact, versions: [version] }),
    method: "POST"
  });
}

export async function deleteArtifact(id: string, token: string): Promise<void> {
  await adminJSON(`/admin/artifacts/${encodeURIComponent(id)}`, token, { method: "DELETE" });
}

export async function quarantineArtifact(id: string, reason: string, token: string): Promise<void> {
  await adminJSON(`/artifacts/${encodeURIComponent(id)}/quarantine`, token, {
    body: JSON.stringify({ reason: reason.trim() || "quarantined from admin console" }),
    method: "POST"
  });
}

export async function unquarantineArtifact(id: string, token: string): Promise<void> {
  await adminJSON(`/artifacts/${encodeURIComponent(id)}/unquarantine`, token, { method: "POST" });
}

export async function listAudit(token: string): Promise<AdminAuditEvent[]> {
  return adminJSON<{ items?: AdminAuditEvent[] }>("/admin/audit?limit=50", token).then((result) => result.items ?? []);
}

export async function listDownloads(token: string): Promise<AdminDownloadStat[]> {
  return adminJSON<{ items?: AdminDownloadStat[] }>("/admin/downloads?limit=50", token).then((result) => result.items ?? []);
}

async function adminJSON<T>(path: string, token: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${REGISTRY_BASE_URL}${path}`, {
    ...init,
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${token}`,
      ...(init.body ? { "Content-Type": "application/json" } : {}),
      ...init.headers
    }
  });
  const payload = (await response.json().catch(() => ({}))) as Envelope<T>;
  if (!response.ok) {
    throw new Error(payload.error?.message || `Admin request failed with HTTP ${response.status}`);
  }
  return payload.data as T;
}

function splitCSV(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizeArtifact(item: RegistryArtifact): MarketItem | null {
  if (!isMarketKind(item.kind)) return null;
  return {
    compatibility: item.compatibility,
    hitSignals: item.hit_signals ?? [],
    id: item.id,
    kind: item.kind,
    name: item.name || item.id,
    permissions: item.permissions ?? [],
    publisher: item.publisher || "未知",
    risk: item.risk_level || "unknown",
    score: item.score,
    sizeBytes: item.size_bytes,
    source: item.source || "registry",
    summary: item.summary || "Registry 能力",
    tags: item.tags ?? [],
    trust: item.trust_level || "unknown",
    updatedAt: item.updated_at,
    version: item.latest_version || item.version || "unknown"
  };
}

function isMarketKind(value: string): value is MarketKind {
  return value === "agent" || value === "skill" || value === "cli";
}
