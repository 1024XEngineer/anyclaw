import { Bot, Cloud, Sparkles, Store, TerminalSquare } from "lucide-react";

import { BackendSidebarItem } from "@/features/backend-ui/BackendSidebarItem";
import { BackendSidebarSection } from "@/features/backend-ui/BackendSidebarSection";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import { useMarketDirectory } from "@/features/market/useMarketDirectory";

export function MarketSidebar() {
  const { counts, kind, localEntries, query, selectedId, setKind, setSelected, setSource, source } = useMarketDirectory();

  const categoryItems = [
    { count: counts.localAgents, icon: Bot, key: "agent" as const, label: "代理" },
    { count: counts.localSkills, icon: Sparkles, key: "skill" as const, label: "技能" },
    { count: counts.localCLIs, icon: TerminalSquare, key: "cli" as const, label: "命令行" },
  ];

  const localCount = kind === "agent" ? counts.localAgents : kind === "skill" ? counts.localSkills : counts.localCLIs;
  const cloudCount = source === "cloud" ? localEntries.length : undefined;
  const kindName = kind === "agent" ? "代理" : kind === "skill" ? "技能" : "命令行";
  const EntryIcon = kind === "agent" ? Bot : kind === "skill" ? Sparkles : TerminalSquare;

  const sourceItems = [
    { key: "cloud" as const, label: "云端目录", trailing: cloudCount === undefined ? "" : String(cloudCount) },
    { key: "local" as const, label: "本地已安装", trailing: String(localCount) },
  ];

  return (
    <aside className="relative z-10 flex w-full shrink-0 flex-col border-b border-skin bg-white/72 px-4 py-5 backdrop-blur-xl lg:fixed lg:inset-y-0 lg:left-[104px] lg:w-[352px] lg:min-w-[352px] lg:border-b-0 lg:border-r lg:px-5 lg:py-7">
      <div className="border-b border-skin pb-5">
        <div className="inline-flex items-center gap-2 text-sm text-[#607699]">
          <Store size={15} strokeWidth={2.1} />
          市场
        </div>
        <h2 className="mt-4 text-[28px] font-semibold text-ink">能力市场</h2>
      </div>

      <div className="mt-5 flex min-h-0 flex-1 flex-col gap-5">
        <BackendSidebarSection title="类型">
          <div className="overflow-hidden rounded-[8px] border border-skin bg-white/80">
            {categoryItems.map(({ count, icon, key, label }) => (
              <BackendSidebarItem key={key} active={kind === key} icon={icon} label={label} onClick={() => setKind(key)} trailing={String(count)} />
            ))}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection title="来源">
          <div className="overflow-hidden rounded-[8px] border border-skin bg-white/80">
            {sourceItems.map((item) => (
              <BackendSidebarItem key={item.key} active={source === item.key} label={item.label} onClick={() => setSource(item.key)} trailing={item.trailing} />
            ))}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection
          bodyClassName="min-h-0 flex-1 overflow-y-auto rounded-[8px] border border-skin bg-white/80"
          className="min-h-0 flex flex-1 flex-col border-b-0 pb-0"
          count={String(localEntries.length)}
          title={source === "local" ? "已安装目录" : "云端目录"}
        >
          {localEntries.length > 0 ? (
            localEntries.map((entry) => (
              <BackendSidebarItem
                key={entry.id}
                active={selectedId === entry.id}
                icon={entry.source === "cloud" ? Cloud : EntryIcon}
                label={entry.name}
                meta={entry.owner}
                onClick={() => setSelected(entry.id)}
                trailing={<StatusBadge label={entry.status} tone={getStatusTone(entry.status)} />}
              />
            ))
          ) : (
            <div className="px-3 py-4 text-sm leading-7 text-mute">
              {query ? "没有匹配的能力" : source === "cloud" ? `暂无云端${kindName}条目` : "暂无本地条目"}
            </div>
          )}
        </BackendSidebarSection>
      </div>
    </aside>
  );
}
