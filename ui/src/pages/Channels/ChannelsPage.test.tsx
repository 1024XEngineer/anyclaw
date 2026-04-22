import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

import { ChannelsPage } from "./ChannelsPage";

vi.mock("@/features/workspace/useWorkspaceOverview", () => ({
  useWorkspaceOverview: vi.fn(),
}));

vi.mock("@/features/channels/ChannelsControlConsole", () => ({
  ChannelsControlConsole: ({ selectedChannel }: { selectedChannel: { slug?: string } | null }) => (
    <div data-testid="channels-control-console">{selectedChannel?.slug ?? "none"}</div>
  ),
}));

const useWorkspaceOverviewMock = vi.mocked(useWorkspaceOverview);

describe("ChannelsPage", () => {
  beforeEach(() => {
    useWorkspaceOverviewMock.mockReturnValue({
      data: {
        channelSettings: [
          { hint: "默认拒绝私信", label: "安全策略", value: "提及门已开启" },
          { hint: "当前优先渠道数", label: "优先接入", value: "微信、飞书" },
        ],
        extensionAdapters: ["wechat-adapter", "discord-adapter"],
        meta: {
          liveConnected: true,
          sourceLabel: "网关叠加",
        },
        priorityChannels: [
          {
            configured: true,
            enabled: true,
            healthy: true,
            name: "微信",
            note: "优先承接私域服务",
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
            note: "补充型渠道位",
            running: false,
            slug: "signal",
            status: "未接入",
            summary: "适合补充型渠道接入",
          },
        ],
      },
    } as never);
  });

  it("renders channels overview and control console", () => {
    render(
      <MemoryRouter initialEntries={["/channels"]}>
        <ChannelsPage />
      </MemoryRouter>,
    );

    expect(screen.getByRole("heading", { name: "渠道" })).toBeInTheDocument();
    expect(screen.getByText("优先渠道表")).toBeInTheDocument();
    expect(screen.getAllByText("微信").length).toBeGreaterThan(0);
    expect(screen.getByTestId("channels-control-console")).toHaveTextContent("wechat");
  });

  it("updates the selected channel from the table action", () => {
    render(
      <MemoryRouter initialEntries={["/channels"]}>
        <ChannelsPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getAllByRole("button", { name: "规划接入" })[1]);

    expect(screen.getByTestId("channels-control-console")).toHaveTextContent("signal");
  });

  it("keeps the planned summary aligned with the planned filter logic", () => {
    render(
      <MemoryRouter initialEntries={["/channels"]}>
        <ChannelsPage />
      </MemoryRouter>,
    );

    fireEvent.click(screen.getAllByRole("button", { name: "待接入" })[0]);

    expect(screen.getByTestId("channels-control-console")).toHaveTextContent("signal");
  });
});
