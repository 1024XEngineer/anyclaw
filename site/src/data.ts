import {
  Blocks,
  Bot,
  Braces,
  CheckCircle2,
  Command,
  FileText,
  Globe2,
  HelpCircle,
  KeyRound,
  LockKeyhole,
  PackageCheck,
  ScrollText,
  ShieldCheck,
  TerminalSquare,
  type LucideIcon
} from "lucide-react";

export type MarketKind = "agent" | "skill" | "cli";

export type MarketItem = {
  id: string;
  name: string;
  kind: MarketKind;
  version: string;
  summary: string;
  source: string;
  publisher?: string;
  risk: string;
  riskLabel?: string;
  trust: string;
  trustLabel?: string;
  permissions: string[];
  tags?: string[];
  hitSignals?: string[];
  compatibility?: {
    anyclaw_min?: string;
    os?: string[];
    arch?: string[];
  };
  sizeBytes?: number;
  score?: number;
  updatedAt?: string;
};

export type MarketVersion = {
  version: string;
  releasedAt?: string;
  changelog?: string;
  permissionsDiff?: string[];
  sizeBytes?: number;
  deprecated?: boolean;
};

export type FAQItem = {
  question: string;
  answer: string;
};

export type DocLink = {
  title: string;
  description: string;
  href: string;
  icon: LucideIcon;
};

export const featureBlocks = [
  {
    title: "一个入口，连接真实任务",
    description: "把工作区、工具执行、浏览器和 Web 控制台接在一起，让 Agent 能在你的本地环境里完成可验证的工作。",
    icon: Blocks
  },
  {
    title: "统一能力市场",
    description: "Agent / Skill / CLI 通过 Registry 发现、安装和绑定，状态清楚，升级和卸载可追踪。",
    icon: PackageCheck
  },
  {
    title: "可控，不黑盒",
    description: "策略、权限、风险等级、审计日志和事件记录参与每一步决策，自动补能也受策略约束。",
    icon: ShieldCheck
  }
];

export const quickstartSteps = [
  "从源码构建 AnyClaw CLI。",
  "运行 onboard 配置模型供应商和本地工作区。",
  "启动交互模式或 Gateway，再从市场安装需要的能力。"
];

export const docLinks: DocLink[] = [
  {
    title: "Quickstart",
    description: "从构建 CLI、onboard 到启动交互模式的最短路径。",
    href: "https://github.com/1024XEngineer/anyclaw#快速开始",
    icon: CheckCircle2
  },
  {
    title: "Deployment",
    description: "Docker Compose、环境变量、端口和服务器部署说明。",
    href: "https://github.com/1024XEngineer/anyclaw/blob/main/docs/DEPLOYMENT.md",
    icon: Globe2
  },
  {
    title: "Marketplace Registry",
    description: "Registry API、发布、quarantine、download 和审计接口。",
    href: "https://github.com/1024XEngineer/anyclaw/blob/main/docs/MARKETPLACE_REGISTRY_DEV.md",
    icon: Braces
  },
  {
    title: "Registry Runbook",
    description: "生产启动、publisher token、smoke test 和当前限制。",
    href: "https://github.com/1024XEngineer/anyclaw/blob/main/docs/REGISTRY_PRODUCTION_RUNBOOK.md",
    icon: FileText
  },
  {
    title: "Security",
    description: "安装策略、checksum、quarantine、包签名方案和 token 边界。",
    href: "https://github.com/1024XEngineer/anyclaw/blob/main/docs/SECURITY.md",
    icon: ShieldCheck
  },
  {
    title: "Publishing",
    description: "发布 manifest、publisher token、checksum 和发布 smoke checklist。",
    href: "https://github.com/1024XEngineer/anyclaw/blob/main/docs/PUBLISHING.md",
    icon: ScrollText
  }
];

export const downloadCommands = {
  windows: `git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw.exe ./cmd/anyclaw
.\\anyclaw.exe onboard
.\\anyclaw.exe doctor
.\\anyclaw.exe -i`,
  unix: `git clone https://github.com/1024XEngineer/anyclaw.git
cd anyclaw
go build -o anyclaw ./cmd/anyclaw
./anyclaw onboard
./anyclaw doctor
./anyclaw -i`,
  docker: `cp .env.production.example .env.production
# edit .env.production and set ANYCLAW_REGISTRY_ADMIN_TOKEN
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build`
};

export type DownloadOption = {
  platform: string;
  status: "available" | "planned";
  description: string;
  commandKey?: keyof typeof downloadCommands;
  releaseAsset?: string;
};

export const downloadOptions: DownloadOption[] = [
  {
    platform: "Windows",
    status: "available",
    description: "当前推荐从源码构建 CLI，输出 anyclaw.exe 后运行 onboard 和 doctor。",
    commandKey: "windows",
    releaseAsset: "anyclaw_VERSION_windows_amd64.zip"
  },
  {
    platform: "macOS / Linux",
    status: "available",
    description: "当前推荐从源码构建 CLI。Release 发布后会提供 darwin/linux 的 tar.gz。",
    commandKey: "unix",
    releaseAsset: "anyclaw_VERSION_linux_amd64.tar.gz"
  },
  {
    platform: "Docker Compose Gateway",
    status: "available",
    description: "适合服务器运行 Gateway、Registry 和官网，先设置 .env.production 再启动。",
    commandKey: "docker"
  }
];

export const releaseAssets = [
  "anyclaw_VERSION_windows_amd64.zip",
  "anyclaw_VERSION_linux_amd64.tar.gz",
  "anyclaw_VERSION_linux_arm64.tar.gz",
  "anyclaw_VERSION_darwin_amd64.tar.gz",
  "anyclaw_VERSION_darwin_arm64.tar.gz"
];

export const installChecklist = [
  "安装 Go 1.25+。",
  "克隆仓库并构建 CLI。",
  "运行 onboard 写入本地配置。",
  "运行 doctor 检查模型供应商和工作区。",
  "进入交互模式或连接 Gateway。"
];

export const marketplaceInstallSteps = [
  "在 AnyClaw 桌面端或 Web 控制台打开“能力市场”。",
  "选择云端条目，先查看风险、可信度、权限和版本权限 diff。",
  "点击安装并确认策略提示。高危权限需要额外确认。",
  "安装成功后再选择绑定目标，例如主代理、工作区或全局运行时。"
];

export const faqItems: FAQItem[] = [
  {
    question: "官网市场能直接安装吗？",
    answer: "官网是公开目录和安装指引，不保存用户 token，也不直接操作你的本地运行时。真正安装在 AnyClaw 客户端里完成。"
  },
  {
    question: "为什么有些条目显示英文？",
    answer: "artifact 名称、描述、权限、changelog 都来自发布者 manifest，官网只翻译界面控件，不强制翻译发布内容。"
  },
  {
    question: "checksum 和签名是什么关系？",
    answer: "当前安装链路强制校验 checksum。签名字段已经预留并写入安全方案，后续启用签名验证前不会假装已经完成。"
  },
  {
    question: "quarantine 后用户还能下载吗？",
    answer: "不能。Registry 会让 resolve/download 返回不可用状态，客户端也会把原因展示出来。"
  }
];

export const publishLifecycle = [
  {
    title: "准备包",
    description: "agent / skill / cli 包需要携带 anyclaw.artifact.json，用来声明类型、版本、权限、风险和下载元数据。",
    icon: Bot
  },
  {
    title: "申请 Token",
    description: "管理员使用 admin token 创建 publisher token。发布者只拿到自己的 publisher token，不共享 admin token。",
    icon: KeyRound
  },
  {
    title: "发布与审计",
    description: "包进入 Registry 后会产生审计记录；必要时管理员可以 quarantine，下载次数也会被记录。",
    icon: LockKeyhole
  }
];

export const publishCommands = {
  createToken: `.\\scripts\\registry-create-publisher-token.ps1 \`
  -BaseUrl http://127.0.0.1:8791 \`
  -AdminToken $env:ANYCLAW_REGISTRY_ADMIN_TOKEN \`
  -PublisherId "AnyClaw Labs"`,
  publishSkill: `.\\scripts\\registry-publish-artifact.ps1 \`
  -BaseUrl http://127.0.0.1:8791 \`
  -PublisherToken $env:ANYCLAW_PUBLISHER_TOKEN \`
  -Manifest examples/marketplace/skill-release-notes/anyclaw.artifact.json`
};

export const kindLabels: Record<MarketKind, string> = {
  agent: "代理",
  skill: "技能",
  cli: "命令行"
};

export const kindIcons: Record<MarketKind, LucideIcon> = {
  agent: Bot,
  skill: Command,
  cli: TerminalSquare
};

export const supportLinks: DocLink[] = [
  {
    title: "FAQ",
    description: "查看市场安装、安全校验和发布内容展示规则。",
    href: "/docs#faq",
    icon: HelpCircle
  },
  {
    title: "更新日志",
    description: "查看仓库 release 和版本变更入口。",
    href: "https://github.com/1024XEngineer/anyclaw/releases",
    icon: ScrollText
  }
];
