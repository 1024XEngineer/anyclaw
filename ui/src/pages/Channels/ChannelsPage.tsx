import { Cable, MessageSquare, ShieldCheck, Waypoints } from "lucide-react";
import { useMemo } from "react";
import type { KeyboardEvent } from "react";

import { BackendDetailSection } from "@/features/backend-ui/BackendDetailSection";
import { BackendEmptyState } from "@/features/backend-ui/BackendEmptyState";
import { BackendPageHeader } from "@/features/backend-ui/BackendPageHeader";
import { BackendPropertyList } from "@/features/backend-ui/BackendPropertyList";
import { BackendSectionHeader } from "@/features/backend-ui/BackendSectionHeader";
import { BackendSummaryStrip } from "@/features/backend-ui/BackendSummaryStrip";
import { BackendToolbar } from "@/features/backend-ui/BackendToolbar";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import { ChannelsControlConsole } from "@/features/channels/ChannelsControlConsole";
import { useChannelsConsole } from "@/features/channels/useChannelsConsole";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";
import type { ChannelRecord } from "@/features/workspace/useWorkspaceOverview";

function matchesQuery(query: string, fields: string[]) {
  const normalized = query.trim().toLowerCase();
  if (normalized === "") return true;

  return fields.join(" ").toLowerCase().includes(normalized);
}

function rowKeyHandler(event: KeyboardEvent<HTMLElement>, onSelect: () => void) {
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    onSelect();
  }
}

function scrollToSection(id: string) {
  document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" });
}

function getChannelActionLabel(channel: ChannelRecord) {
  if (channel.running && channel.healthy) return "查看运行";
  if (channel.enabled) return "继续联调";
  if (channel.configured) return "完成启用";
  return "规划接入";
}

function getChannelNextStep(channel: ChannelRecord) {
  if (channel.running && channel.healthy) return "继续观察运行数据";
  if (channel.enabled) return "补齐联调与健康检查";
  if (channel.configured) return "进入启用与验证阶段";
  return "先完成接入规划与配置";
}

export function ChannelsPage() {
  const { data } = useWorkspaceOverview();
  const { filter, filteredChannels, query, selectedChannel, selectedSlug, setFilter, setQuery, setSelected } =
    useChannelsConsole(data.priorityChannels);

  const enabledCount = data.priorityChannels.filter((channel) => channel.enabled || channel.running).length;
  const configuredCount = data.priorityChannels.filter((channel) => channel.configured).length;
  const plannedCount = data.priorityChannels.filter((channel) => !channel.enabled && !channel.running).length;

  const filteredSettings = useMemo(
    () => data.channelSettings.filter((item) => matchesQuery(query, [item.label, item.value, item.hint])),
    [data.channelSettings, query],
  );

  const filteredAdapters = useMemo(
    () => data.extensionAdapters.filter((adapter) => matchesQuery(query, [adapter])),
    [data.extensionAdapters, query],
  );

  const detailPanel = selectedChannel ? (
    <div className="space-y-4">
      <BackendDetailSection title="渠道详情">
        <div className="flex items-start gap-4">
          <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-[18px] bg-[#f3f6fb] text-[#607699]">
            <MessageSquare size={22} strokeWidth={2.1} />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-[24px] font-semibold tracking-[-0.04em] text-ink">{selectedChannel.name}</h3>
              <StatusBadge label={selectedChannel.status} tone={getStatusTone(selectedChannel.status)} />
            </div>
            <div className="mt-2 text-sm text-[#607699]">{selectedChannel.slug}</div>
            <p className="mt-3 text-sm leading-7 text-mute">{selectedChannel.summary}</p>
          </div>
        </div>
      </BackendDetailSection>

      <BackendDetailSection title="运行状态">
        <BackendPropertyList
          items={[
            { label: "渠道标识", value: selectedChannel.slug },
            { label: "已配置", value: selectedChannel.configured ? "是" : "否" },
            { label: "已启用", value: selectedChannel.enabled ? "是" : "否" },
            { label: "运行中", value: selectedChannel.running ? "是" : "否" },
            {
              label: "健康状态",
              value: selectedChannel.healthy ? "健康" : selectedChannel.enabled ? "待联调" : "未检查",
            },
          ]}
        />
      </BackendDetailSection>

      <BackendDetailSection title="备注与下一步">
        <p className="text-sm leading-7 text-mute">{selectedChannel.note}</p>

        <div className="mt-4">
          <BackendSummaryStrip
            items={[
              {
                active: true,
                label: "建议动作",
                value: getChannelActionLabel(selectedChannel),
              },
              {
                label: "下一步",
                value: getChannelNextStep(selectedChannel),
              },
              {
                label: "接入策略",
                onClick: () => scrollToSection("channel-strategy-section"),
                value: "跳转查看",
              },
              {
                label: "渠道控制台",
                onClick: () => scrollToSection("channel-control-section"),
                value: "真实接口",
              },
              {
                label: "扩展适配器",
                onClick: () => scrollToSection("channel-adapters-section"),
                value: "跳转查看",
              },
            ]}
          />
        </div>
      </BackendDetailSection>
    </div>
  ) : (
    <BackendEmptyState icon={Waypoints} title="等待选择" />
  );

  return (
    <div className="relative z-10 flex min-h-full flex-1 flex-col px-5 py-5 sm:px-6 lg:px-8 lg:py-7">
      <BackendPageHeader
        icon={Waypoints}
        sectionLabel="Channels"
        sourceLabel={data.meta.sourceLabel}
        stats={[
          { label: "优先渠道", value: String(data.priorityChannels.length) },
          { label: "当前启用", value: String(enabledCount) },
          { label: "扩展适配器", value: String(data.extensionAdapters.length) },
          { label: "数据来源", value: data.meta.liveConnected ? "网关叠加" : "仓库快照" },
        ]}
        title="渠道"
      />

      <BackendToolbar
        groups={[
          {
            items: [
              { active: filter === "all", label: "全部", onClick: () => setFilter("all") },
              { active: filter === "enabled", label: "已启用", onClick: () => setFilter("enabled") },
              { active: filter === "planned", label: "待接入", onClick: () => setFilter("planned") },
            ],
          },
        ]}
        onSearchChange={setQuery}
        searchPlaceholder="搜索渠道、状态、配置项或适配器"
        searchValue={query}
      />

      <section className="mt-6" id="channel-priority-section">
        <BackendSectionHeader countLabel={`${filteredChannels.length} rows`} title="优先渠道表" />

        <div className="mt-4">
          <BackendSummaryStrip
            items={[
              {
                active: filter === "enabled",
                label: "已启用",
                value: `${enabledCount} 个`,
              },
              {
                label: "已配置",
                value: `${configuredCount} 个`,
              },
              {
                active: filter === "planned",
                label: "待接入",
                value: `${plannedCount} 个`,
              },
              {
                label: "接入策略",
                onClick: () => scrollToSection("channel-strategy-section"),
                value: "跳到策略表",
              },
              {
                label: "渠道控制台",
                onClick: () => scrollToSection("channel-control-section"),
                value: "跳到控制台",
              },
              {
                label: "适配器",
                onClick: () => scrollToSection("channel-adapters-section"),
                value: "跳到适配器表",
              },
            ]}
          />
        </div>

        {filteredChannels.length > 0 ? (
          <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
            <div className="overflow-hidden rounded-[18px] border border-skin bg-white">
              <div className="hidden border-b border-skin bg-[#fafbfd] lg:block">
                <table className="w-full table-fixed border-collapse">
                  <thead>
                    <tr className="text-left">
                      <th className="w-[220px] px-5 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                        渠道
                      </th>
                      <th className="px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                        接入定位
                      </th>
                      <th className="w-[140px] px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                        状态
                      </th>
                      <th className="w-[220px] px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                        备注
                      </th>
                      <th className="w-[110px] px-4 py-4 text-right text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                        操作
                      </th>
                    </tr>
                  </thead>
                </table>
              </div>

              <div className="hidden lg:block">
                <table className="w-full table-fixed border-collapse">
                  <tbody>
                    {filteredChannels.map((channel) => {
                      const active = selectedSlug === channel.slug;

                      return (
                        <tr
                          key={channel.slug}
                          aria-selected={active}
                          className={[
                            "cursor-pointer border-b border-skin align-top transition-colors duration-150 last:border-b-0",
                            active ? "bg-[#f7faff]" : "hover:bg-[#fbfcfe]",
                          ].join(" ")}
                          onClick={() => setSelected(channel.slug)}
                          onKeyDown={(event) => rowKeyHandler(event, () => setSelected(channel.slug))}
                          role="button"
                          tabIndex={0}
                        >
                          <td className="px-5 py-4">
                            <div className="flex items-start gap-4">
                              <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
                                <MessageSquare size={17} strokeWidth={2.1} />
                              </span>
                              <div>
                                <div className="text-[18px] font-semibold tracking-[-0.02em] text-ink">
                                  {channel.name}
                                </div>
                                <div className="mt-1 text-sm text-[#607699]">{channel.slug}</div>
                              </div>
                            </div>
                          </td>
                          <td className="px-4 py-4 text-sm leading-7 text-mute">{channel.summary}</td>
                          <td className="px-4 py-4">
                            <StatusBadge label={channel.status} tone={getStatusTone(channel.status)} />
                          </td>
                          <td className="px-4 py-4 text-sm leading-7 text-mute">{channel.note}</td>
                          <td className="px-4 py-4 text-right">
                            <button
                              className={[
                                "rounded-[10px] px-3 py-2 text-sm font-medium transition-colors duration-150",
                                active
                                  ? "bg-[#1f2430] text-white"
                                  : "bg-[#f5f7fb] text-[#5b6f8b] hover:bg-[#edf2fa] hover:text-ink",
                              ].join(" ")}
                              onClick={(event) => {
                                event.stopPropagation();
                                setSelected(channel.slug);
                              }}
                              type="button"
                            >
                              {getChannelActionLabel(channel)}
                            </button>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>

              <div className="lg:hidden">
                {filteredChannels.map((channel) => {
                  const active = selectedSlug === channel.slug;

                  return (
                    <article
                      key={channel.slug}
                      className={["border-b border-skin px-4 py-4 last:border-b-0", active ? "bg-[#f7faff]" : ""].join(
                        " ",
                      )}
                    >
                      <div className="flex items-start gap-4">
                        <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
                          <MessageSquare size={17} strokeWidth={2.1} />
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <div className="text-[18px] font-semibold tracking-[-0.02em] text-ink">
                                {channel.name}
                              </div>
                              <div className="mt-1 text-sm text-[#607699]">{channel.slug}</div>
                            </div>
                            <button
                              className={[
                                "rounded-[10px] px-3 py-2 text-sm font-medium transition-colors duration-150",
                                active
                                  ? "bg-[#1f2430] text-white"
                                  : "bg-[#f5f7fb] text-[#5b6f8b] hover:bg-[#edf2fa] hover:text-ink",
                              ].join(" ")}
                              onClick={() => setSelected(channel.slug)}
                              type="button"
                            >
                              {getChannelActionLabel(channel)}
                            </button>
                          </div>
                          <p className="mt-3 text-sm leading-7 text-mute">{channel.summary}</p>
                          <div className="mt-3">
                            <StatusBadge label={channel.status} tone={getStatusTone(channel.status)} />
                          </div>
                          <p className="mt-3 text-sm leading-7 text-mute">{channel.note}</p>
                        </div>
                      </div>
                    </article>
                  );
                })}
              </div>
            </div>

            <aside className="hidden xl:block">
              <div className="sticky top-6">{detailPanel}</div>
            </aside>
          </div>
        ) : (
          <div className="mt-5">
            <BackendEmptyState icon={Waypoints} title="没有匹配渠道" />
          </div>
        )}

        {filteredChannels.length > 0 ? <div className="mt-5 xl:hidden">{detailPanel}</div> : null}
      </section>

      <section className="mt-6" id="channel-control-section">
        <BackendSectionHeader
          countLabel={selectedChannel ? selectedChannel.slug : "select a channel"}
          title="渠道控制台"
        />

        <div className="mt-5">
          <ChannelsControlConsole selectedChannel={selectedChannel} />
        </div>
      </section>

      <section className="mt-6" id="channel-strategy-section">
        <BackendSectionHeader countLabel={`${filteredSettings.length} rows`} title="接入策略表" />

        {filteredSettings.length > 0 ? (
          <div className="mt-5 overflow-hidden rounded-[18px] border border-skin bg-white">
            <div className="hidden border-b border-skin bg-[#fafbfd] lg:block">
              <table className="w-full table-fixed border-collapse">
                <thead>
                  <tr className="text-left">
                    <th className="w-[220px] px-5 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                      项目
                    </th>
                    <th className="w-[240px] px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                      当前值
                    </th>
                    <th className="px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                      说明
                    </th>
                  </tr>
                </thead>
              </table>
            </div>

            <div className="hidden lg:block">
              <table className="w-full table-fixed border-collapse">
                <tbody>
                  {filteredSettings.map((item) => (
                    <tr
                      key={item.label}
                      className="border-b border-skin align-top transition-colors duration-150 last:border-b-0 hover:bg-[#fbfcfe]"
                    >
                      <td className="px-5 py-4">
                        <div className="flex items-center gap-3 text-[16px] font-semibold text-ink">
                          <ShieldCheck size={16} strokeWidth={2.1} className="text-[#607699]" />
                          {item.label}
                        </div>
                      </td>
                      <td className="px-4 py-4 text-sm text-[#607699]">{item.value}</td>
                      <td className="px-4 py-4 text-sm leading-7 text-mute">{item.hint}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="lg:hidden">
              {filteredSettings.map((item) => (
                <article key={item.label} className="border-b border-skin px-4 py-4 last:border-b-0">
                  <div className="flex items-center gap-3 text-[16px] font-semibold text-ink">
                    <ShieldCheck size={16} strokeWidth={2.1} className="text-[#607699]" />
                    {item.label}
                  </div>
                  <div className="mt-2 text-sm text-[#607699]">{item.value}</div>
                  <p className="mt-3 text-sm leading-7 text-mute">{item.hint}</p>
                </article>
              ))}
            </div>
          </div>
        ) : (
          <div className="mt-5">
            <BackendEmptyState icon={ShieldCheck} title="没有匹配策略" />
          </div>
        )}
      </section>

      <section className="mt-6" id="channel-adapters-section">
        <BackendSectionHeader countLabel={`${filteredAdapters.length} rows`} title="扩展适配器表" />

        {filteredAdapters.length > 0 ? (
          <div className="mt-5 overflow-hidden rounded-[18px] border border-skin bg-white">
            <div className="hidden border-b border-skin bg-[#fafbfd] lg:block">
              <table className="w-full table-fixed border-collapse">
                <thead>
                  <tr className="text-left">
                    <th className="w-[120px] px-5 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                      序号
                    </th>
                    <th className="px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                      适配器
                    </th>
                    <th className="w-[180px] px-4 py-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
                      状态
                    </th>
                  </tr>
                </thead>
              </table>
            </div>

            <div className="hidden lg:block">
              <table className="w-full table-fixed border-collapse">
                <tbody>
                  {filteredAdapters.map((adapter, index) => (
                    <tr
                      key={adapter}
                      className="border-b border-skin align-top transition-colors duration-150 last:border-b-0 hover:bg-[#fbfcfe]"
                    >
                      <td className="px-5 py-4 text-sm text-[#64748b]">{String(index + 1).padStart(2, "0")}</td>
                      <td className="px-4 py-4">
                        <div className="flex items-center gap-3">
                          <span className="flex h-9 w-9 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
                            <Cable size={16} strokeWidth={2.1} />
                          </span>
                          <span className="text-[16px] font-medium text-ink">{adapter}</span>
                        </div>
                      </td>
                      <td className="px-4 py-4">
                        <StatusBadge label="已识别" tone="info" />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <div className="lg:hidden">
              {filteredAdapters.map((adapter, index) => (
                <article key={adapter} className="border-b border-skin px-4 py-4 last:border-b-0">
                  <div className="flex items-center gap-3">
                    <span className="flex h-9 w-9 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
                      <Cable size={16} strokeWidth={2.1} />
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="text-[16px] font-medium text-ink">{adapter}</div>
                      <div className="mt-2 flex items-center gap-3 text-sm text-[#64748b]">
                        <span>#{String(index + 1).padStart(2, "0")}</span>
                        <StatusBadge label="已识别" tone="info" />
                      </div>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          </div>
        ) : (
          <div className="mt-5">
            <BackendEmptyState icon={Cable} title="没有匹配适配器" />
          </div>
        )}
      </section>
    </div>
  );
}
