import { LoaderCircle, Radar, RefreshCcw, ScanSearch } from "lucide-react";

import { BackendSidebarItem } from "../backend-ui/BackendSidebarItem";
import { BackendSidebarSection } from "../backend-ui/BackendSidebarSection";
import { useDiscoveryDirectory } from "./useDiscoveryDirectory";

function formatLastSeen(value: string) {
  const timestamp = new Date(value);
  if (Number.isNaN(timestamp.getTime())) {
    return "unknown";
  }

  return timestamp.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function DiscoverySidebar() {
  const { isFetching, isScanning, peerInstances, refetch, scanNetwork, selfInstance } = useDiscoveryDirectory();

  return (
    <aside className="relative z-10 flex w-full shrink-0 flex-col border-b border-skin bg-white/72 px-4 py-5 backdrop-blur-xl lg:fixed lg:inset-y-0 lg:left-[104px] lg:w-[352px] lg:min-w-[352px] lg:border-b-0 lg:border-r lg:px-5 lg:py-7">
      <div className="border-b border-skin pb-5">
        <div className="inline-flex items-center gap-2 text-sm text-[#607699]">
          <Radar size={15} strokeWidth={2.1} />
          Discovery
        </div>
        <h2 className="mt-4 text-[28px] font-semibold tracking-[-0.04em] text-ink">发现</h2>
      </div>

      <div className="mt-5 flex min-h-0 flex-1 flex-col gap-5">
        <BackendSidebarSection title="Actions">
          <div className="overflow-hidden rounded-[16px] border border-skin bg-white/80">
            <BackendSidebarItem
              icon={ScanSearch}
              label={isScanning ? "正在扫描网络" : "扫描网络"}
              meta="向局域网广播新的发现请求"
              onClick={() => {
                void scanNetwork();
              }}
              trailing={isScanning ? <LoaderCircle className="animate-spin" size={14} strokeWidth={2.1} /> : null}
            />
            <BackendSidebarItem
              icon={RefreshCcw}
              label={isFetching ? "正在刷新" : "刷新列表"}
              meta="立即重新拉取当前发现结果"
              onClick={() => {
                void refetch();
              }}
              trailing={isFetching ? <LoaderCircle className="animate-spin" size={14} strokeWidth={2.1} /> : null}
            />
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection title="Current node">
          <div className="overflow-hidden rounded-[16px] border border-skin bg-white/80">
            {selfInstance ? (
              <BackendSidebarItem
                active
                icon={Radar}
                label={selfInstance.name || "This instance"}
                meta={selfInstance.url || selfInstance.address || "127.0.0.1"}
                trailing={`v${selfInstance.version || "unknown"}`}
              />
            ) : (
              <div className="px-3 py-4 text-sm leading-7 text-mute">当前还没有发现本机实例信息。</div>
            )}
          </div>
        </BackendSidebarSection>

        <BackendSidebarSection
          bodyClassName="min-h-0 flex-1 overflow-y-auto rounded-[16px] border border-skin bg-white/80"
          className="min-h-0 flex flex-1 flex-col border-b-0 pb-0"
          count={String(peerInstances.length)}
          title="Peers"
        >
          {peerInstances.length > 0 ? (
            peerInstances.map((instance) => (
              <BackendSidebarItem
                key={instance.id}
                icon={Radar}
                label={instance.name || instance.id}
                meta={instance.url || instance.address || "No URL"}
                trailing={formatLastSeen(instance.last_seen)}
              />
            ))
          ) : (
            <div className="px-3 py-4 text-sm leading-7 text-mute">当前还没有发现其他局域网节点。</div>
          )}
        </BackendSidebarSection>
      </div>
    </aside>
  );
}
