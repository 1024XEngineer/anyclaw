import { useQuery } from "@tanstack/react-query";

import { workspaceSnapshot } from "@/generated/workspaceSnapshot.generated";
import { normalizeSkillDescription } from "@/features/workspace/skillDescription";

export type AgentRecord = {
  active: boolean;
  model: string;
  name: string;
  permissionLevel: string;
  providerName: string;
  role: string;
  skillsCount: number;
  status: string;
  summary: string;
  tags: string[];
  workingDir: string;
};

export type SkillRecord = {
  description: string;
  enabled: boolean;
  installCommand: string;
  loaded: boolean;
  name: string;
  registry: string;
  source: string;
  version: string;
};

export type ChannelRecord = {
  configured: boolean;
  enabled: boolean;
  healthy: boolean;
  name: string;
  note: string;
  running: boolean;
  slug: string;
  status: string;
  summary: string;
};

export type ProviderRecord = {
  enabled: boolean;
  health: string;
  id: string;
  isDefault: boolean;
  model: string;
  name: string;
  provider: string;
};

export type SettingRecord = {
  hint: string;
  label: string;
  value: string;
};

export type CloudRecord = {
  status: string;
  summary: string;
  title: string;
};

export type RuntimeProfile = {
  address: string;
  description: string;
  events: number;
  gatewayOnline: boolean;
  gatewaySource: string;
  language: string;
  model: string;
  name: string;
  orchestrator: string;
  permission: string;
  provider: string;
  providerLabel: string;
  providersCount: number;
  runtimes: number;
  secured: boolean;
  sessions: number;
  skills: number;
  startedAt: string;
  title: string;
  tools: number;
  workDir: string;
  workspace: string;
};

export type WorkspaceOverview = {
  appearanceSettings: SettingRecord[];
  channelSettings: SettingRecord[];
  cloudRoadmap: CloudRecord[];
  extensionAdapters: string[];
  localAgents: AgentRecord[];
  localSkills: SkillRecord[];
  meta: {
    generatedAt: string;
    liveConnected: boolean;
    sourceLabel: string;
  };
  priorityChannels: ChannelRecord[];
  providers: ProviderRecord[];
  runtimeProfile: RuntimeProfile;
  runtimeSettings: SettingRecord[];
};

type LiveStatus = {
  address?: string;
  events?: number;
  model?: string;
  provider?: string;
  secured?: boolean;
  sessions?: number;
  skills?: number;
  started_at?: string;
  tools?: number;
  work_dir?: string;
  working_dir?: string;
};

type LiveProvider = {
  default_model?: string;
  enabled?: boolean;
  health?: { message?: string; status?: string };
  id?: string;
  is_default?: boolean;
  name?: string;
  provider?: string;
};

type LiveAgent = {
  active?: boolean;
  default_model?: string;
  description?: string;
  enabled?: boolean;
  name?: string;
  permission_level?: string;
  provider?: string;
  provider_name?: string;
  role?: string;
  skills?: Array<{ enabled?: boolean; name?: string }>;
  working_dir?: string;
};

type LiveSkill = {
  description?: string;
  enabled?: boolean;
  installHint?: string;
  loaded?: boolean;
  name?: string;
  registry?: string;
  source?: string;
  version?: string;
};

type LiveChannel = {
  enabled?: boolean;
  healthy?: boolean;
  last_error?: string;
  name?: string;
  running?: boolean;
};

type LiveRuntime = {
  key?: string;
  session_count?: number;
  workspace?: string;
};

type LivePayload = {
  agents: LiveAgent[];
  channels: LiveChannel[];
  providers: LiveProvider[];
  runtimes: LiveRuntime[];
  skills: LiveSkill[];
  status: LiveStatus | null;
};

type SnapshotProvider = {
  defaultModel: string;
  enabled: boolean;
  id: string;
  isDefault: boolean;
  name: string;
  provider: string;
};

type SnapshotAgent = {
  active: boolean;
  defaultModel: string;
  description: string;
  enabled: boolean;
  name: string;
  permissionLevel: string;
  providerRef: string;
  role: string;
  skills: ReadonlyArray<{ enabled?: boolean; name?: string }>;
  workingDir: string;
};

type SnapshotSkill = {
  description: string;
  installCommand: string;
  name: string;
  registry: string;
  source: string;
  version: string;
};

type SnapshotExtension = {
  channels: ReadonlyArray<string>;
  name: string;
};

type SnapshotConfiguredChannel = {
  configured: boolean;
  enabled: boolean;
  key: string;
};

const snapshotProviders = workspaceSnapshot.providers as readonly SnapshotProvider[];
const snapshotAgents = workspaceSnapshot.agents as readonly SnapshotAgent[];
const snapshotSkills = workspaceSnapshot.skills as readonly SnapshotSkill[];
const snapshotExtensions = workspaceSnapshot.extensions as readonly SnapshotExtension[];
const snapshotConfiguredChannels = workspaceSnapshot.configuredChannels as readonly SnapshotConfiguredChannel[];

const channelMeta: Record<string, { name: string; note: string; summary: string }> = {
  wechat: {
    name: "微信",
    summary: "面向微信生态的核心接入入口，适合作为私域服务、通知和业务触达主渠道。",
    note: "优先承接私域服务、通知消息和业务触达。",
  },
  feishu: {
    name: "飞书",
    summary: "适合内部协作、机器人通知和企业工作流接入。",
    note: "后续可继续接审批、知识库和机器人能力。",
  },
  telegram: {
    name: "Telegram",
    summary: "适合海外用户、自助接入和自动推送场景。",
    note: "配置层已经预留，可继续接入真实运行状态。",
  },
  slack: {
    name: "Slack",
    summary: "适合团队协作、告警通知和工作流消息分发。",
    note: "更适合团队空间内的高频通知流。",
  },
  discord: {
    name: "Discord",
    summary: "适合社区交互、Bot 运营和公共频道接入。",
    note: "扩展目录已存在，可继续接入真实运行态。",
  },
  whatsapp: {
    name: "WhatsApp",
    summary: "适合国际用户的私域消息和服务触达。",
    note: "配置层已经预留，后续可接 webhook 与模板消息。",
  },
  signal: {
    name: "Signal",
    summary: "适合更强调私密性的点对点消息场景。",
    note: "当前更适合作为补充型渠道位。",
  },
};

const priorityChannelOrder = ["wechat", "feishu", "telegram", "slack", "discord"] as const;

const cloudRoadmap: CloudRecord[] = [
  {
    title: "云端 Skill Hub",
    status: "预留位",
    summary: "未来可以像 ClawHub 一样浏览、安装和更新云端 Skill。",
  },
  {
    title: "云端 Agent Catalog",
    status: "预留位",
    summary: "后续支持把云端 Agent 作为模板或商品直接接进来使用。",
  },
  {
    title: "统一连接中心",
    status: "规划中",
    summary: "本地能力、云端能力和渠道接入会统一汇总在一个视图里。",
  },
];

function fallbackAgentDescription(name: string) {
  return `${name} 已存在于当前仓库结构中，后续可以继续接入更细的实时状态。`;
}

function normalizeLanguage(language: string) {
  if (language.toUpperCase() === "CN") return "中文";
  if (language.toUpperCase() === "EN") return "English";
  return language || "未设置";
}

function compactText(value: string, fallback: string) {
  const text = value.trim();
  return text === "" ? fallback : text;
}

async function fetchJSON<T>(input: string): Promise<T | null> {
  try {
    const response = await fetch(input, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return null;
    return (await response.json()) as T;
  } catch {
    return null;
  }
}

async function fetchLivePayload(): Promise<LivePayload> {
  const [status, providers, agents, skills, channels, runtimes] = await Promise.all([
    fetchJSON<LiveStatus>("/status"),
    fetchJSON<LiveProvider[]>("/providers"),
    fetchJSON<LiveAgent[]>("/agents"),
    fetchJSON<LiveSkill[]>("/skills"),
    fetchJSON<LiveChannel[]>("/channels"),
    fetchJSON<LiveRuntime[]>("/runtimes"),
  ]);

  return {
    agents: Array.isArray(agents) ? agents : [],
    channels: Array.isArray(channels) ? channels : [],
    providers: Array.isArray(providers) ? providers : [],
    runtimes: Array.isArray(runtimes) ? runtimes : [],
    skills: Array.isArray(skills) ? skills : [],
    status,
  };
}

function buildProviderRecords(live: LivePayload): ProviderRecord[] {
  const liveMap = new Map(
    live.providers
      .filter((provider) => (provider.id ?? "").trim() !== "")
      .map((provider) => [provider.id!.trim(), provider]),
  );
  const snapshotMap = new Map<string, SnapshotProvider>(
    snapshotProviders.map((provider) => [provider.id, provider]),
  );
  const orderedIds = [
    ...snapshotProviders.map((provider) => provider.id),
    ...[...liveMap.keys()].filter((id) => !snapshotMap.has(id)),
  ];

  return orderedIds.map((id) => {
    const liveProvider = liveMap.get(id);
    const snapshotProvider = snapshotMap.get(id);
    const enabled = liveProvider?.enabled ?? snapshotProvider?.enabled ?? true;
    const health = liveProvider?.health?.status ?? (enabled ? "ready" : "disabled");

    return {
      enabled,
      health,
      id,
      isDefault: liveProvider?.is_default ?? snapshotProvider?.isDefault ?? false,
      model: liveProvider?.default_model ?? snapshotProvider?.defaultModel ?? "",
      name: liveProvider?.name ?? snapshotProvider?.name ?? id,
      provider: liveProvider?.provider ?? snapshotProvider?.provider ?? "compatible",
    };
  });
}

function buildAgentRecords(live: LivePayload, providers: ProviderRecord[]): AgentRecord[] {
  const providerById = new Map(providers.map((provider) => [provider.id, provider]));
  const liveMap = new Map(
    live.agents
      .filter((agent) => (agent.name ?? "").trim() !== "")
      .map((agent) => [agent.name!.trim(), agent]),
  );

  const items: AgentRecord[] = snapshotAgents.map((agent) => {
    const liveAgent = liveMap.get(agent.name);
    const linkedProvider = providerById.get(agent.providerRef);
    const providerName =
      liveAgent?.provider_name?.trim() ||
      linkedProvider?.name ||
      liveAgent?.provider?.trim() ||
      "默认 Provider";
    const skillsCount =
      liveAgent?.skills?.filter((skill) => skill?.enabled ?? true).length ?? agent.skills.length;
    const active = liveAgent?.active ?? agent.active;
    const enabled = liveAgent?.enabled ?? agent.enabled;
    const model =
      liveAgent?.default_model?.trim() ||
      agent.defaultModel ||
      (providers.find((provider) => provider.isDefault)?.model ?? "");

    return {
      active,
      model,
      name: agent.name,
      permissionLevel: liveAgent?.permission_level?.trim() || agent.permissionLevel,
      providerName,
      role: liveAgent?.role?.trim() || (agent.role === "main" ? "主 Agent" : "Agent Profile"),
      skillsCount,
      status: active ? "当前启用" : enabled ? "已配置" : "已停用",
      summary: compactText(
        liveAgent?.description?.trim() || agent.description,
        fallbackAgentDescription(agent.name),
      ),
      tags: [
        active ? "当前入口" : "候选 Agent",
        liveAgent?.permission_level?.trim() || agent.permissionLevel,
        providerName,
        model,
        `${skillsCount} skills`,
      ].filter(Boolean),
      workingDir: liveAgent?.working_dir?.trim() || agent.workingDir,
    };
  });

  if (workspaceSnapshot.orchestrator.enabled) {
    items.push({
      active: false,
      model: "",
      name: "Orchestrator",
      permissionLevel: "system",
      providerName: "协调层",
      role: "协同编排",
      skillsCount: 0,
      status: "已开启",
      summary: `当前已开启多 Agent 编排，并发上限 ${workspaceSnapshot.orchestrator.maxConcurrentAgents}。`,
      tags: [
        `${workspaceSnapshot.orchestrator.maxConcurrentAgents} 并发`,
        `${workspaceSnapshot.orchestrator.retryCount} 次重试`,
      ],
      workingDir: workspaceSnapshot.agent.workingDir,
    });
  }

  return items;
}

function buildSkillRecords(live: LivePayload): SkillRecord[] {
  const liveMap = new Map(
    live.skills
      .filter((skill) => (skill.name ?? "").trim() !== "")
      .map((skill) => [skill.name!.trim(), skill]),
  );

  return snapshotSkills
    .map((skill) => {
      const liveSkill = liveMap.get(skill.name);

      return {
        description: normalizeSkillDescription(
          liveSkill?.description?.trim() || skill.description,
          `${skill.name} 已经存在于本地技能目录中。`,
        ),
        enabled: liveSkill?.enabled ?? (liveSkill?.loaded ?? liveSkill !== undefined),
        installCommand: liveSkill?.installHint?.trim() || skill.installCommand,
        loaded: liveSkill?.loaded ?? liveSkill !== undefined,
        name: skill.name,
        registry: liveSkill?.registry?.trim() || skill.registry,
        source: liveSkill?.source?.trim() || skill.source,
        version: liveSkill?.version?.trim() || skill.version,
      };
    })
    .sort((left, right) => Number(right.loaded) - Number(left.loaded) || left.name.localeCompare(right.name));
}

function deriveChannelStatus(
  slug: string,
  configured: boolean,
  enabled: boolean,
  running: boolean,
  healthy: boolean,
  lastError: string,
  hasExtension: boolean,
) {
  if (running && healthy) return "运行中";
  if (enabled && lastError !== "") return "异常";
  if (enabled) return "已启用";
  if (configured) return "已配置";
  if (hasExtension) return "适配器已就位";
  if (slug === "wechat" || slug === "feishu") return "待接入";
  return "未接入";
}

function buildChannelRecords(live: LivePayload): ChannelRecord[] {
  const liveMap = new Map(
    live.channels
      .filter((channel) => (channel.name ?? "").trim() !== "")
      .map((channel) => [channel.name!.trim().toLowerCase(), channel]),
  );
  const configuredMap = new Map<string, SnapshotConfiguredChannel>(
    snapshotConfiguredChannels.map((channel) => [channel.key, channel]),
  );
  const extensionChannels = new Set<string>(
    snapshotExtensions.flatMap((extension) => [...extension.channels]),
  );

  return priorityChannelOrder.map((slug) => {
    const meta = channelMeta[slug];
    const liveChannel = liveMap.get(slug);
    const configured = configuredMap.get(slug)?.configured ?? false;
    const enabled = liveChannel?.enabled ?? configuredMap.get(slug)?.enabled ?? false;
    const running = liveChannel?.running ?? false;
    const healthy = liveChannel?.healthy ?? false;
    const lastError = liveChannel?.last_error?.trim() ?? "";
    const hasExtension = extensionChannels.has(slug);

    return {
      configured,
      enabled,
      healthy,
      name: meta.name,
      note: lastError || meta.note,
      running,
      slug,
      status: deriveChannelStatus(slug, configured, enabled, running, healthy, lastError, hasExtension),
      summary: meta.summary,
    };
  });
}

function buildRuntimeProfile(
  live: LivePayload,
  providers: ProviderRecord[],
  agents: AgentRecord[],
  skills: SkillRecord[],
): RuntimeProfile {
  const activeAgent = agents.find((agent) => agent.active) ?? agents[0];
  const defaultProvider = providers.find((provider) => provider.isDefault) ?? providers[0];
  const status = live.status;
  const gatewayOnline = status !== null;
  const gatewayAddress =
    status?.address?.trim() || `${workspaceSnapshot.gateway.host}:${workspaceSnapshot.gateway.port}`;

  return {
    address: gatewayAddress,
    description: activeAgent?.summary ?? workspaceSnapshot.agent.description,
    events: status?.events ?? 0,
    gatewayOnline,
    gatewaySource: gatewayOnline ? "网关在线 + 仓库快照" : "仓库快照",
    language: normalizeLanguage(workspaceSnapshot.agent.language),
    model: status?.model?.trim() || activeAgent?.model || defaultProvider?.model || "未配置",
    name: activeAgent?.name ?? workspaceSnapshot.agent.name,
    orchestrator: workspaceSnapshot.orchestrator.enabled
      ? `已开启 · 并发 ${workspaceSnapshot.orchestrator.maxConcurrentAgents}`
      : "未开启",
    permission: activeAgent?.permissionLevel ?? workspaceSnapshot.agent.permissionLevel,
    provider: status?.provider?.trim() || defaultProvider?.provider || "unknown",
    providerLabel: defaultProvider?.name || "默认 Provider",
    providersCount: providers.length,
    runtimes: live.runtimes.length,
    secured: status?.secured ?? false,
    sessions: status?.sessions ?? 0,
    skills: status?.skills ?? skills.filter((skill) => skill.enabled).length,
    startedAt: status?.started_at?.trim() || "",
    title: activeAgent?.active ? "当前主 Agent" : "默认主 Agent",
    tools: status?.tools ?? 0,
    workDir: status?.work_dir?.trim() || workspaceSnapshot.agent.workDir,
    workspace: status?.working_dir?.trim() || workspaceSnapshot.agent.workingDir,
  };
}

function buildAppearanceSettings(runtimeProfile: RuntimeProfile): SettingRecord[] {
  return [
    {
      label: "首页模式",
      value: "欢迎页 + 输入区 + 工作台骨架",
      hint: "首页继续保持干净，不放推荐卡片。",
    },
    {
      label: "导航结构",
      value: "对话 / 市场 / 渠道 / 设置",
      hint: "当前继续保持信息架构稳定，不回到静态宣传页。",
    },
    {
      label: "数据来源",
      value: runtimeProfile.gatewaySource,
      hint: runtimeProfile.gatewayOnline
        ? "已叠加当前网关状态。"
        : "网关不在线时仍可基于仓库快照预览界面。",
    },
  ];
}

function buildRuntimeSettings(
  runtimeProfile: RuntimeProfile,
  providers: ProviderRecord[],
  agents: AgentRecord[],
): SettingRecord[] {
  const activeAgent = agents.find((agent) => agent.active) ?? agents[0];
  const defaultProvider = providers.find((provider) => provider.isDefault) ?? providers[0];

  return [
    {
      label: "默认 Provider",
      value: defaultProvider?.name || "未配置",
      hint: `${runtimeProfile.provider} · ${runtimeProfile.model}`,
    },
    {
      label: "当前 Agent",
      value: activeAgent?.name || runtimeProfile.name,
      hint: `${runtimeProfile.permission} · 工作目录 ${runtimeProfile.workspace}`,
    },
    {
      label: "网关状态",
      value: runtimeProfile.gatewayOnline ? "在线" : "离线",
      hint: `${runtimeProfile.address} · 会话 ${runtimeProfile.sessions} · 运行时 ${runtimeProfile.runtimes}`,
    },
  ];
}

function buildChannelSettings(channels: ChannelRecord[], extensionAdapters: string[]): SettingRecord[] {
  const enabledCount = channels.filter((channel) => channel.enabled || channel.running).length;
  const configuredCount = channels.filter((channel) => channel.configured).length;

  return [
    {
      label: "优先接入",
      value: "微信、飞书",
      hint: `当前优先渠道已启用 ${enabledCount} 个，已配置 ${configuredCount} 个。`,
    },
    {
      label: "连接器规模",
      value: `${extensionAdapters.length} 个扩展适配器`,
      hint: "仓库里的渠道扩展清单已经自动汇总进来。",
    },
    {
      label: "安全策略",
      value: workspaceSnapshot.security.mentionGate ? "提及门已开启" : "提及门未开启",
      hint: workspaceSnapshot.security.defaultDenyDM ? "默认拒绝私信开启。" : "默认私信放开。",
    },
  ];
}

function buildOverview(live: LivePayload): WorkspaceOverview {
  const providers = buildProviderRecords(live);
  const localAgents = buildAgentRecords(live, providers);
  const localSkills = buildSkillRecords(live);
  const priorityChannels = buildChannelRecords(live);
  const extensionAdapters = snapshotExtensions.map((extension) => extension.name);
  const runtimeProfile = buildRuntimeProfile(live, providers, localAgents, localSkills);

  return {
    appearanceSettings: buildAppearanceSettings(runtimeProfile),
    channelSettings: buildChannelSettings(priorityChannels, extensionAdapters),
    cloudRoadmap,
    extensionAdapters,
    localAgents,
    localSkills,
    meta: {
      generatedAt: workspaceSnapshot.generatedAt,
      liveConnected: runtimeProfile.gatewayOnline,
      sourceLabel: runtimeProfile.gatewaySource,
    },
    priorityChannels,
    providers,
    runtimeProfile,
    runtimeSettings: buildRuntimeSettings(runtimeProfile, providers, localAgents),
  };
}

function initialOverview(): WorkspaceOverview {
  return buildOverview({
    agents: [],
    channels: [],
    providers: [],
    runtimes: [],
    skills: [],
    status: null,
  });
}

async function loadWorkspaceOverview() {
  const live = await fetchLivePayload();
  return buildOverview(live);
}

export function useWorkspaceOverview() {
  return useQuery({
    queryKey: ["workspace-overview"],
    queryFn: loadWorkspaceOverview,
    initialData: initialOverview(),
    refetchInterval: 15000,
    staleTime: 8000,
  });
}
