import { Bot, Cloud, Sparkles, Store } from "lucide-react";

import { BackendSidebarItem } from "@/features/backend-ui/BackendSidebarItem";
import { BackendSidebarSection } from "@/features/backend-ui/BackendSidebarSection";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import { useMarketDirectory } from "@/features/market/useMarketDirectory";

export function MarketSidebar() {
  const {
    cloudPanels,
    counts,
    kind,
    localEntries,
    query,
    selectedId,
    setKind,
    setSelected,
    setSource,
    source,
  } = useMarketDirectory();

  const categoryItems = [
    {
      count: counts.localAgents,
      icon: Bot,
      key: "agent" as const,
      label: "Agent",
    },
    {
      count: counts.localSkills,
      icon: Sparkles,
      key: "skill" as const,
      label: "Skill",
    },
  ];

  const sourceItems = [
    {
      key: "local" as const,
      label: "本地已安装",
      trailing: kind === "agent" ? String(counts.localAgents) : String(counts.localSkills),
    },
    {
      description: `云端 ${kind === "agent" ? "Agent" : "Skill"} 尚未接入`,
      key: "cloud" as const,
      label: "云端预留",
    },
  ];

  return (
    <aside className="relative z-10 flex w-full shrink-0 flex-col border-b border-skin bg-white/72 px-4 py-5 backdrop-blur-xl lg:fixed lg:inset-y-0 lg:left-[104px] lg:w-[352px] lg:min-w-[352px] lg:border-b-0 lg:border-r lg:px-5 lg:py-7">
      <div className="border-b border-skin pb-5">
        <div className="inline-flex items-center gap-2 text-sm text-[#607699]">
          <Store size={15} strokeWidth={2.1} />
          Market
        </div>
        <h2 className="mt-4 text-[28px] font-semibold tracking-[-0.04em] text-ink">市场</h2>
      </div>

      <div className="mt-5 flex min-h-0 flex-1 flex-col gap-5">
        <BackendSidebarSection title="分类">
          <div className="overflow-hidden rounded-[16px] border border-skin bg-white/80">
            {categoryItems.map(({ count, icon, key, label }) => (
              <BackendSidebarItem
                key={key}
                active={kind === key}
                icon={icon}
                label={label}
                onClick={() => setKind(key)}
                trailing={String(count)}
              />
            ))}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection title="来源">
          <div className="overflow-hidden rounded-[16px] border border-skin bg-white/80">
            {sourceItems.map((item) => (
              <BackendSidebarItem
                key={item.key}
                active={source === item.key}
                description={item.description}
                label={item.label}
                onClick={() => setSource(item.key)}
                trailing={item.trailing}
              />
            ))}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection
          bodyClassName="min-h-0 flex-1 overflow-y-auto rounded-[16px] border border-skin bg-white/80"
          className="min-h-0 flex flex-1 flex-col border-b-0 pb-0"
          count={String(source === "local" ? localEntries.length : cloudPanels.length)}
          title={source === "local" ? "当前目录" : "云端预留"}
        >
          {source === "local" ? (
            localEntries.length > 0 ? (
              localEntries.map((entry) => (
                <BackendSidebarItem
                  key={entry.id}
                  active={selectedId === entry.id}
                  icon={kind === "agent" ? Bot : Sparkles}
                  label={entry.name}
                  meta={entry.owner}
                  onClick={() => setSelected(entry.id)}
                  trailing={<StatusBadge label={entry.status} tone={getStatusTone(entry.status)} />}
                />
              ))
            ) : (
              <div className="px-3 py-4 text-sm leading-7 text-mute">{query ? "没有匹配结果" : "暂无条目"}</div>
            )
          ) : (
            cloudPanels.map((panel) => (
              <BackendSidebarItem
                key={panel.title}
                description={panel.summary}
                icon={Cloud}
                label={panel.title}
                trailing={<StatusBadge label={panel.status} tone={getStatusTone(panel.status)} />}
              />
            ))
          )}
        </BackendSidebarSection>
      </div>
    </aside>
  );
}
