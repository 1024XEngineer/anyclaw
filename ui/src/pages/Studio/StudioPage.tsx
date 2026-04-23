import { Hammer } from "lucide-react";

import { BackendDetailSection } from "@/features/backend-ui/BackendDetailSection";
import { BackendPageHeader } from "@/features/backend-ui/BackendPageHeader";
import { BackendSummaryStrip } from "@/features/backend-ui/BackendSummaryStrip";

export function StudioPage() {
  return (
    <div className="min-h-dvh px-5 py-6 sm:px-6 lg:px-8 lg:py-8">
      <div className="mx-auto max-w-[1240px]">
        <BackendPageHeader
          icon={Hammer}
          sectionLabel="Studio"
          sourceLabel="coming soon"
          title="工作台"
          description="先把工作台入口挂进新的导航壳里，具体功能在后续 PR 再逐步接入。"
          stats={[
            { label: "状态", value: "预留" },
            { label: "阶段", value: "shell ready" },
          ]}
        />

        <div className="mt-6">
          <BackendSummaryStrip
            items={[
              { label: "当前状态", value: "未接入具体工具" },
              { label: "目标", value: "承接后续工作流能力" },
            ]}
          />
        </div>

        <div className="mt-6">
          <BackendDetailSection title="后续接入计划" description="这条 PR 只负责把工作台页面占位接进 app shell。">
            <div className="space-y-3 text-sm leading-7 text-mute">
              <p>后续会在这里接入更完整的工作流入口、运行面板和辅助能力。</p>
              <p>当前先确保导航、路由和页面骨架稳定，避免和聊天页、渠道页耦合。</p>
            </div>
          </BackendDetailSection>
        </div>
      </div>
    </div>
  );
}
