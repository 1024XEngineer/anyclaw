export type AgentRecord = {
  name: string;
  role: string;
  status: string;
  summary: string;
  tags: string[];
};

export type SkillRecord = {
  name: string;
  source: string;
  summary: string;
};

export type CloudRecord = {
  title: string;
  summary: string;
  status: string;
};

export type ChannelRecord = {
  name: string;
  slug: string;
  status: string;
  summary: string;
  note: string;
};

export const localAgents: AgentRecord[] = [
  {
    name: "binbin",
    role: "主 Agent",
    status: "当前配置",
    summary: "当前 AnyClaw 主控代理，负责接住对话、工作流和默认执行链路。",
    tags: ["本地运行", "默认入口"],
  },
  {
    name: "Orchestrator",
    role: "协同编排",
    status: "能力已开启",
    summary: "配置层已经开启多 Agent 编排，后续可以继续挂接专业 Agent。",
    tags: ["多 Agent", "待扩展"],
  },
];

export const localSkills: SkillRecord[] = [
  {
    name: "app-controller",
    source: "local",
    summary: "控制本地应用和系统操作的工作技能。",
  },
  {
    name: "cli-anything",
    source: "local",
    summary: "把命令行能力封装成可调用技能。",
  },
  {
    name: "coder",
    source: "local",
    summary: "偏代码生成与改写的开发技能。",
  },
  {
    name: "github",
    source: "local",
    summary: "面向 GitHub 仓库与协作动作的技能。",
  },
  {
    name: "github-issues",
    source: "local",
    summary: "聚焦 Issue 跟踪、整理和回填的技能。",
  },
  {
    name: "summarize",
    source: "local",
    summary: "面向长文档与上下文压缩的摘要技能。",
  },
  {
    name: "vision-agent",
    source: "local",
    summary: "处理截图、图片理解和视觉辅助分析。",
  },
  {
    name: "voice-assistant",
    source: "local",
    summary: "语音交互相关的输入输出能力封装。",
  },
  {
    name: "weather",
    source: "registry",
    summary: "天气查询示例技能，适合作为外部技能接入样板。",
  },
  {
    name: "web-search",
    source: "skillhub",
    summary: "把网页搜索封装成技能调用，适合联网任务。",
  },
];

export const cloudRoadmap: CloudRecord[] = [
  {
    title: "云端 Skill Hub",
    status: "预留位",
    summary: "未来可以像 ClawHub 一样浏览、安装和更新云端 Skill。",
  },
  {
    title: "云端 Agent Catalog",
    status: "预留位",
    summary: "后续支持把云端 Agent 当作商品或模板接进来直接使用。",
  },
  {
    title: "统一连接中心",
    status: "规划中",
    summary: "本地能力、云端能力和渠道接入统一汇总在一个市场视图里。",
  },
];

export const priorityChannels: ChannelRecord[] = [
  {
    name: "微信",
    slug: "wechat",
    status: "待接入",
    summary: "面向微信生态的主入口，占据优先接入位。",
    note: "适合作为核心业务入口。",
  },
  {
    name: "飞书",
    slug: "feishu",
    status: "待接入",
    summary: "适合内部协作与企业场景通知流。",
    note: "可承接机器人、审批和知识库联动。",
  },
  {
    name: "Telegram",
    slug: "telegram",
    status: "配置项已预留",
    summary: "配置文件里已有 Telegram 渠道位，当前尚未启用。",
    note: "适合海外私域与自动推送。",
  },
  {
    name: "Slack",
    slug: "slack",
    status: "配置项已预留",
    summary: "适合团队协作与工作流通知联动。",
    note: "后续可连接到团队空间与告警流。",
  },
  {
    name: "Discord",
    slug: "discord",
    status: "配置项已预留",
    summary: "社区型渠道入口，适合 Bot 与社群运营。",
    note: "已经存在扩展目录，前端先保留接入位。",
  },
];

export const extensionAdapters = [
  "BlueBubbles",
  "Discord",
  "飞书",
  "Google Chat",
  "iMessage",
  "IRC",
  "LINE",
  "Matrix",
  "Mattermost",
  "Microsoft Teams",
  "Nextcloud Talk",
  "Nostr",
  "Signal",
  "Slack",
  "Synology Chat",
  "Telegram",
  "Tlon",
  "Twitch",
  "微信",
  "WhatsApp",
  "Zalo",
  "Zalo User",
];
