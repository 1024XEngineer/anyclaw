import { Bot, Cloud, Sparkles, Store } from "lucide-react";
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

  const desktopDetailPanel = selectedEntry ? (
    <div className="sticky top-6 space-y-4">
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
      </BackendDetailSection>

      <BackendDetailSection title="目录定位">
        <BackendPropertyList
          items={[
            { label: "目录类型", value: kindLabel },
            { label: "来源", value: selectedEntry.owner },
            { label: "状态", value: selectedEntry.status },
            { label: "标识", value: selectedEntry.id },
            { label: "标签数", value: String(selectedEntry.chips.length) },
          ]}
        />

        <div className="mt-4">
          <BackendSummaryStrip
            items={[
              {
                label: "当前来源",
                value: sourceLabel,
              },
              {
                label: "云端目录",
                onClick: () => setSource("cloud"),
                value: `查看云端 ${kindLabel}`,
              },
              {
                label: "切换分类",
                onClick: () => setKind(alternateKind),
                value: `切到 ${alternateKindLabel}`,
              },
            ]}
          />
        </div>
      </BackendDetailSection>
    </div>
  ) : (
    <BackendEmptyState icon={EntryIcon} title="等待选择" />
  );

  return (
    <div className="relative z-10 flex min-h-full flex-1 flex-col px-5 py-5 sm:px-6 lg:px-8 lg:py-7">
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
        localEntries.length > 0 ? (
          <section className="mt-6">
            <BackendSectionHeader countLabel={`${visibleCount} entries`} title={`本地 ${kindLabel} 目录`} />

            <div className="mt-4">
              <BackendSummaryStrip
                items={[
                  {
                    active: Boolean(selectedEntry),
                    label: "当前条目",
                    value: selectedEntry ? selectedEntry.name : "等待选择",
                  },
                  {
                    label: "目录来源",
                    value: `${kindLabel} / ${sourceLabel}`,
                  },
                  {
                    label: "本地已安装",
                    value: String(currentLocalCount),
                  },
                  {
                    label: `切到 ${alternateKindLabel}`,
                    onClick: () => setKind(alternateKind),
                    value: `查看 ${alternateKindLabel}`,
                  },
                ]}
              />
            </div>

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
                                    <h3 className="truncate text-[18px] font-semibold tracking-[-0.02em] text-ink">
                                      {entry.name}
                                    </h3>
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

                            <div className="flex items-start pl-14 text-sm text-[#607699] xl:pl-0">
                              {entry.owner}
                            </div>

                            <div className="flex items-start pl-14 xl:justify-end xl:pl-0">
                              <StatusBadge label={entry.status} tone={getStatusTone(entry.status)} />
                            </div>
                          </div>
                        </div>

                        {active ? (
                          <div className="border-t border-skin bg-[#fcfdff] px-4 py-4 xl:hidden lg:px-5">
                            <div className="grid gap-5">
                              <div>
                                <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">
                                  条目说明
                                </div>
                                <p className="mt-2 max-w-[920px] text-[15px] leading-8 text-mute">
                                  {entry.summary}
                                </p>
                              </div>

                              {entry.chips.length > 0 ? (
                                <div>
                                  <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">
                                    标签
                                  </div>
                                  <div className="mt-2 flex flex-wrap gap-2">
                                    {entry.chips.map((chip) => (
                                      <span
                                        key={chip}
                                        className="whitespace-nowrap rounded-[10px] bg-[#f5f7fb] px-2.5 py-1.5 text-xs text-[#5b6f8b]"
                                      >
                                        {chip}
                                      </span>
                                    ))}
                                  </div>
                                </div>
                              ) : null}
                            </div>
                          </div>
                        ) : null}
                      </article>
                    );
                  })}
                </div>
              </div>

              <aside className="hidden xl:block">{desktopDetailPanel}</aside>
            </div>
          </section>
        ) : (
          <section className="mt-8 border-t border-skin pt-6">
            <BackendEmptyState icon={EntryIcon} title="没有匹配结果" />
          </section>
        )
      ) : (
        <section className="mt-6">
          <BackendSectionHeader
            countLabel={`${visibleCount} sections`}
            description={`云端 ${kindLabel} 尚未接入，接入后会直接显示在这里。`}
            title={`云端 ${kindLabel} 目录`}
          />

          <div className="mt-4">
            <BackendSummaryStrip
              items={[
                { label: "当前视图", value: `云端 ${kindLabel}` },
                { label: "预留区块", value: `${visibleCount} 个` },
                { label: "本地已安装", value: String(currentLocalCount) },
                {
                  label: "返回本地",
                  onClick: () => setSource("local"),
                  value: `查看本地 ${kindLabel}`,
                },
              ]}
            />
          </div>

          {cloudPanels.length > 0 ? (
            <div className="mt-5 overflow-hidden rounded-[18px] border border-dashed border-[#dbe4f0] bg-white">
              <div className="hidden border-b border-skin bg-[#fafbfd] px-5 py-3 xl:block">
                <div className="grid gap-4 text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3] xl:grid-cols-[minmax(0,1.7fr)_140px]">
                  <div>目录条目</div>
                  <div className="text-right">状态</div>
                </div>
              </div>

              {cloudPanels.map((panel) => (
                <article key={panel.title} className="border-b border-skin px-4 py-4 last:border-b-0 lg:px-5">
                  <div className="grid gap-4 xl:grid-cols-[minmax(0,1.7fr)_140px]">
                    <div className="min-w-0">
                      <div className="flex items-start gap-4">
                        <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
                          <Cloud size={17} strokeWidth={2.1} />
                        </span>
                        <div className="min-w-0 flex-1">
                          <h3 className="text-[18px] font-semibold tracking-[-0.02em] text-ink">{panel.title}</h3>
                          <p className="mt-2 max-w-[58ch] text-sm leading-7 text-mute">{panel.summary}</p>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-start pl-14 xl:justify-end xl:pl-0">
                      <StatusBadge label={panel.status} tone={getStatusTone(panel.status)} />
                    </div>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <div className="mt-5">
              <BackendEmptyState
                description={`云端 ${kindLabel} 尚未接入。`}
                icon={Cloud}
                title="云端目录为空"
              />
            </div>
          )}
        </section>
      )}
    </div>
  );
}
