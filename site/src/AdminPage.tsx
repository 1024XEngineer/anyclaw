import { AlertCircle, Clipboard, KeyRound, PackageCheck, RefreshCw, ShieldCheck, Trash2, UploadCloud } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import {
  type AdminArtifactForm,
  type AdminAuditEvent,
  type AdminDownloadStat,
  type AdminPublisherToken,
  createPublisherToken,
  deleteArtifact,
  listAdminArtifacts,
  listAdminVersions,
  listAudit,
  listDownloads,
  listPublisherTokens,
  publishArtifact,
  quarantineArtifact,
  revokePublisherToken,
  unquarantineArtifact
} from "./adminClient";
import { kindLabels, type MarketItem, type MarketVersion } from "./data";

const EMPTY_FORM: AdminArtifactForm = {
  arch: "amd64,arm64",
  changelog: "Initial release.",
  description: "",
  hitSignals: "",
  id: "",
  kind: "skill",
  name: "",
  os: "windows,linux,darwin",
  permissions: "fs.read",
  publisher: "AnyClaw Labs",
  risk: "low",
  summary: "",
  tags: "marketplace",
  trust: "verified",
  version: "1.0.0"
};

export function AdminPage() {
  const [adminToken, setAdminToken] = useState("");
  const [authed, setAuthed] = useState(false);
  const [activeTab, setActiveTab] = useState<"artifacts" | "publish" | "tokens" | "audit">("artifacts");
  const [artifacts, setArtifacts] = useState<MarketItem[]>([]);
  const [selectedID, setSelectedID] = useState("");
  const [versions, setVersions] = useState<MarketVersion[]>([]);
  const [tokens, setTokens] = useState<AdminPublisherToken[]>([]);
  const [audit, setAudit] = useState<AdminAuditEvent[]>([]);
  const [downloads, setDownloads] = useState<AdminDownloadStat[]>([]);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [publisherID, setPublisherID] = useState("AnyClaw Labs");
  const [newToken, setNewToken] = useState("");
  const [quarantineReason, setQuarantineReason] = useState("manual admin review");
  const [form, setForm] = useState<AdminArtifactForm>(EMPTY_FORM);

  const selected = useMemo(() => artifacts.find((item) => item.id === selectedID) ?? artifacts[0] ?? null, [artifacts, selectedID]);

  useEffect(() => {
    if (!selected || !authed) {
      setVersions([]);
      return;
    }
    void run(() => listAdminVersions(selected.id, adminToken).then(setVersions), { silent: true });
  }, [adminToken, authed, selected]);

  async function refreshAll() {
    if (!adminToken.trim()) {
      setError("请输入 admin token。");
      return;
    }
    await run(async () => {
      const [nextArtifacts, nextTokens, nextAudit, nextDownloads] = await Promise.all([
        listAdminArtifacts(adminToken),
        listPublisherTokens(adminToken),
        listAudit(adminToken),
        listDownloads(adminToken)
      ]);
      setArtifacts(nextArtifacts);
      setTokens(nextTokens);
      setAudit(nextAudit);
      setDownloads(nextDownloads);
      setAuthed(true);
      setSelectedID((current) => (nextArtifacts.some((item) => item.id === current) ? current : nextArtifacts[0]?.id ?? ""));
      setMessage("后台数据已刷新。");
    });
  }

  async function run(action: () => Promise<void>, options: { silent?: boolean } = {}) {
    setBusy(true);
    if (!options.silent) {
      setError("");
      setMessage("");
    }
    try {
      await action();
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败");
    } finally {
      setBusy(false);
    }
  }

  if (!authed) {
    return (
      <section className="page-section admin-page">
        <div className="page-heading">
          <p className="eyebrow">Admin</p>
          <h1>发布后台</h1>
          <p>输入 admin token 后管理 Registry。token 只保存在当前页面状态，不写入本地存储。</p>
        </div>
        <div className="admin-login">
          <KeyRound size={22} />
          <input
            autoComplete="off"
            onChange={(event) => setAdminToken(event.target.value)}
            placeholder="Admin token"
            type="password"
            value={adminToken}
          />
          <button className="primary-button compact" disabled={busy} onClick={() => void refreshAll()} type="button">
            <ShieldCheck size={16} />
            进入后台
          </button>
        </div>
        {error ? <AdminNotice tone="error" text={error} /> : null}
      </section>
    );
  }

  return (
    <section className="page-section admin-page">
      <div className="admin-header">
        <div className="page-heading">
          <p className="eyebrow">Admin</p>
          <h1>发布后台</h1>
          <p>管理 artifact、发布元数据、隔离状态、publisher token、audit 与 downloads。</p>
        </div>
        <button className="secondary-button compact" disabled={busy} onClick={() => void refreshAll()} type="button">
          <RefreshCw size={16} />
          刷新
        </button>
      </div>

      {message ? <AdminNotice tone="success" text={message} /> : null}
      {error ? <AdminNotice tone="error" text={error} /> : null}

      <div className="admin-tabs">
        {[
          ["artifacts", "Artifact 列表"],
          ["publish", "发布表单"],
          ["tokens", "Publisher Token"],
          ["audit", "Audit / Downloads"]
        ].map(([value, label]) => (
          <button className={activeTab === value ? "active" : ""} key={value} onClick={() => setActiveTab(value as typeof activeTab)} type="button">
            {label}
          </button>
        ))}
      </div>

      {activeTab === "artifacts" ? (
        <div className="admin-grid">
          <div className="admin-list">
            {artifacts.map((item) => (
              <button className={selected?.id === item.id ? "active" : ""} key={item.id} onClick={() => setSelectedID(item.id)} type="button">
                <strong>{item.name}</strong>
                <span>{item.id}</span>
                <small>
                  {kindLabels[item.kind]} · {item.publisher} · v{item.version}
                </small>
              </button>
            ))}
          </div>
          <div className="admin-panel">
            {selected ? (
              <>
                <h2>{selected.name}</h2>
                <p>{selected.summary}</p>
                <div className="admin-kv">
                  <span>ID</span>
                  <strong>{selected.id}</strong>
                  <span>风险 / 可信度</span>
                  <strong>
                    {selected.risk} / {selected.trust}
                  </strong>
                  <span>权限</span>
                  <strong>{selected.permissions.join(", ") || "未声明"}</strong>
                  <span>兼容性</span>
                  <strong>
                    {(selected.compatibility?.os ?? []).join(", ") || "any"} / {(selected.compatibility?.arch ?? []).join(", ") || "any"}
                  </strong>
                </div>
                <div className="admin-actions">
                  <input value={quarantineReason} onChange={(event) => setQuarantineReason(event.target.value)} />
                  <button onClick={() => void run(async () => { await quarantineArtifact(selected.id, quarantineReason, adminToken); await refreshAll(); })} type="button">
                    隔离
                  </button>
                  <button onClick={() => void run(async () => { await unquarantineArtifact(selected.id, adminToken); await refreshAll(); })} type="button">
                    解除隔离
                  </button>
                  <button className="danger" onClick={() => void run(async () => { await deleteArtifact(selected.id, adminToken); await refreshAll(); })} type="button">
                    <Trash2 size={15} />
                    删除
                  </button>
                </div>
                <h3>版本</h3>
                <div className="version-list">
                  {versions.map((version) => (
                    <div key={version.version}>
                      <span>v{version.version}</span>
                      <small>{version.releasedAt || "未声明"} · {version.sizeBytes ? `${version.sizeBytes} B` : "未声明"}</small>
                      {version.changelog ? <p>{version.changelog}</p> : null}
                    </div>
                  ))}
                </div>
              </>
            ) : (
              <p>暂无 artifact。</p>
            )}
          </div>
        </div>
      ) : null}

      {activeTab === "publish" ? (
        <div className="admin-panel">
          <div className="admin-form-grid">
            <AdminInput label="Artifact ID" value={form.id} onChange={(id) => setForm({ ...form, id })} />
            <label>
              <span>类型</span>
              <select value={form.kind} onChange={(event) => setForm({ ...form, kind: event.target.value as AdminArtifactForm["kind"] })}>
                <option value="agent">agent</option>
                <option value="skill">skill</option>
                <option value="cli">cli</option>
              </select>
            </label>
            <AdminInput label="名称" value={form.name} onChange={(name) => setForm({ ...form, name })} />
            <AdminInput label="版本" value={form.version} onChange={(version) => setForm({ ...form, version })} />
            <AdminInput label="发布者" value={form.publisher} onChange={(publisher) => setForm({ ...form, publisher })} />
            <AdminInput label="摘要" value={form.summary} onChange={(summary) => setForm({ ...form, summary })} />
            <AdminInput label="权限 CSV" value={form.permissions} onChange={(permissions) => setForm({ ...form, permissions })} />
            <AdminInput label="标签 CSV" value={form.tags} onChange={(tags) => setForm({ ...form, tags })} />
            <AdminInput label="命中关键词 CSV" value={form.hitSignals} onChange={(hitSignals) => setForm({ ...form, hitSignals })} />
            <AdminInput label="OS CSV" value={form.os} onChange={(os) => setForm({ ...form, os })} />
            <AdminInput label="Arch CSV" value={form.arch} onChange={(arch) => setForm({ ...form, arch })} />
            <AdminInput label="Changelog" value={form.changelog} onChange={(changelog) => setForm({ ...form, changelog })} />
            <label>
              <span>风险</span>
              <select value={form.risk} onChange={(event) => setForm({ ...form, risk: event.target.value })}>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </label>
            <label>
              <span>可信度</span>
              <select value={form.trust} onChange={(event) => setForm({ ...form, trust: event.target.value })}>
                <option value="verified">verified</option>
                <option value="community">community</option>
                <option value="unverified">unverified</option>
              </select>
            </label>
          </div>
          <label className="admin-textarea">
            <span>详细描述</span>
            <textarea value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
          </label>
          <button className="primary-button compact" disabled={busy} onClick={() => void run(async () => { await publishArtifact(form, adminToken); await refreshAll(); })} type="button">
            <UploadCloud size={16} />
            发布 Artifact
          </button>
        </div>
      ) : null}

      {activeTab === "tokens" ? (
        <div className="admin-panel">
          <div className="admin-actions">
            <input value={publisherID} onChange={(event) => setPublisherID(event.target.value)} placeholder="Publisher ID" />
            <button onClick={() => void run(async () => { const created = await createPublisherToken(publisherID, adminToken); setNewToken(created.token ?? ""); await refreshAll(); })} type="button">
              <PackageCheck size={15} />
              创建 Token
            </button>
          </div>
          {newToken ? (
            <div className="admin-secret">
              <Clipboard size={16} />
              <code>{newToken}</code>
            </div>
          ) : null}
          <div className="admin-table">
            {tokens.map((item) => (
              <div key={item.id}>
                <span>{item.id}</span>
                <span>{item.publisher_id}</span>
                <span>{item.revoked_at ? "revoked" : "active"}</span>
                <button disabled={Boolean(item.revoked_at)} onClick={() => void run(async () => { await revokePublisherToken(item.id, adminToken); await refreshAll(); })} type="button">
                  吊销
                </button>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {activeTab === "audit" ? (
        <div className="admin-grid">
          <div className="admin-panel">
            <h2>Audit</h2>
            <div className="admin-table">
              {audit.map((item) => (
                <div key={`${item.id}-${item.created_at}`}>
                  <span>{item.event_type}</span>
                  <span>{item.artifact_id || "-"}</span>
                  <span>{item.created_at || "-"}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="admin-panel">
            <h2>Downloads</h2>
            <div className="admin-table">
              {downloads.map((item) => (
                <div key={`${item.artifact_id}-${item.version}`}>
                  <span>{item.artifact_id}</span>
                  <span>v{item.version}</span>
                  <span>{item.count}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function AdminInput({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label>
      <span>{label}</span>
      <input value={value} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

function AdminNotice({ tone, text }: { tone: "success" | "error"; text: string }) {
  const Icon = tone === "success" ? ShieldCheck : AlertCircle;
  return (
    <div className={`admin-notice ${tone}`}>
      <Icon size={17} />
      <span>{text}</span>
    </div>
  );
}
