import { Copy, ExternalLink, LoaderCircle, Radar, RefreshCcw, ScanSearch, Server } from "lucide-react";
import { useMemo, useState } from "react";

import { BackendDetailSection } from "../../features/backend-ui/BackendDetailSection";
import { BackendEmptyState } from "../../features/backend-ui/BackendEmptyState";
import { BackendPageHeader } from "../../features/backend-ui/BackendPageHeader";
import { BackendPropertyList } from "../../features/backend-ui/BackendPropertyList";
import { BackendSectionHeader } from "../../features/backend-ui/BackendSectionHeader";
import { BackendSummaryStrip } from "../../features/backend-ui/BackendSummaryStrip";
import { BackendToolbar } from "../../features/backend-ui/BackendToolbar";
import { StatusBadge } from "../../features/backend-ui/StatusBadge";
import { type DiscoveryInstance, useDiscoveryDirectory } from "../../features/discovery/useDiscoveryDirectory";

type NoticeTone = "error" | "info" | "success";

function formatLastSeen(value: string) {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return "未知";
  }

  return timestamp.toLocaleString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    month: "numeric",
    day: "numeric",
  });
}

function getInstanceStatus(instance: DiscoveryInstance) {
  if (instance.is_self) {
    return { label: "当前节点", tone: "info" as const };
  }

  const lastSeen = new Date(instance.last_seen).getTime();
  if (!Number.isNaN(lastSeen) && Date.now() - lastSeen < 20_000) {
    return { label: "在线", tone: "success" as const };
  }

  return { label: "最近发现", tone: "default" as const };
}

function formatScanStatus(status: string) {
  if (status === "query sent") return "已发送扫描请求";
  if (status === "discovery not enabled") return "当前未启用 discovery 服务";
  return status;
}

export function DiscoveryPage() {
  const [query, setQuery] = useState("");
  const [notice, setNotice] = useState<{ message: string; tone: NoticeTone } | null>(null);
  const { errorMessage, isFetching, isLoading, isScanning, lastUpdatedAt, peerInstances, refetch, scanNetwork, scanStatus, selfInstance } =
    useDiscoveryDirectory();

  const filteredPeers = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (keyword === "") {
      return peerInstances;
    }

    return peerInstances.filter((instance) =>
      [
        instance.name,
        instance.id,
        instance.url,
        instance.address,
        instance.capabilities.join(" "),
        Object.values(instance.metadata).join(" "),
      ]
        .join(" ")
        .toLowerCase()
        .includes(keyword),
    );
  }, [peerInstances, query]);

  async function handleScan() {
    try {
      const status = await scanNetwork();
      setNotice({
        message: formatScanStatus(status),
        tone: status === "discovery not enabled" ? "info" : "success",
      });
    } catch (error) {
      setNotice({
        message: error instanceof Error ? error.message : "扫描失败",
        tone: "error",
      });
    }
  }

  async function handleCopyURL(url: string) {
    try {
      await navigator.clipboard.writeText(url);
      setNotice({ message: "已复制节点地址", tone: "success" });
    } catch (error) {
      setNotice({
        message: error instanceof Error ? error.message : "复制失败",
        tone: "error",
      });
    }
  }

  const latestSourceLabel =
    lastUpdatedAt > 0
      ? `最近刷新 ${new Date(lastUpdatedAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })}`
      : "等待首次发现结果";

  return (
    <div className="relative z-10 flex min-h-full flex-1 flex-col px-5 py-5 sm:px-6 lg:px-8 lg:py-7">
      <BackendPageHeader
        description="查看当前 AnyClaw 节点、手动触发局域网扫描，并浏览最近发现的其他实例。"
        icon={Radar}
        sectionLabel="Discovery"
        sourceLabel={latestSourceLabel}
        stats={[
          { label: "当前节点", value: selfInstance ? "1" : "0" },
          { label: "已发现节点", value: String(peerInstances.length) },
          { label: "本机能力", value: String(selfInstance?.capabilities.length ?? 0) },
          { label: "扫描状态", value: isScanning ? "扫描中" : scanStatus ? formatScanStatus(scanStatus) : "待命" },
        ]}
        title="发现"
      />

      <BackendToolbar
        groups={[
          {
            items: [
              { active: !isScanning, label: "刷新", onClick: () => void refetch() },
              { active: isScanning, label: "扫描网络", onClick: () => void handleScan() },
            ],
          },
        ]}
        onSearchChange={setQuery}
        searchPlaceholder="搜索节点名称、地址、能力或元数据"
        searchValue={query}
      />

      <section className="mt-6">
        <BackendSummaryStrip
          items={[
            {
              active: Boolean(selfInstance),
              label: "当前节点",
              value: selfInstance?.name || "未发现",
            },
            {
              label: "当前地址",
              value: selfInstance?.url || selfInstance?.address || "未提供",
            },
            {
              label: "最近刷新",
              value:
                lastUpdatedAt > 0
                  ? new Date(lastUpdatedAt).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" })
                  : "尚未刷新",
            },
            {
              label: "远端节点",
              value: `${peerInstances.length} 个`,
            },
          ]}
        />
      </section>

      {notice || errorMessage ? (
        <section className="mt-6">
          <div
            className={[
              "rounded-[18px] border px-5 py-4 text-sm leading-7",
              errorMessage || notice?.tone === "error"
                ? "border-[#f6d7d4] bg-[#fff7f6] text-[#9f2d20]"
                : notice?.tone === "success"
                  ? "border-[#d7eadf] bg-[#f5fbf7] text-[#2d6a4f]"
                  : "border-[#dbe4f0] bg-[#f8fbff] text-[#49658d]",
            ].join(" ")}
          >
            {errorMessage || notice?.message}
          </div>
        </section>
      ) : null}

      <section className="mt-6">
        <BackendSectionHeader countLabel={selfInstance ? "ready" : "pending"} title="当前节点" />

        <div className="mt-5">
          {selfInstance ? (
            <BackendDetailSection title="本机实例信息">
              <div className="flex items-start gap-4">
                <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-[18px] bg-[#eef4ff] text-[#49658d]">
                  <Server size={22} strokeWidth={2.1} />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <h3 className="text-[24px] font-semibold tracking-[-0.04em] text-ink">
                      {selfInstance.name || "This instance"}
                    </h3>
                    <StatusBadge label="当前节点" tone="info" />
                  </div>
                  <p className="mt-3 text-sm leading-7 text-mute">
                    当前 Discovery 页面会实时显示本机广播信息，并随着局域网扫描结果自动刷新。
                  </p>
                </div>
              </div>

              <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
                <BackendPropertyList
                  items={[
                    { label: "URL", value: selfInstance.url || "未提供" },
                    { label: "Address", value: selfInstance.address || selfInstance.host || "未提供" },
                    { label: "Version", value: selfInstance.version || "unknown" },
                    { label: "Capabilities", value: String(selfInstance.capabilities.length) },
                  ]}
                />

                <div className="space-y-3">
                  <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">Capabilities</div>
                  <div className="flex flex-wrap gap-2">
                    {selfInstance.capabilities.length > 0 ? (
                      selfInstance.capabilities.map((capability) => (
                        <span
                          key={capability}
                          className="whitespace-nowrap rounded-[10px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]"
                        >
                          {capability}
                        </span>
                      ))
                    ) : (
                      <span className="text-sm text-mute">暂无能力标记</span>
                    )}
                  </div>
                </div>
              </div>
            </BackendDetailSection>
          ) : (
            <BackendEmptyState
              description="当前还没有从 Discovery 服务拿到本机实例信息，可以稍后刷新一次。"
              icon={Server}
              title="未发现本机节点"
            />
          )}
        </div>
      </section>

      <section className="mt-6">
        <BackendSectionHeader
          countLabel={`${filteredPeers.length} nodes`}
          description="这里展示最近在局域网里发现的 AnyClaw 实例。"
          title="局域网节点"
        />

        <div className="mt-5">
          {isLoading ? (
            <div className="flex items-center gap-3 rounded-[18px] border border-skin bg-white px-5 py-4 text-sm text-mute">
              <LoaderCircle className="animate-spin" size={18} strokeWidth={2.1} />
              <span>正在加载 discovery 结果...</span>
            </div>
          ) : filteredPeers.length > 0 ? (
            <div className="grid gap-4 xl:grid-cols-2">
              {filteredPeers.map((instance) => {
                const status = getInstanceStatus(instance);

                return (
                  <article
                    key={instance.id}
                    className="rounded-[20px] border border-skin bg-white p-5 shadow-[0_12px_24px_rgba(15,23,42,0.04)]"
                  >
                    <div className="flex items-start justify-between gap-4">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h3 className="truncate text-[22px] font-semibold tracking-[-0.03em] text-ink">
                            {instance.name || instance.id}
                          </h3>
                          <StatusBadge label={status.label} tone={status.tone} />
                        </div>
                        <p className="mt-2 text-sm text-[#607699]">{instance.url || instance.address || "No URL"}</p>
                      </div>

                      <span className="text-xs text-[#98a2b3]">v{instance.version || "unknown"}</span>
                    </div>

                    <div className="mt-4 grid gap-3 text-sm text-mute sm:grid-cols-2">
                      <div>
                        <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">Address</div>
                        <div className="mt-1 break-all text-ink">{instance.address || instance.host || "未知"}</div>
                      </div>
                      <div>
                        <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">Last seen</div>
                        <div className="mt-1 text-ink">{formatLastSeen(instance.last_seen)}</div>
                      </div>
                    </div>

                    <div className="mt-4">
                      <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">Capabilities</div>
                      <div className="mt-2 flex flex-wrap gap-2">
                        {instance.capabilities.length > 0 ? (
                          instance.capabilities.map((capability) => (
                            <span
                              key={capability}
                              className="whitespace-nowrap rounded-[10px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]"
                            >
                              {capability}
                            </span>
                          ))
                        ) : (
                          <span className="text-sm text-mute">暂无能力标记</span>
                        )}
                      </div>
                    </div>

                    <div className="mt-5 flex flex-wrap gap-3">
                      {instance.url ? (
                        <button
                          className="inline-flex items-center gap-2 rounded-[12px] bg-[#1f2430] px-4 py-2.5 text-sm font-medium text-white transition-colors duration-150 hover:bg-[#111827]"
                          onClick={() => window.open(instance.url, "_blank", "noopener,noreferrer")}
                          type="button"
                        >
                          <ExternalLink size={16} strokeWidth={2.1} />
                          <span>打开节点</span>
                        </button>
                      ) : null}

                      <button
                        className="inline-flex items-center gap-2 rounded-[12px] border border-skin bg-white px-4 py-2.5 text-sm font-medium text-[#49658d] transition-colors duration-150 hover:bg-[#f8fafc]"
                        onClick={() => void handleCopyURL(instance.url || instance.address)}
                        type="button"
                      >
                        <Copy size={16} strokeWidth={2.1} />
                        <span>复制地址</span>
                      </button>
                    </div>
                  </article>
                );
              })}
            </div>
          ) : (
            <BackendEmptyState
              description={
                query.trim() === ""
                  ? "当前还没有发现其他局域网节点。你可以先点击上方“扫描网络”触发一次主动搜索。"
                  : "没有找到符合当前搜索条件的节点。"
              }
              icon={ScanSearch}
              title="暂无发现结果"
            />
          )}
        </div>
      </section>

      <section className="mt-6 border-t border-skin pt-6">
        <div className="flex flex-wrap gap-3">
          <button
            className="inline-flex items-center gap-2 rounded-[12px] border border-skin bg-white px-4 py-2.5 text-sm font-medium text-[#49658d] transition-colors duration-150 hover:bg-[#f8fafc]"
            onClick={() => void refetch()}
            type="button"
          >
            <RefreshCcw size={16} strokeWidth={2.1} />
            <span>{isFetching ? "正在刷新" : "刷新 discovery"}</span>
          </button>

          <button
            className="inline-flex items-center gap-2 rounded-[12px] bg-[#1f2430] px-4 py-2.5 text-sm font-medium text-white transition-colors duration-150 hover:bg-[#111827]"
            onClick={() => void handleScan()}
            type="button"
          >
            <ScanSearch size={16} strokeWidth={2.1} />
            <span>{isScanning ? "扫描中..." : "再次扫描网络"}</span>
          </button>
        </div>
      </section>
    </div>
  );
}
