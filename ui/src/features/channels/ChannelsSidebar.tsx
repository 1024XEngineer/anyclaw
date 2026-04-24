import { Cable, ShieldCheck, Waypoints } from "lucide-react";

import { BackendSidebarItem } from "@/features/backend-ui/BackendSidebarItem";
import { BackendSidebarSection } from "@/features/backend-ui/BackendSidebarSection";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import { useChannelsConsole } from "@/features/channels/useChannelsConsole";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

export function ChannelsSidebar() {
  const { data } = useWorkspaceOverview();
  const { filteredChannels, selectedSlug, setSelected } = useChannelsConsole(data.priorityChannels);
  const enabledCount = data.priorityChannels.filter((channel) => channel.enabled || channel.running).length;

  return (
    <aside className="relative z-10 flex w-full shrink-0 flex-col border-b border-skin bg-white/72 px-4 py-5 backdrop-blur-xl lg:fixed lg:inset-y-0 lg:left-[104px] lg:w-[352px] lg:min-w-[352px] lg:border-b-0 lg:border-r lg:px-5 lg:py-7">
      <div className="border-b border-skin pb-5">
        <div className="inline-flex items-center gap-2 text-sm text-[#607699]">
          <Waypoints size={15} strokeWidth={2.1} />
          Channels
        </div>
        <h2 className="mt-4 text-[28px] font-semibold tracking-[-0.04em] text-ink">渠道</h2>
      </div>

      <div className="mt-5 flex min-h-0 flex-1 flex-col gap-5">
        <BackendSidebarSection count={String(filteredChannels.length)} title="优先渠道">
          <div className="overflow-hidden rounded-[16px] border border-skin bg-white/80">
            {filteredChannels.map((channel) => (
              <BackendSidebarItem
                key={channel.slug}
                active={selectedSlug === channel.slug}
                icon={Waypoints}
                label={channel.name}
                meta={channel.slug}
                onClick={() => setSelected(channel.slug)}
                trailing={<StatusBadge label={channel.status} tone={getStatusTone(channel.status)} />}
              />
            ))}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection count={`${enabledCount} 已启用`} title="接入策略">
          <div className="overflow-hidden rounded-[16px] border border-skin bg-white/80">
            {data.channelSettings.map((item) => (
              <BackendSidebarItem key={item.label} icon={ShieldCheck} label={item.label} meta={item.value} />
            ))}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection
          bodyClassName="min-h-0 flex-1 overflow-y-auto rounded-[16px] border border-skin bg-white/80"
          className="min-h-0 flex flex-1 flex-col border-b-0 pb-0"
          count={String(data.extensionAdapters.length)}
          title="扩展适配器"
        >
          {data.extensionAdapters.map((adapter) => (
            <BackendSidebarItem
              key={adapter}
              icon={Cable}
              label={adapter}
              trailing={<StatusBadge label="已识别" tone="info" />}
            />
          ))}
        </BackendSidebarSection>
      </div>
    </aside>
  );
}
