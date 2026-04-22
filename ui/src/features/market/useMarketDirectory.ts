import { useSearchParams } from "react-router-dom";

import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

export type MarketKind = "agent" | "skill";
export type MarketSource = "local" | "cloud";

export type MarketDirectoryEntry = {
  chips: string[];
  footnote: string;
  id: string;
  kind: MarketKind;
  name: string;
  owner: string;
  status: string;
  summary: string;
};

function normalizeKind(value: string | null): MarketKind {
  return value === "skill" ? "skill" : "agent";
}

function normalizeSource(value: string | null): MarketSource {
  return value === "cloud" ? "cloud" : "local";
}

function matchesQuery(entry: MarketDirectoryEntry, query: string) {
  if (query === "") return true;

  const haystack = [entry.name, entry.owner, entry.summary, entry.status, entry.footnote, ...entry.chips]
    .join(" ")
    .toLowerCase();

  return haystack.includes(query.toLowerCase());
}

export function useMarketDirectory() {
  const { data } = useWorkspaceOverview();
  const [searchParams, setSearchParams] = useSearchParams();

  const kind = normalizeKind(searchParams.get("kind"));
  const source = normalizeSource(searchParams.get("source"));
  const query = (searchParams.get("q") ?? "").trim();

  const localAgentEntries: MarketDirectoryEntry[] = data.localAgents.map((agent) => ({
    chips: [agent.role, `${agent.skillsCount} skills`, agent.permissionLevel].filter(Boolean),
    footnote: `工作目录 ${agent.workingDir || "未设置"}`,
    id: `agent:${agent.name}`,
    kind: "agent",
    name: agent.name,
    owner: agent.providerName || "默认 Provider",
    status: agent.status,
    summary: agent.summary,
  }));

  const localSkillEntries: MarketDirectoryEntry[] = data.localSkills.map((skill) => ({
    chips: [skill.version || "未标记版本", skill.registry || skill.source || "本地目录"].filter(Boolean),
    footnote: skill.installCommand || "已从本地技能目录识别",
    id: `skill:${skill.name}`,
    kind: "skill",
    name: skill.name,
    owner: skill.registry || skill.source || "local",
    status: skill.loaded ? "已加载" : "本地识别",
    summary: skill.description,
  }));

  const currentLocalEntries = (kind === "agent" ? localAgentEntries : localSkillEntries).filter((entry) =>
    matchesQuery(entry, query),
  );

  const selectedParam = searchParams.get("selected");
  const selectedId =
    source === "local" && currentLocalEntries.some((entry) => entry.id === selectedParam)
      ? selectedParam
      : source === "local"
        ? (currentLocalEntries[0]?.id ?? null)
        : null;

  const selectedEntry =
    source === "local"
      ? currentLocalEntries.find((entry) => entry.id === selectedId) ?? null
      : null;

  const cloudPanels =
    kind === "agent"
      ? data.cloudRoadmap.filter((item) => item.title.includes("Agent") || item.title.includes("统一"))
      : data.cloudRoadmap.filter((item) => item.title.includes("Skill") || item.title.includes("统一"));

  function patchParams(
    patch: Partial<{
      kind: MarketKind;
      q: string;
      selected: string | null;
      source: MarketSource;
    }>,
  ) {
    const next = new URLSearchParams(searchParams);

    if (patch.kind !== undefined) {
      next.set("kind", patch.kind);
      next.delete("selected");
    }

    if (patch.source !== undefined) {
      next.set("source", patch.source);
      next.delete("selected");
    }

    if (patch.q !== undefined) {
      const value = patch.q.trim();
      if (value === "") next.delete("q");
      else next.set("q", value);
    }

    if (patch.selected !== undefined) {
      if (patch.selected) next.set("selected", patch.selected);
      else next.delete("selected");
    }

    setSearchParams(next, { replace: true });
  }

  return {
    cloudPanels,
    counts: {
      cloudAgents: 0,
      cloudSkills: 0,
      localAgents: localAgentEntries.length,
      localSkills: localSkillEntries.length,
    },
    data,
    kind,
    kindLabel: kind === "agent" ? "Agent" : "Skill",
    localEntries: currentLocalEntries,
    query,
    selectedEntry,
    selectedId,
    setKind: (nextKind: MarketKind) => patchParams({ kind: nextKind }),
    setQuery: (nextQuery: string) => patchParams({ q: nextQuery }),
    setSelected: (nextSelected: string | null) => patchParams({ selected: nextSelected }),
    setSource: (nextSource: MarketSource) => patchParams({ source: nextSource }),
    source,
    sourceLabel: source === "local" ? "本地已安装" : "云端预留",
  };
}
