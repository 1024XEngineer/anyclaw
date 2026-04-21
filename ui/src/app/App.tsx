import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Bot, Cable, Cloud, LayoutDashboard, Sparkles } from "lucide-react";
import { useState } from "react";

import { BackendDetailSection } from "@/features/backend-ui/BackendDetailSection";
import { BackendEmptyState } from "@/features/backend-ui/BackendEmptyState";
import { BackendPageHeader } from "@/features/backend-ui/BackendPageHeader";
import { BackendPropertyList } from "@/features/backend-ui/BackendPropertyList";
import { BackendSectionHeader } from "@/features/backend-ui/BackendSectionHeader";
import { BackendSummaryStrip } from "@/features/backend-ui/BackendSummaryStrip";
import { StatusBadge } from "@/features/backend-ui/StatusBadge";
import { BackendToolbar } from "@/features/backend-ui/BackendToolbar";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { type AgentRecord, type ChannelRecord, type SkillRecord, useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

type OverviewFilter = "all" | "agents" | "skills" | "channels";

function matchesQuery(query: string, fields: Array<string | undefined>) {
  const normalized = query.trim().toLowerCase();
  if (normalized === "") return true;

  return fields
    .filter((field): field is string => Boolean(field))
    .join(" ")
    .toLowerCase()
    .includes(normalized);
}

function providerTone(health: string) {
  switch (health.trim().toLowerCase()) {
    case "ready":
    case "reachable":
      return "success" as const;
    case "missing_key":
    case "invalid":
    case "invalid_base_url":
      return "warning" as const;
    default:
      return "info" as const;
  }
}

function formatStartedAt(value: string) {
  if (!value) return "未上报";

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;

  return date.toLocaleString("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  });
}

function AgentList({ items }: { items: AgentRecord[] }) {
  if (items.length === 0) {
    return <BackendEmptyState icon={Bot} title="没有匹配的 Agent" description="调整筛选条件后再试一次。" />;
  }

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      {items.map((agent) => (
        <BackendDetailSection
          key={agent.name}
          title={agent.name}
          description={agent.summary}
        >
          <div className="flex flex-wrap gap-2">
            <StatusBadge label={agent.status} tone={agent.active ? "success" : "info"} />
            <StatusBadge label={agent.providerName} />
            <StatusBadge label={agent.permissionLevel} />
          </div>
          <div className="mt-4">
            <BackendPropertyList
              items={[
                { label: "角色", value: agent.role },
                { label: "模型", value: agent.model || "未配置" },
                { label: "技能数", value: String(agent.skillsCount) },
                { label: "工作目录", value: agent.workingDir },
              ]}
            />
          </div>
        </BackendDetailSection>
      ))}
    </div>
  );
}

function SkillList({ items }: { items: SkillRecord[] }) {
  if (items.length === 0) {
    return <BackendEmptyState icon={Sparkles} title="没有匹配的 Skill" description="当前筛选条件下没有找到对应项。" />;
  }

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      {items.map((skill) => (
        <BackendDetailSection
          key={skill.name}
          title={skill.name}
          description={skill.description}
        >
          <div className="flex flex-wrap gap-2">
            <StatusBadge label={skill.loaded ? "已加载" : "本地识别"} tone={skill.loaded ? "success" : "info"} />
            {skill.version ? <StatusBadge label={skill.version} /> : null}
            <StatusBadge label={skill.registry || skill.source || "local"} />
          </div>
          <div className="mt-4">
            <BackendPropertyList
              items={[
                { label: "启用状态", value: skill.enabled ? "已启用" : "未启用" },
                { label: "安装来源", value: skill.source || "local" },
                { label: "安装提示", value: skill.installCommand || "仓库内置能力" },
              ]}
            />
          </div>
        </BackendDetailSection>
      ))}
    </div>
  );
}

function ChannelList({ items }: { items: ChannelRecord[] }) {
  if (items.length === 0) {
    return <BackendEmptyState icon={Cable} title="没有匹配的渠道" description="可以修改搜索词或切换视图后再查看。" />;
  }

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      {items.map((channel) => (
        <BackendDetailSection
          key={channel.slug}
          title={channel.name}
          description={channel.summary}
        >
          <div className="flex flex-wrap gap-2">
            <StatusBadge label={channel.status} tone={getStatusTone(channel.status)} />
            <StatusBadge label={channel.configured ? "已配置" : "未配置"} />
            <StatusBadge label={channel.running ? "运行中" : "未运行"} tone={channel.running ? "success" : "default"} />
          </div>
          <div className="mt-4">
            <BackendPropertyList
              items={[
                { label: "渠道标识", value: channel.slug },
                { label: "启用状态", value: channel.enabled ? "已启用" : "未启用" },
                { label: "健康状态", value: channel.healthy ? "健康" : "待检查" },
                { label: "备注", value: channel.note },
              ]}
            />
          </div>
        </BackendDetailSection>
      ))}
    </div>
  );
}

function WorkspaceOverviewApp() {
  const { data } = useWorkspaceOverview();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<OverviewFilter>("all");

  const visibleAgents = data.localAgents.filter((agent) =>
    matchesQuery(query, [agent.name, agent.summary, agent.providerName, agent.role, agent.model]),
  );
  const visibleSkills = data.localSkills.filter((skill) =>
    matchesQuery(query, [skill.name, skill.description, skill.registry, skill.source, skill.version]),
  );
  const visibleChannels = data.priorityChannels.filter((channel) =>
    matchesQuery(query, [channel.name, channel.slug, channel.status, channel.summary, channel.note]),
  );

  return (
    <div className="min-h-dvh bg-[#f6f8fb] text-ink">
      <div className="mx-auto max-w-[1440px] px-5 py-6 sm:px-6 lg:px-8 lg:py-8">
        <BackendPageHeader
          icon={LayoutDashboard}
          sectionLabel="Workspace"
          sourceLabel={data.meta.sourceLabel}
          title="工作区总览"
          description="这一页先把运行时、Agent、Skill 和重点渠道汇总出来，作为后续聊天页、设置页和渠道页的 UI 基础。"
          stats={[
            { label: "Provider", value: String(data.providers.length) },
            { label: "Agent", value: String(data.localAgents.length) },
            { label: "Skill", value: String(data.localSkills.length) },
            { label: "渠道", value: String(data.priorityChannels.length) },
          ]}
        />

        <div className="mt-6">
          <BackendSummaryStrip
            items={[
              {
                active: data.meta.liveConnected,
                label: "数据来源",
                value: data.meta.sourceLabel,
              },
              {
                label: "默认模型",
                value: data.runtimeProfile.model || "未配置",
              },
              {
                label: "工作区",
                value: data.runtimeProfile.workspace,
              },
              {
                label: "启动时间",
                value: formatStartedAt(data.runtimeProfile.startedAt),
              },
            ]}
          />
        </div>

        <BackendToolbar
          groups={[
            {
              items: [
                { active: filter === "all", label: "全部", onClick: () => setFilter("all") },
                { active: filter === "agents", label: "Agent", onClick: () => setFilter("agents") },
                { active: filter === "skills", label: "Skill", onClick: () => setFilter("skills") },
                { active: filter === "channels", label: "渠道", onClick: () => setFilter("channels") },
              ],
            },
          ]}
          onSearchChange={setQuery}
          searchPlaceholder="搜索 Agent、Skill、Provider 或渠道"
          searchValue={query}
        />

        <section className="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]">
          <BackendDetailSection
            title="运行时画像"
            description="当前工作区的基础运行信息、默认模型以及运行入口。"
          >
            <BackendPropertyList
              items={[
                { label: "入口 Agent", value: data.runtimeProfile.name },
                { label: "Provider", value: data.runtimeProfile.providerLabel || data.runtimeProfile.provider },
                { label: "工作目录", value: data.runtimeProfile.workDir },
                { label: "Gateway", value: data.runtimeProfile.address },
                { label: "会话数", value: String(data.runtimeProfile.sessions) },
                { label: "运行时", value: String(data.runtimeProfile.runtimes) },
              ]}
            />
          </BackendDetailSection>

          <BackendDetailSection
            title="Provider 概览"
            description="先把模型配置的当前状态展示出来，后续设置弹层会在这个基础上接入。"
          >
            <div className="space-y-3">
              {data.providers.map((provider) => (
                <div
                  key={provider.id}
                  className="rounded-[18px] border border-skin bg-[#fbfcfe] px-4 py-4"
                >
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <div className="text-[18px] font-semibold tracking-[-0.03em] text-ink">
                        {provider.name}
                      </div>
                      <div className="mt-1 text-sm text-mute">
                        {provider.provider} · {provider.model || "未配置模型"}
                      </div>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {provider.isDefault ? <StatusBadge label="默认" tone="success" /> : null}
                      <StatusBadge label={provider.health || "unknown"} tone={providerTone(provider.health)} />
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </BackendDetailSection>
        </section>

        {(filter === "all" || filter === "agents") && (
          <section className="mt-8">
            <BackendSectionHeader
              title="本地 Agent"
              countLabel={`${visibleAgents.length} items`}
              description="优先把本地 Agent 的工作角色、模型和能力挂载关系透出来。"
            />
            <div className="mt-4">
              <AgentList items={visibleAgents} />
            </div>
          </section>
        )}

        {(filter === "all" || filter === "skills") && (
          <section className="mt-8">
            <BackendSectionHeader
              title="本地 Skill"
              countLabel={`${visibleSkills.length} items`}
              description="Skill 的描述、来源和启用状态在这里做统一整理。"
            />
            <div className="mt-4">
              <SkillList items={visibleSkills} />
            </div>
          </section>
        )}

        {(filter === "all" || filter === "channels") && (
          <section className="mt-8">
            <BackendSectionHeader
              title="优先渠道"
              countLabel={`${visibleChannels.length} items`}
              description="重点看当前渠道是否已配置、是否已启用，以及运行健康状态。"
            />
            <div className="mt-4">
              <ChannelList items={visibleChannels} />
            </div>
          </section>
        )}

        <section className="mt-8 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <BackendDetailSection
            title="界面约定"
            description="这些设置决定了后续聊天页、设置页和渠道页的整体 UI 方向。"
          >
            <BackendPropertyList
              items={data.appearanceSettings.map((item) => ({
                label: item.label,
                value: item.value,
              }))}
            />
          </BackendDetailSection>

          <BackendDetailSection
            title="云端能力预留"
            description="这部分先作为 roadmap 展示，后续再接上市场页和云端目录。"
          >
            {data.cloudRoadmap.length > 0 ? (
              <div className="space-y-3">
                {data.cloudRoadmap.map((item) => (
                  <div
                    key={item.title}
                    className="rounded-[18px] border border-skin bg-[#fbfcfe] px-4 py-4"
                  >
                    <div className="flex flex-wrap items-center gap-3">
                      <Cloud className="text-[#607699]" size={18} strokeWidth={2.1} />
                      <div className="text-[18px] font-semibold tracking-[-0.03em] text-ink">
                        {item.title}
                      </div>
                      <StatusBadge label={item.status} tone="info" />
                    </div>
                    <p className="mt-3 text-sm leading-7 text-mute">{item.summary}</p>
                  </div>
                ))}
              </div>
            ) : (
              <BackendEmptyState icon={Cloud} title="还没有云端规划" description="后续会把云端能力目录接到这里。" />
            )}
          </BackendDetailSection>
        </section>
      </div>
    </div>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <WorkspaceOverviewApp />
    </QueryClientProvider>
  );
}
