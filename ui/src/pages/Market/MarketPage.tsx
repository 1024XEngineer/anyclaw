import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  Cloud,
  Link2,
  PackageCheck,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Store,
  TerminalSquare,
  Trash2,
  UploadCloud,
} from "lucide-react";
import type { KeyboardEvent } from "react";
import { useState } from "react";

import { BackendDetailSection } from "@/features/backend-ui/BackendDetailSection";
import { BackendEmptyState } from "@/features/backend-ui/BackendEmptyState";
import { BackendPageHeader } from "@/features/backend-ui/BackendPageHeader";
import { BackendPropertyList } from "@/features/backend-ui/BackendPropertyList";
import { BackendSectionHeader } from "@/features/backend-ui/BackendSectionHeader";
import { BackendSummaryStrip } from "@/features/backend-ui/BackendSummaryStrip";
import { BackendToolbar } from "@/features/backend-ui/BackendToolbar";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import type { MarketArtifactDetail, MarketBinding, MarketFilters } from "@/features/market/useMarketDirectory";
import { useMarketDirectory } from "@/features/market/useMarketDirectory";

type BindingTarget = MarketBinding["target_type"];

const BINDING_TARGETS: Array<{ label: string; value: BindingTarget }> = [
  { label: "主代理", value: "main_agent" },
  { label: "工作区", value: "workspace" },
  { label: "全局运行时", value: "runtime_global" },
  { label: "持久子代理", value: "persistent_subagent" },
];

const INSTALL_STEPS = ["解析", "下载", "校验", "安装", "回执"];
const TERMINAL_ERROR_STATES = ["failed", "canceled", "rolled_back", "interrupted"];
const FILTER_INPUTS: Array<{ key: keyof MarketFilters; label: string; placeholder: string }> = [
  { key: "tag", label: "标签", placeholder: "例如 marketplace" },
  { key: "permission", label: "权限", placeholder: "例如 fs.read" },
  { key: "publisher", label: "发布者", placeholder: "发布者名称" },
  { key: "os", label: "系统", placeholder: "windows / linux / darwin" },
  { key: "arch", label: "架构", placeholder: "amd64 / arm64" },
];

function rowKeyHandler(event: KeyboardEvent<HTMLElement>, onSelect: () => void) {
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    onSelect();
  }
}

function csv(values: string[] | undefined, empty = "无") {
  return values && values.length > 0 ? values.join(", ") : empty;
}

function riskReasons(values: string[] | undefined) {
  return values && values.length > 0 ? values : ["需确认来源、权限和本地安装写入行为。"];
}

function compactStrings(values: Array<string | undefined>) {
  return values.filter((value): value is string => Boolean(value && value.trim() !== ""));
}

function formatBytes(value: number | undefined) {
  if (!value) return "未知";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${Math.round(value / 102.4) / 10} KB`;
  return `${Math.round(value / 1024 / 102.4) / 10} MB`;
}

function formatScore(value: number | undefined) {
  if (value === undefined || value === null || Number.isNaN(value)) return "--";
  return value.toFixed(2);
}

function riskLabel(artifact: MarketArtifactDetail | null | undefined) {
  return artifact?.risk_level || "鏈煡";
}

function trustLabel(artifact: MarketArtifactDetail | null | undefined) {
  return artifact?.trust_level || "鏈煡";
}

function statusTone(status: string) {
  const normalized = status.toLowerCase();
  if (["error", "failed", "rolled back", "quarantined"].some((item) => normalized.includes(item))) return "warning";
  if (["active", "bound", "installed", "succeeded"].some((item) => normalized.includes(item))) return "success";
  if (["installing", "available"].some((item) => normalized.includes(item))) return "info";
  return getStatusTone(status);
}

function bindingLabel(value: BindingTarget) {
  return BINDING_TARGETS.find((target) => target.value === value)?.label ?? value;
}

export function MarketPage() {
  const {
    actionError,
    artifactLookup,
    bindArtifact,
    bindings,
    canBind,
    canInstall,
    counts,
    data,
    detail,
    errorMessage,
    filters,
    installArtifact,
    installJob,
    installPending,
    isFetching,
    kind,
    kindLabel,
    lastUninstall,
    localEntries,
    query,
    refetch,
    selectedEntry,
    selectedId,
    setFilter,
    setKind,
    setQuery,
    setSelected,
    setSource,
    source,
    sourceLabel,
    uninstallArtifact,
    uninstallPending,
    upgradeArtifact,
    upgradePending,
    versions,
  } = useMarketDirectory();
  const [confirmInstall, setConfirmInstall] = useState(false);
  const [confirmUninstall, setConfirmUninstall] = useState(false);
  const [bindingTarget, setBindingTarget] = useState<BindingTarget>("main_agent");

  const visibleCount = localEntries.length;
  const currentSourceCount =
    source === "cloud" ? visibleCount : kind === "agent" ? counts.localAgents : kind === "skill" ? counts.localSkills : counts.localCLIs;
  const EntryIcon = kind === "agent" ? Bot : kind === "skill" ? Sparkles : TerminalSquare;
  const selectedStatus = selectedEntry?.rawStatus ?? "";
  const selectedBindings = selectedId ? bindings.filter((binding) => binding.artifact_id === selectedId) : [];
  const selectedSearchArtifact = selectedId ? artifactLookup[selectedId] ?? null : null;
  const progressIndex = installJob?.progress_index ?? 0;
  const progressTotal = installJob?.progress_total || 5;
  const progressPercent = Math.min(100, Math.max(0, Math.round((progressIndex / progressTotal) * 100)));
  const installError = installJob && TERMINAL_ERROR_STATES.includes(installJob.state.toLowerCase()) ? installJob.error : "";
  const combinedError = actionError || installError || errorMessage;
  const alternateKind = kind === "agent" ? "skill" : "agent";
  const alternateKindLabel = kind === "agent" ? "技能" : "代理";
  const installedOrBound = ["installed", "bound", "active"].includes(selectedStatus);
  const upgradeVersions = versions.filter((version) => version.version && version.version !== detail?.version && !version.deprecated);
  const recommendedUpgradeVersion = upgradeVersions[0]?.version ?? "";
  const currentVersion = versions.find((version) => version.version === detail?.version) ?? versions[0];
  const permissionsDiff = currentVersion?.permissions_diff ?? [];
  const highRiskPermissions = detail?.permissions?.filter((permission) =>
    ["process.exec", "process.kill", "desktop.control", "browser.control", "network.any", "secrets.read", "fs.delete"].includes(permission.toLowerCase()),
  );

  const detailPanel = selectedEntry ? (
    <div className="sticky top-6 space-y-4">
      <BackendDetailSection title="能力详情">
        <div className="flex items-start gap-4">
          <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
            <EntryIcon size={22} strokeWidth={2.1} />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-[24px] font-semibold text-ink">{selectedEntry.name}</h3>
              <StatusBadge label={selectedEntry.status} tone={statusTone(selectedEntry.status)} />
            </div>
            <div className="mt-2 text-sm text-[#607699]">{selectedEntry.owner}</div>
            {selectedEntry.source === "cloud" && selectedSearchArtifact ? (
              <>
              <div className="mt-3 flex flex-wrap gap-2 text-xs text-[#607699]">
                <span className="rounded-[8px] border border-[#d8e3f0] bg-[#f7fbff] px-2.5 py-1.5 font-medium text-[#355070]">
                  Final {formatScore(selectedSearchArtifact.final_score ?? selectedSearchArtifact.score)}
                </span>
                <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                  Lexical {formatScore(selectedSearchArtifact.lexical_score)}
                </span>
                <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                  Tag {formatScore(selectedSearchArtifact.tag_score)}
                </span>
                <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                  Trust {formatScore(selectedSearchArtifact.trust_score)}
                </span>
                <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                  Risk -{formatScore(selectedSearchArtifact.risk_penalty)}
                </span>
              </div>
              <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
                <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-xs text-[#607699]">
                  <div className="font-medium text-ink">Lexical</div>
                  <div className="mt-1">{formatScore(selectedSearchArtifact.lexical_score)}</div>
                </div>
                <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-xs text-[#607699]">
                  <div className="font-medium text-ink">Tag Match</div>
                  <div className="mt-1">{formatScore(selectedSearchArtifact.tag_score)}</div>
                </div>
                <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-xs text-[#607699]">
                  <div className="font-medium text-ink">Trust Boost</div>
                  <div className="mt-1">{formatScore(selectedSearchArtifact.trust_score)}</div>
                </div>
                <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-xs text-[#607699]">
                  <div className="font-medium text-ink">Freshness</div>
                  <div className="mt-1">{formatScore(selectedSearchArtifact.freshness_score)}</div>
                </div>
                <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-xs text-[#607699]">
                  <div className="font-medium text-ink">Risk Penalty</div>
                  <div className="mt-1">-{formatScore(selectedSearchArtifact.risk_penalty)}</div>
                </div>
                <div className="rounded-[8px] border border-[#d8e3f0] bg-[#f7fbff] px-3 py-2 text-xs text-[#355070]">
                  <div className="font-medium text-ink">Final Score</div>
                  <div className="mt-1">{formatScore(selectedSearchArtifact.final_score ?? selectedSearchArtifact.score)}</div>
                </div>
              </div>
              </>
            ) : null}
            <p className="mt-3 text-sm leading-7 text-mute">{detail?.description || selectedEntry.summary}</p>
          </div>
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          {[...(selectedEntry.chips ?? []), ...(detail?.hit_signals ?? [])].slice(0, 8).map((chip) => (
            <span key={chip} className="whitespace-nowrap rounded-[8px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]">
              {chip}
            </span>
          ))}
        </div>
      </BackendDetailSection>

      <BackendDetailSection title="安全审查">
        <BackendPropertyList
          items={[
            { label: "权限", value: csv(detail?.permissions) },
            { label: "权限变化", value: csv(permissionsDiff) },
            { label: "高危权限", value: csv(highRiskPermissions) },
            { label: "风险", value: detail?.risk_level || "未知" },
            { label: "可信度", value: detail?.trust_level || "未知" },
            {
              label: "兼容性",
              value: csv(compactStrings([detail?.compatibility?.anyclaw_min, csv(detail?.compatibility?.os, ""), csv(detail?.compatibility?.arch, "")])),
            },
            {
              label: "依赖",
              value:
                detail?.dependencies && detail.dependencies.length > 0
                  ? detail.dependencies.map((item) => `${item.id}${item.version_range ? ` ${item.version_range}` : ""}`).join(", ")
                  : "无",
            },
          ]}
        />
      </BackendDetailSection>

      <BackendDetailSection title="安装与绑定">
        <div className="space-y-4">
          <div className="grid grid-cols-3 gap-3 text-sm">
            <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-3">
              <div className="text-[#64748b]">已安装</div>
              <div className="mt-1 font-semibold text-ink">{["installed", "bound", "active"].includes(selectedStatus) ? "是" : "否"}</div>
            </div>
            <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-3">
              <div className="text-[#64748b]">已绑定</div>
              <div className="mt-1 font-semibold text-ink">{selectedBindings.length > 0 || selectedStatus === "bound" || selectedStatus === "active" ? "是" : "否"}</div>
            </div>
            <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-3">
              <div className="text-[#64748b]">已启用</div>
              <div className="mt-1 font-semibold text-ink">{selectedStatus === "active" ? "是" : "否"}</div>
            </div>
          </div>

          {confirmInstall ? (
            <div className="rounded-[8px] border border-[#f2d4a7] bg-[#fff8ec] p-4">
              <div className="flex items-start gap-3">
                <ShieldCheck className="mt-0.5 shrink-0 text-[#8a6135]" size={18} strokeWidth={2.1} />
                <div className="min-w-0 flex-1 text-sm leading-6 text-[#7a4c12]">
                  安装前请确认权限和可信度信息。安装会写入本地回执，绑定是安装后的独立操作。
                </div>
              </div>
              <div className="mt-3 grid gap-2 text-sm text-[#7a4c12]">
                <div>
                  <span className="font-medium">风险：</span>
                  {detail?.risk_level || "未知"}
                  <span className="ml-3 font-medium">可信度：</span>
                  {detail?.trust_level || "未知"}
                </div>
                <div>
                  <span className="font-medium">权限：</span>
                  {csv(detail?.permissions)}
                </div>
                <div>
                  <span className="font-medium">权限变化：</span>
                  {csv(permissionsDiff)}
                </div>
                {highRiskPermissions && highRiskPermissions.length > 0 ? (
                  <div className="rounded-[8px] border border-[#efc77f] bg-white/60 px-3 py-2">
                    <span className="font-medium">高危权限二次确认：</span>
                    {csv(highRiskPermissions)}
                  </div>
                ) : null}
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                <button
                  className="inline-flex items-center gap-2 rounded-[8px] bg-[#1f2430] px-3.5 py-2 text-sm font-medium text-white disabled:opacity-60"
                  disabled={installPending}
                  onClick={() => {
                    installArtifact();
                    setConfirmInstall(false);
                  }}
                  type="button"
                >
                  <PackageCheck size={16} strokeWidth={2.1} />
                  确认安装
                </button>
                <button
                  className="rounded-[8px] border border-skin bg-white px-3.5 py-2 text-sm font-medium text-[#64748b]"
                  onClick={() => setConfirmInstall(false)}
                  type="button"
                >
                  取消
                </button>
              </div>
            </div>
          ) : (
            <button
              className="inline-flex w-full items-center justify-center gap-2 rounded-[8px] bg-[#1f2430] px-4 py-2.5 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-45"
              disabled={!canInstall || installPending}
              onClick={() => setConfirmInstall(true)}
              type="button"
            >
              <PackageCheck size={16} strokeWidth={2.1} />
              {installPending ? "正在开始安装" : canInstall ? "安装能力" : "暂不可安装"}
            </button>
          )}

          <div className="flex gap-2">
            <select
              className="min-w-0 flex-1 rounded-[8px] border border-skin bg-white px-3 py-2 text-sm text-ink outline-none"
              onChange={(event) => setBindingTarget(event.target.value as BindingTarget)}
              value={bindingTarget}
            >
              {BINDING_TARGETS.map((target) => (
                <option key={target.value} value={target.value}>
                  {target.label}
                </option>
              ))}
            </select>
            <button
              className="inline-flex items-center gap-2 rounded-[8px] border border-skin bg-white px-3.5 py-2 text-sm font-medium text-ink disabled:cursor-not-allowed disabled:opacity-45"
              disabled={!canBind}
              onClick={() => bindArtifact(bindingTarget)}
              type="button"
            >
              <Link2 size={16} strokeWidth={2.1} />
              绑定
            </button>
          </div>

          {selectedBindings.length > 0 ? (
            <div className="space-y-2">
              {selectedBindings.map((binding) => (
                <div key={binding.id} className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-sm">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-medium text-ink">{bindingLabel(binding.target_type)}</span>
                    <StatusBadge label={binding.state} tone={binding.state === "enabled" ? "success" : "default"} />
                  </div>
                  <div className="mt-1 truncate text-xs text-[#64748b]">{binding.target_id || "全局"}</div>
                </div>
              ))}
            </div>
          ) : null}
        </div>
      </BackendDetailSection>

      <BackendDetailSection title="生命周期">
        <div className="space-y-4">
          <div className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-3 text-sm leading-6 text-mute">
            升级成功后会保留已有绑定。如果升级校验失败，旧回执和绑定会继续保留。
          </div>

          <div className="flex flex-wrap gap-2">
            <button
              className="inline-flex items-center gap-2 rounded-[8px] border border-skin bg-white px-3.5 py-2 text-sm font-medium text-ink disabled:cursor-not-allowed disabled:opacity-45"
              disabled={!installedOrBound || !recommendedUpgradeVersion || upgradePending}
              onClick={() => upgradeArtifact(recommendedUpgradeVersion)}
              type="button"
            >
              <UploadCloud size={16} strokeWidth={2.1} />
              {recommendedUpgradeVersion ? `升级到 v${recommendedUpgradeVersion}` : "暂无可升级版本"}
            </button>

            {confirmUninstall ? (
              <>
                <button
                  className="inline-flex items-center gap-2 rounded-[8px] bg-[#7a332c] px-3.5 py-2 text-sm font-medium text-white disabled:opacity-60"
                  disabled={uninstallPending}
                  onClick={() => {
                    uninstallArtifact();
                    setConfirmUninstall(false);
                  }}
                  type="button"
                >
                  <Trash2 size={16} strokeWidth={2.1} />
                  确认卸载
                </button>
                <button
                  className="rounded-[8px] border border-skin bg-white px-3.5 py-2 text-sm font-medium text-[#64748b]"
                  onClick={() => setConfirmUninstall(false)}
                  type="button"
                >
                  取消
                </button>
              </>
            ) : (
              <button
                className="inline-flex items-center gap-2 rounded-[8px] border border-[#efc8bf] bg-white px-3.5 py-2 text-sm font-medium text-[#7a332c] disabled:cursor-not-allowed disabled:opacity-45"
                disabled={!installedOrBound || uninstallPending}
                onClick={() => setConfirmUninstall(true)}
                type="button"
              >
                <Trash2 size={16} strokeWidth={2.1} />
                卸载
              </button>
            )}
          </div>
        </div>
      </BackendDetailSection>

      {installJob ? (
        <BackendDetailSection title="安装进度">
          <div className="space-y-3">
            <div className="flex items-center justify-between gap-4 text-sm">
              <span className="font-medium text-ink">{installJob.progress_step || installJob.state}</span>
              <StatusBadge label={installJob.state} tone={statusTone(installJob.state)} />
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-[#edf2f7]">
              <div className="h-full rounded-full bg-[#4f6f9f]" style={{ width: `${progressPercent}%` }} />
            </div>
            <div className="grid grid-cols-5 gap-1">
              {INSTALL_STEPS.map((step, index) => {
                const done = progressIndex >= index + 1;
                return (
                  <div key={step} className="min-w-0 text-center text-[11px] text-[#64748b]">
                    <span
                      className={[
                        "mx-auto mb-1 flex h-6 w-6 items-center justify-center rounded-full border",
                        done ? "border-[#4f6f9f] bg-[#eef4ff] text-[#4f6f9f]" : "border-skin bg-white",
                      ].join(" ")}
                    >
                      {done ? <CheckCircle2 size={13} strokeWidth={2.2} /> : index + 1}
                    </span>
                    <span className="block truncate">{step}</span>
                  </div>
                );
              })}
            </div>
            {installJob.decision ? (
              <div className="rounded-[8px] border border-[#e6edf5] bg-[#fbfcfe] px-3 py-3 text-sm leading-6 text-mute">
                <div className="font-medium text-ink">策略结果：{installJob.decision.decision}</div>
                <ul className="mt-2 space-y-1">
                  {riskReasons(installJob.decision.reasons).map((reason) => (
                    <li key={reason}>{reason}</li>
                  ))}
                </ul>
                {installJob.decision.high_risk_permissions && installJob.decision.high_risk_permissions.length > 0 ? (
                  <div className="mt-2 text-[#7a4c12]">高危权限：{csv(installJob.decision.high_risk_permissions)}</div>
                ) : null}
                {installJob.checksum_sha256 ? <div className="mt-2 break-all text-xs text-[#64748b]">SHA256：{installJob.checksum_sha256}</div> : null}
              </div>
            ) : null}
          </div>
        </BackendDetailSection>
      ) : null}

      <BackendDetailSection title="版本">
        {versions.length > 0 ? (
          <div className="space-y-2">
            {versions.slice(0, 5).map((version) => (
              <div key={version.version} className="rounded-[8px] border border-skin bg-[#fbfcfe] px-3 py-2 text-sm">
                <div className="flex items-center justify-between gap-3">
                  <span className="font-medium text-ink">v{version.version}</span>
                  <span className="text-[#64748b]">{formatBytes(version.size_bytes)}</span>
                </div>
                {version.permissions_diff && version.permissions_diff.length > 0 ? (
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {version.permissions_diff.map((permission) => (
                      <span key={permission} className="rounded-[8px] border border-[#f2d4a7] bg-[#fff8ec] px-2 py-1 text-[11px] font-medium text-[#7a4c12]">
                        {permission}
                      </span>
                    ))}
                  </div>
                ) : null}
                {version.changelog_md ? <p className="mt-1 line-clamp-2 text-xs leading-5 text-mute">{version.changelog_md}</p> : null}
                {installedOrBound && version.version !== detail?.version ? (
                  <button
                    className="mt-2 inline-flex items-center gap-2 rounded-[8px] border border-skin bg-white px-2.5 py-1.5 text-xs font-medium text-ink disabled:opacity-45"
                    disabled={upgradePending}
                    onClick={() => upgradeArtifact(version.version)}
                    type="button"
                  >
                    <UploadCloud size={13} strokeWidth={2.1} />
                    升级
                  </button>
                ) : null}
              </div>
            ))}
          </div>
        ) : (
          <div className="text-sm text-mute">暂无版本元数据。</div>
        )}
      </BackendDetailSection>
    </div>
  ) : (
    <BackendEmptyState icon={EntryIcon} title="请选择一个能力" />
  );

  return (
    <div className="relative z-10 flex min-h-full flex-1 flex-col px-5 py-5 sm:px-6 lg:px-8 lg:py-7">
      <BackendPageHeader
        icon={Store}
        sectionLabel="市场"
        sourceLabel={data.meta.sourceLabel}
        stats={[
          { label: "目录", value: kindLabel },
          { label: "来源", value: sourceLabel },
          { label: source === "cloud" ? "云端条目" : "本地已安装", value: String(currentSourceCount) },
          { label: "当前显示", value: String(visibleCount) },
        ]}
        title="能力市场"
      />

      <BackendToolbar
        groups={[
          {
            items: [
              { active: kind === "agent", label: "代理", onClick: () => setKind("agent") },
              { active: kind === "skill", label: "技能", onClick: () => setKind("skill") },
              { active: kind === "cli", label: "命令行", onClick: () => setKind("cli") },
            ],
          },
          {
            items: [
              { active: source === "cloud", label: "云端", onClick: () => setSource("cloud") },
              { active: source === "local", label: "本地", onClick: () => setSource("local") },
            ],
          },
        ]}
        onSearchChange={setQuery}
        searchPlaceholder={`搜索${kindLabel}名称或标签`}
        searchValue={query}
      />

      <div className="mt-4 grid gap-3 rounded-[8px] border border-skin bg-white p-4 md:grid-cols-2 xl:grid-cols-4">
        <label className="space-y-1.5 text-xs font-medium text-[#64748b]">
          <span>风险</span>
          <select
            className="w-full rounded-[8px] border border-skin bg-white px-3 py-2 text-sm text-ink outline-none"
            onChange={(event) => setFilter("risk", event.target.value)}
            value={filters.risk}
          >
            <option value="">全部</option>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
            <option value="unknown">unknown</option>
          </select>
        </label>
        <label className="space-y-1.5 text-xs font-medium text-[#64748b]">
          <span>可信度</span>
          <select
            className="w-full rounded-[8px] border border-skin bg-white px-3 py-2 text-sm text-ink outline-none"
            onChange={(event) => setFilter("trust", event.target.value)}
            value={filters.trust}
          >
            <option value="">全部</option>
            <option value="verified">verified</option>
            <option value="community">community</option>
            <option value="unverified">unverified</option>
            <option value="unknown">unknown</option>
          </select>
        </label>
        <label className="space-y-1.5 text-xs font-medium text-[#64748b]">
          <span>排序</span>
          <select
            className="w-full rounded-[8px] border border-skin bg-white px-3 py-2 text-sm text-ink outline-none"
            onChange={(event) => setFilter("sort", event.target.value)}
            value={filters.sort}
          >
            <option value="score">综合评分</option>
            <option value="updated">最近更新</option>
            <option value="name">名称 A-Z</option>
          </select>
        </label>
        {FILTER_INPUTS.map((item) => (
          <label className="space-y-1.5 text-xs font-medium text-[#64748b]" key={item.key}>
            <span>{item.label}</span>
            <input
              className="w-full rounded-[8px] border border-skin bg-white px-3 py-2 text-sm text-ink outline-none"
              onChange={(event) => setFilter(item.key, event.target.value)}
              placeholder={item.placeholder}
              value={filters[item.key]}
            />
          </label>
        ))}
      </div>

      {combinedError ? (
        <div className="mt-5 flex items-start justify-between gap-4 rounded-[8px] border border-[#f2d4a7] bg-[#fff8ec] px-4 py-3 text-sm leading-6 text-[#7a4c12]">
          <div className="flex min-w-0 items-start gap-3">
            <AlertTriangle className="mt-0.5 shrink-0" size={17} strokeWidth={2.1} />
            <span>{combinedError}</span>
          </div>
          <button
            className="inline-flex shrink-0 items-center gap-2 rounded-[8px] border border-[#e7c98f] bg-white/60 px-3 py-1.5 text-sm font-medium"
            onClick={() => void refetch()}
            type="button"
          >
            <RefreshCw size={14} strokeWidth={2.1} />
            重试
          </button>
        </div>
      ) : null}

      {lastUninstall ? (
        <div className="mt-5 rounded-[8px] border border-[#cce5d5] bg-[#f2fbf5] px-4 py-3 text-sm leading-6 text-[#356548]">
          已卸载 {lastUninstall.artifact_id}。相关绑定已移除；如果本地恢复元数据可用，可在 {lastUninstall.undo_available_seconds ?? 30} 秒内撤销。
        </div>
      ) : null}

      <section className="mt-6">
        <BackendSectionHeader
          countLabel={isFetching ? "正在刷新" : `${visibleCount} 个条目`}
          description={`${sourceLabel}${kindLabel}能力。安装和绑定是两个独立步骤。`}
          title={`${sourceLabel}${kindLabel}目录`}
        />

        <div className="mt-4">
          <BackendSummaryStrip
            items={[
              { active: Boolean(selectedEntry), label: "已选择", value: selectedEntry ? selectedEntry.name : "无" },
              { label: "来源", value: `${kindLabel} / ${sourceLabel}` },
              { label: source === "cloud" ? "云端条目" : "本地已安装", value: String(currentSourceCount) },
              { label: `切换到${alternateKindLabel}`, onClick: () => setKind(alternateKind), value: `打开${alternateKindLabel}` },
            ]}
          />
        </div>

        {localEntries.length > 0 ? (
          <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1fr)_390px]">
            <div className="overflow-hidden rounded-[8px] border border-skin bg-white">
              <div className="hidden border-b border-skin bg-[#fafbfd] px-5 py-3 xl:block">
                <div className="grid gap-4 text-xs font-medium uppercase text-[#98a2b3] xl:grid-cols-[minmax(0,2.2fr)_minmax(160px,0.95fr)_minmax(120px,0.7fr)]">
                  <div>能力</div>
                  <div>来源</div>
                  <div className="text-right">状态</div>
                </div>
              </div>

              {localEntries.map((entry) => {
                const active = selectedId === entry.id;
                const searchArtifact = artifactLookup[entry.id] ?? null;

                return (
                  <article key={entry.id} className="border-b border-skin last:border-b-0">
                    <div
                      aria-selected={active}
                      className={["cursor-pointer transition-colors duration-150", active ? "bg-[#f7faff]" : "hover:bg-[#fbfcfe]"].join(" ")}
                      onClick={() => setSelected(entry.id)}
                      onKeyDown={(event) => rowKeyHandler(event, () => setSelected(entry.id))}
                      role="button"
                      tabIndex={0}
                    >
                      <div className="grid gap-4 px-4 py-4 xl:grid-cols-[minmax(0,2.2fr)_minmax(160px,0.95fr)_minmax(120px,0.7fr)] lg:px-5">
                        <div className="min-w-0">
                          <div className="flex items-start gap-4">
                            <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
                              <EntryIcon size={17} strokeWidth={2.1} />
                            </span>
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <h3 className="truncate text-[18px] font-semibold text-ink">{entry.name}</h3>
                                {entry.source === "cloud" ? <Cloud size={15} className="text-[#607699]" strokeWidth={2.1} /> : null}
                              </div>
                              <p className="mt-2 max-w-[58ch] text-sm leading-7 text-mute">{entry.summary}</p>
                              {entry.chips.length > 0 ? (
                                <div className="mt-3 flex flex-wrap gap-2">
                                  {entry.chips.map((chip) => (
                                    <span key={chip} className="whitespace-nowrap rounded-[8px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]">
                                      {chip}
                                    </span>
                                  ))}
                                </div>
                              ) : null}
                              {entry.source === "cloud" && searchArtifact ? (
                                <div className="mt-3 flex flex-wrap gap-2 text-[11px] text-[#607699]">
                                  <span className="rounded-[8px] border border-[#d8e3f0] bg-[#f7fbff] px-2.5 py-1.5 font-medium text-[#355070]">
                                    Score {formatScore(searchArtifact.final_score ?? searchArtifact.score)}
                                  </span>
                                  <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                                    Lex {formatScore(searchArtifact.lexical_score)}
                                  </span>
                                  <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                                    Trust {formatScore(searchArtifact.trust_score)}
                                  </span>
                                  <span className="rounded-[8px] border border-skin bg-white px-2.5 py-1.5">
                                    Risk -{formatScore(searchArtifact.risk_penalty)}
                                  </span>
                                </div>
                              ) : null}
                            </div>
                          </div>
                        </div>

                        <div className="flex flex-col items-start gap-2 pl-14 text-sm text-[#607699] xl:pl-0">
                          <span>{entry.owner}</span>
                          {entry.source === "cloud" && searchArtifact ? (
                            <div className="flex flex-wrap gap-2 text-[11px]">
                              <span className="rounded-[8px] bg-[#f5f7fb] px-2 py-1 text-[#5b6f8b]">{riskLabel(searchArtifact)}</span>
                              <span className="rounded-[8px] bg-[#f5f7fb] px-2 py-1 text-[#5b6f8b]">{trustLabel(searchArtifact)}</span>
                            </div>
                          ) : null}
                        </div>

                        <div className="flex items-start pl-14 xl:justify-end xl:pl-0">
                          <StatusBadge label={entry.status} tone={statusTone(entry.status)} />
                        </div>
                      </div>
                    </div>

                    {active ? <div className="border-t border-skin bg-[#fcfdff] px-4 py-4 xl:hidden lg:px-5">{detailPanel}</div> : null}
                  </article>
                );
              })}
            </div>

            <aside className="hidden xl:block">{detailPanel}</aside>
          </div>
        ) : (
          <div className="mt-5">
            <BackendEmptyState
              description={source === "cloud" ? "云端 Registry 没有返回匹配能力，或当前未配置云端市场。" : "本地没有匹配当前筛选条件的能力。"}
              icon={source === "cloud" ? Cloud : EntryIcon}
              title="没有匹配的能力"
            />
          </div>
        )}
      </section>
    </div>
  );
}
