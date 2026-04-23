import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import type { ChannelRecord } from "@/features/workspace/useWorkspaceOverview";

import { useChannelsConsole } from "./useChannelsConsole";

const channels: ChannelRecord[] = [
  {
    configured: true,
    enabled: true,
    healthy: true,
    name: "微信",
    note: "私域入口",
    running: true,
    slug: "wechat",
    status: "运行中",
    summary: "适合作为核心渠道入口",
  },
  {
    configured: false,
    enabled: false,
    healthy: false,
    name: "Signal",
    note: "待规划",
    running: false,
    slug: "signal",
    status: "未接入",
    summary: "补充型渠道位",
  },
];

function createWrapper(initialEntries: string[]) {
  return function Wrapper({ children }: PropsWithChildren) {
    return <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>;
  };
}

describe("useChannelsConsole", () => {
  it("defaults to the first visible channel", () => {
    const { result } = renderHook(() => useChannelsConsole(channels), {
      wrapper: createWrapper(["/channels"]),
    });

    expect(result.current.filter).toBe("all");
    expect(result.current.selectedSlug).toBe("wechat");
    expect(result.current.filteredChannels.map((channel) => channel.slug)).toEqual(["wechat", "signal"]);
  });

  it("filters by enabled channels and resets selection", () => {
    const { result } = renderHook(() => useChannelsConsole(channels), {
      wrapper: createWrapper(["/channels?selected=signal"]),
    });

    expect(result.current.selectedSlug).toBe("signal");

    act(() => {
      result.current.setFilter("enabled");
    });

    expect(result.current.filteredChannels.map((channel) => channel.slug)).toEqual(["wechat"]);
    expect(result.current.selectedSlug).toBe("wechat");
  });

  it("filters channels by the search query", () => {
    const { result } = renderHook(() => useChannelsConsole(channels), {
      wrapper: createWrapper(["/channels"]),
    });

    act(() => {
      result.current.setQuery("signal");
    });

    expect(result.current.filteredChannels.map((channel) => channel.slug)).toEqual(["signal"]);
    expect(result.current.selectedSlug).toBe("signal");
  });
});
