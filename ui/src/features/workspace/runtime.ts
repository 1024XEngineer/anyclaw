export type RuntimeProfile = {
  name: string;
  title: string;
  description: string;
  provider: string;
  model: string;
  permission: string;
  workDir: string;
  workspace: string;
  language: string;
  orchestrator: string;
};

export const runtimeProfile: RuntimeProfile = {
  name: "binbin",
  title: "默认主 Agent",
  description: "当前运行时的核心代理，负责接住对话、调用技能并把任务交给后续执行链路。",
  provider: "DashScope Compatible",
  model: "qwen-max",
  permission: "limited",
  workDir: ".anyclaw",
  workspace: "workflows",
  language: "中文",
  orchestrator: "已开启",
};

export const appearanceSettings = [
  {
    label: "首页模式",
    value: "欢迎页 + 输入区 + 工作台骨架",
    hint: "保持首页干净，不回到大卡片推荐流。",
  },
  {
    label: "界面风格",
    value: "浅色玻璃拟态",
    hint: "继续沿用当前柔和、桌面工作台式的视觉语气。",
  },
  {
    label: "动效节奏",
    value: "轻量过渡",
    hint: "第二轮只做抽屉、弹窗和菜单的微动画。",
  },
];

export const runtimeSettings = [
  {
    label: "默认模型",
    value: "qwen-max",
    hint: "后续可以在这里切换主模型与推理档位。",
  },
  {
    label: "权限级别",
    value: "limited",
    hint: "保持受控执行，避免前端壳看起来可做但实际越权。",
  },
  {
    label: "工作目录",
    value: "workflows / .anyclaw",
    hint: "第三轮如果做真实状态，会把运行目录和实例状态接上来。",
  },
];

export const channelSettings = [
  {
    label: "优先接入",
    value: "微信、飞书",
    hint: "先覆盖最重要的业务入口，再补充海外和团队工具链。",
  },
  {
    label: "已预留配置",
    value: "Telegram、Slack、Discord",
    hint: "配置层已存在，第二轮只展示统一入口。",
  },
  {
    label: "市场联动",
    value: "规划中",
    hint: "未来会和云端市场联动，形成连接中心。",
  },
];
