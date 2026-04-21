import type { StatusBadgeTone } from "@/features/backend-ui/StatusBadge";

const successHints = ["运行", "启用", "加载", "在线", "健康", "ready", "active", "loaded"];
const infoHints = ["配置", "识别", "本地", "预留", "规划", "等待", "catalog", "local"];
const warningHints = ["异常", "错误", "失败", "离线", "停止", "未接入", "warning", "error", "offline"];

function includesAny(label: string, hints: string[]) {
  return hints.some((hint) => label.includes(hint));
}

export function getStatusTone(label: string): StatusBadgeTone {
  const normalized = label.toLowerCase();

  if (includesAny(normalized, warningHints)) return "warning";
  if (includesAny(normalized, successHints)) return "success";
  if (includesAny(normalized, infoHints)) return "info";

  return "default";
}
