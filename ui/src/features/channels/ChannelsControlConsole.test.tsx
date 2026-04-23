import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { ChannelRecord } from "@/features/workspace/useWorkspaceOverview";

import { ChannelsControlConsole } from "./ChannelsControlConsole";
import { useChannelControl } from "./useChannelControl";

vi.mock("./useChannelControl", () => ({
  useChannelControl: vi.fn(),
}));

const useChannelControlMock = vi.mocked(useChannelControl);

const selectedChannel: ChannelRecord = {
  configured: true,
  enabled: true,
  healthy: true,
  name: "微信",
  note: "优先承接私域服务",
  running: true,
  slug: "wechat",
  status: "运行中",
  summary: "私域服务与通知入口",
};

describe("ChannelsControlConsole", () => {
  it("renders an empty state when no channel is selected", () => {
    useChannelControlMock.mockReturnValue({
      adapterStatusesError: null,
      isLoading: false,
      mentionGate: null,
      mentionGateError: null,
      pairDevice: vi.fn(),
      pairDeviceError: null,
      pairDevicePending: false,
      pairingEnabled: false,
      pairingError: null,
      presenceError: null,
      selectedAdapterStatus: null,
      selectedContacts: [],
      selectedPairings: [],
      selectedPresence: [],
      toggleMentionGate: vi.fn(),
      toggleMentionGateError: null,
      toggleMentionGatePending: false,
      unpairDevice: vi.fn(),
      unpairDeviceError: null,
      unpairDevicePending: false,
    } as never);

    render(<ChannelsControlConsole selectedChannel={null} />);

    expect(screen.getByText("等待选择渠道")).toBeInTheDocument();
  });

  it("renders adapter, pairing and mention-gate details for the selected channel", async () => {
    const pairDevice = vi.fn().mockResolvedValue({ status: "ok" });
    const toggleMentionGate = vi.fn().mockResolvedValue({ enabled: false });
    const unpairDevice = vi.fn().mockResolvedValue({ status: "ok" });

    useChannelControlMock.mockReturnValue({
      adapterStatusesError: null,
      isLoading: false,
      mentionGate: { enabled: true },
      mentionGateError: null,
      pairDevice,
      pairDeviceError: null,
      pairDevicePending: false,
      pairingEnabled: true,
      pairingError: null,
      presenceError: null,
      selectedAdapterStatus: {
        enabled: true,
        healthy: true,
        last_activity: "2026-04-22T10:00:00Z",
        last_error: "",
        name: "wechat",
        running: true,
      },
      selectedContacts: [
        {
          added_at: "2026-04-22T09:00:00Z",
          channel: "wechat",
          display_name: "Alice",
          first_name: "Alice",
          is_bot: false,
          last_name: "",
          last_seen: "2026-04-22T10:00:00Z",
          user_id: "alice",
          username: "alice",
        },
      ],
      selectedPairings: [
        {
          channel: "wechat",
          device_id: "device-1",
          display_name: "Alice Phone",
          expires_at: "2026-04-23T10:00:00Z",
          last_seen: "2026-04-22T10:00:00Z",
          paired_at: "2026-04-22T09:00:00Z",
          user_id: "alice",
        },
      ],
      selectedPresence: [
        {
          activity: "typing",
          channel: "wechat",
          key: "wechat:alice",
          last_update: "2026-04-22T10:00:00Z",
          since: "2026-04-22T09:55:00Z",
          status: "online",
          user_id: "alice",
        },
      ],
      toggleMentionGate,
      toggleMentionGateError: null,
      toggleMentionGatePending: false,
      unpairDevice,
      unpairDeviceError: null,
      unpairDevicePending: false,
    } as never);

    render(<ChannelsControlConsole selectedChannel={selectedChannel} />);

    expect(screen.getByText("实时适配器状态")).toBeInTheDocument();
    expect(screen.getByText("设备配对")).toBeInTheDocument();
    expect(screen.getByText("提及门")).toBeInTheDocument();
    expect(screen.getByText("联系人")).toBeInTheDocument();
    expect(screen.getByText("Alice Phone")).toBeInTheDocument();
    expect(screen.getByText("Alice")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "关闭提及门" }));

    await waitFor(() => {
      expect(toggleMentionGate).toHaveBeenCalledWith(false);
    });
  });

  it("falls back to the selected channel snapshot when live adapter status is unavailable", () => {
    useChannelControlMock.mockReturnValue({
      adapterStatusesError: null,
      isLoading: false,
      mentionGate: { enabled: false },
      mentionGateError: null,
      pairDevice: vi.fn(),
      pairDeviceError: null,
      pairDevicePending: false,
      pairingEnabled: false,
      pairingError: null,
      presenceError: null,
      selectedAdapterStatus: null,
      selectedContacts: [],
      selectedPairings: [],
      selectedPresence: [],
      toggleMentionGate: vi.fn(),
      toggleMentionGateError: null,
      toggleMentionGatePending: false,
      unpairDevice: vi.fn(),
      unpairDeviceError: null,
      unpairDevicePending: false,
    } as never);

    render(<ChannelsControlConsole selectedChannel={selectedChannel} />);

    expect(screen.getByText("运行中")).toBeInTheDocument();
    expect(screen.getByText("健康")).toBeInTheDocument();
  });

  it("blocks pairing submission when required identifiers are blank after trimming", async () => {
    const pairDevice = vi.fn().mockResolvedValue({ status: "ok" });

    useChannelControlMock.mockReturnValue({
      adapterStatusesError: null,
      isLoading: false,
      mentionGate: { enabled: true },
      mentionGateError: null,
      pairDevice,
      pairDeviceError: null,
      pairDevicePending: false,
      pairingEnabled: true,
      pairingError: null,
      presenceError: null,
      selectedAdapterStatus: null,
      selectedContacts: [],
      selectedPairings: [],
      selectedPresence: [],
      toggleMentionGate: vi.fn(),
      toggleMentionGateError: null,
      toggleMentionGatePending: false,
      unpairDevice: vi.fn(),
      unpairDeviceError: null,
      unpairDevicePending: false,
    } as never);

    render(<ChannelsControlConsole selectedChannel={selectedChannel} />);

    const inputs = screen.getAllByRole("textbox");
    fireEvent.change(inputs[0], { target: { value: "   " } });
    fireEvent.change(inputs[1], { target: { value: "   " } });

    fireEvent.click(screen.getByRole("button", { name: "新增配对" }));

    expect(pairDevice).not.toHaveBeenCalled();
    expect(screen.getByText("用户 ID 和设备 ID 不能为空")).toBeInTheDocument();
  });
});
