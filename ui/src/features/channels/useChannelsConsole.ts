import { useMemo } from "react";
import { useSearchParams } from "react-router-dom";

import type { ChannelRecord } from "@/features/workspace/useWorkspaceOverview";

export type ChannelsFilter = "all" | "enabled" | "planned";

function normalizeFilter(value: string | null): ChannelsFilter {
  if (value === "enabled") return "enabled";
  if (value === "planned") return "planned";
  return "all";
}

function matchesQuery(query: string, fields: string[]) {
  if (query === "") return true;
  return fields.join(" ").toLowerCase().includes(query.toLowerCase());
}

function matchesFilter(filter: ChannelsFilter, channel: ChannelRecord) {
  const enabled = channel.enabled || channel.running;

  if (filter === "enabled") return enabled;
  if (filter === "planned") return !enabled;
  return true;
}

export function useChannelsConsole(channels: ChannelRecord[]) {
  const [searchParams, setSearchParams] = useSearchParams();

  const filter = normalizeFilter(searchParams.get("status"));
  const query = (searchParams.get("q") ?? "").trim();

  const filteredChannels = useMemo(
    () =>
      channels.filter(
        (channel) =>
          matchesFilter(filter, channel) &&
          matchesQuery(query, [channel.name, channel.slug, channel.summary, channel.note, channel.status]),
      ),
    [channels, filter, query],
  );

  const selectedParam = searchParams.get("selected");
  const selectedChannel =
    filteredChannels.find((channel) => channel.slug === selectedParam) ?? filteredChannels[0] ?? null;

  function patchParams(
    patch: Partial<{
      q: string;
      selected: string | null;
      status: ChannelsFilter;
    }>,
  ) {
    const next = new URLSearchParams(searchParams);

    if (patch.status !== undefined) {
      if (patch.status === "all") next.delete("status");
      else next.set("status", patch.status);
      next.delete("selected");
    }

    if (patch.q !== undefined) {
      const value = patch.q.trim();
      if (value === "") next.delete("q");
      else next.set("q", value);
      next.delete("selected");
    }

    if (patch.selected !== undefined) {
      if (patch.selected) next.set("selected", patch.selected);
      else next.delete("selected");
    }

    setSearchParams(next, { replace: true });
  }

  return {
    filter,
    filteredChannels,
    query,
    selectedChannel,
    selectedSlug: selectedChannel?.slug ?? null,
    setFilter: (nextFilter: ChannelsFilter) => patchParams({ status: nextFilter }),
    setQuery: (nextQuery: string) => patchParams({ q: nextQuery }),
    setSelected: (nextSelected: string | null) => patchParams({ selected: nextSelected }),
  };
}
