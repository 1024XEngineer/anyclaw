function stripWrappingQuotes(value: string) {
  const pairs: Array<[string, string]> = [
    ['"', '"'],
    ["'", "'"],
  ];

  for (const [start, end] of pairs) {
    if (value.startsWith(start) && value.endsWith(end) && value.length >= start.length + end.length) {
      return value.slice(start.length, value.length - end.length).trim();
    }
  }

  return value;
}

export function normalizeSkillDescription(value: string, fallback: string) {
  const normalized = stripWrappingQuotes(value.replace(/\s+/g, " ").trim())
    .replace(/`([^`]+)`/g, "$1")
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, "$1")
    .replace(/\s+/g, " ")
    .trim();

  if (normalized === "") return fallback;
  if (normalized.length <= 160) return normalized;
  return `${normalized.slice(0, 157).trimEnd()}...`;
}
