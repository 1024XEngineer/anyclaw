import { Bot, Cloud, Sparkles, Store } from "lucide-react";
import type { KeyboardEvent } from "react";

import { BackendDetailSection } from "@/features/backend-ui/BackendDetailSection";
import { BackendEmptyState } from "@/features/backend-ui/BackendEmptyState";
import { BackendPageHeader } from "@/features/backend-ui/BackendPageHeader";
import { BackendPropertyList } from "@/features/backend-ui/BackendPropertyList";
import { BackendSectionHeader } from "@/features/backend-ui/BackendSectionHeader";
import { BackendSummaryStrip } from "@/features/backend-ui/BackendSummaryStrip";
import { BackendToolbar } from "@/features/backend-ui/BackendToolbar";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { useMarketDirectory } from "@/features/market/useMarketDirectory";

function rowKeyHandler(event: KeyboardEvent<HTMLElement>, onSelect: () => void) {
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    onSelect();
  }
}

export function MarketPage() {
  const {
    cloudPanels,
    counts,
    data,
    kind,
    kindLabel,
    localEntries,
    query,
    selectedEntry,
    selectedId,
    setKind,
    setQuery,
    setSelected,
    setSource,
    source,
    sourceLabel,
  } = useMarketDirectory();

  const currentLocalCount = kind === "agent" ? counts.localAgents : counts.localSkills;
  const visibleCount = source === "local" ? localEntries.length : cloudPanels.length;
  const EntryIcon = kind === "agent" ? Bot : Sparkles;
  const alternateKind = kind === "agent" ? "skill" : "agent";
  const alternateKindLabel = kind === "agent" ? "Skill" : "Agent";

  const detailPanel = selectedEntry ? (
    <BackendDetailSection title="条目详情">
      <div className="flex items-start gap-4">
        <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-[18px] bg-[#f3f6fb] text-[#607699]">
          <EntryIcon size={22} strokeWidth={2.1} />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-[24px] font-semibold tracking-[-0.04em] text-ink">{selectedEntry.name}</h3>
            <StatusBadge label={selectedEntry.status} tone={getStatusTone(selectedEntry.status)} />
          </div>
          <div className="mt-2 text-sm text-[#607699]">{selectedEntry.owner}</div>
          <p className="mt-3 text-sm leading-7 text-mute">{selectedEntry.summary}</p>
        </div>
      </div>

      {selectedEntry.chips.length > 0 ? (
        <div className="mt-4 flex flex-wrap gap-2">
          {selectedEntry.chips.map((chip) => (
            <span key={chip} className="whitespace-nowrap rounded-[10px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]">
              {chip}
            </span>
          ))}
        </div>
      ) : null}

      <div className="mt-5">
        <BackendPropertyList
          items={[
            { label: "目录类型", value: kindLabel },
            { label: "来源", value: selectedEntry.owner },
            { label: "状态", value: selectedEntry.status },
            { label: "标识", value: selectedEntry.id },
            { label: "附注", value: selectedEntry.footnote },
          ]}
        />
      </div>
    </BackendDetailSection>
  ) : (
    <BackendEmptyState icon={EntryIcon} title="等待选择" description="先从左侧列表选择一个本地条目。" />
  );

  return (
    <div className="min-h-dvh px-5 py-6 sm:px-6 lg:px-8 lg:py-8">
      <div className="mx-auto max-w-[1440px]">
        <BackendPageHeader
          icon={Store}
          sectionLabel="Market"
          sourceLabel={data.meta.sourceLabel}
          stats={[
            { label: "当前目录", value: kindLabel },
            { label: "来源", value: sourceLabel },
            { label: "本地已安装", value: String(currentLocalCount) },
            { label: "当前结果", value: String(visibleCount) },
          ]}
          title="市场"
          description="先把本地 Agent / Skill 目录整理到统一壳层下，云端市场继续保留预留位。"
        />

        <BackendToolbar
          groups={[
            {
              items: [
                { active: kind === "agent", label: "Agent", onClick: () => setKind("agent") },
                { active: kind === "skill", label: "Skill", onClick: () => setKind("skill") },
              ],
            },
            {
              items: [
                { active: source === "local", label: "本地已安装", onClick: () => setSource("local") },
                { active: source === "cloud", label: "云端预留", onClick: () => setSource("cloud") },
              ],
            },
          ]}
          onSearchChange={setQuery}
          searchPlaceholder={`搜索 ${kindLabel} 名称或标签`}
          searchValue={query}
        />

        {source === "local" ? (
          <section className="mt-6">
            <BackendSummaryStrip
              items={[
                { active: Boolean(selectedEntry), label: "当前条目", value: selectedEntry ? selectedEntry.name : "等待选择" },
                { label: "目录来源", value: `${kindLabel} / ${sourceLabel}` },
                { label: "本地已安装", value: String(currentLocalCount) },
                { label: `切到 ${alternateKindLabel}`, onClick: () => setKind(alternateKind), value: `查看 ${alternateKindLabel}` },
              ]}
            />

            <BackendSectionHeader
              countLabel={`${visibleCount} entries`}
              title={`本地 ${kindLabel} 目录`}
              description="这一页先聚焦本地已识别条目，避免和后续云端目录、聊天页逻辑耦合。"
            />

            <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
              <div className="overflow-hidden rounded-[18px] border border-skin bg-white">
                <div className="hidden border-b border-skin bg-[#fafbfd] px-5 py-3 xl:block">
                  <div className="grid gap-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3] xl:grid-cols-[minmax(0,2.2fr)_minmax(160px,0.95fr)_minmax(120px,0.7fr)]">
                    <div>目录条目</div>
                    <div>来源</div>
                    <div className="text-right">状态</div>
                  </div>
                </div>

                <div>
                  {localEntries.map((entry) => {
                    const active = selectedId === entry.id;

                    return (
                      <article key={entry.id} className="border-b border-skin last:border-b-0">
                        <div
                          aria-selected={active}
                          className={[
                            "cursor-pointer transition-colors duration-150",
                            active ? "bg-[#f7faff]" : "hover:bg-[#fbfcfe]",
                          ].join(" ")}
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
                                  <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                                    <h3 className="truncate text-[18px] font-semibold tracking-[-0.02em] text-ink">{entry.name}</h3>
                                  </div>
                                  <p className="mt-2 max-w-[58ch] text-sm leading-7 text-mute">{entry.summary}</p>
                                  {entry.chips.length > 0 ? (
                                    <div className="mt-3 flex flex-wrap gap-2">
                                      {entry.chips.map((chip) => (
                                        <span
                                          key={chip}
                                          className="whitespace-nowrap rounded-[10px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]"
                                        >
                                          {chip}
                                        </span>
                                      ))}
                                    </div>
                                  ) : null}
                                </div>
                              </div>
                            </div>

                            <div className="flex items-start pl-14 text-sm text-[#607699] xl:pl-0">{entry.owner}</div>
                            <div className="flex items-start pl-14 xl:justify-end xl:pl-0">
                              <StatusBadge label={entry.status} tone={getStatusTone(entry.status)} />
                            </div>
                          </div>
                        </div>
                      </article>
                    );
                  })}
                </div>
              </div>

              <div className="xl:sticky xl:top-6">{detailPanel}</div>
            </div>
          </section>
        ) : (
          <section className="mt-6">
            <BackendSummaryStrip
              items={[
                { label: "当前来源", value: sourceLabel },
                { label: "目录类型", value: kindLabel },
                { label: "条目数量", value: String(cloudPanels.length) },
                { label: "切回本地", onClick: () => setSource("local"), value: "查看本地目录" },
              ]}
            />

            <BackendSectionHeader countLabel={`${cloudPanels.length} items`} title="云端预留" description="这里先作为信息预留位，等后续真正接入远端目录再补交互。" />

            <div className="mt-5 grid gap-4 xl:grid-cols-2">
              {cloudPanels.length > 0 ? (
                cloudPanels.map((panel) => (
                  <BackendDetailSection key={panel.title} title={panel.title} description={panel.summary}>
                    <div className="flex flex-wrap gap-2">
                      <StatusBadge label={panel.status} tone="info" />
                      <StatusBadge label={kindLabel} />
                      <StatusBadge label="预留中" />
                    </div>
                  </BackendDetailSection>
                ))
              ) : (
                <BackendEmptyState icon={Cloud} title="还没有云端目录预留" description="后续会把远端市场信息挂到这里。" />
              )}
            </div>
          </section>
        )}
      </div>
    </div>
  );
}
