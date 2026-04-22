import { Activity, AtSign, Users, Waypoints } from "lucide-react";
import { useEffect, useState } from "react";
import type { FormEvent } from "react";

import { BackendDetailSection } from "@/features/backend-ui/BackendDetailSection";
import { BackendEmptyState } from "@/features/backend-ui/BackendEmptyState";
import { BackendPropertyList } from "@/features/backend-ui/BackendPropertyList";
import { BackendSummaryStrip } from "@/features/backend-ui/BackendSummaryStrip";
import { StatusBadge, type StatusBadgeTone } from "@/features/backend-ui/StatusBadge";
import { getStatusTone } from "@/features/backend-ui/getStatusTone";
import { useChannelControl } from "@/features/channels/useChannelControl";
import type { ChannelRecord } from "@/features/workspace/useWorkspaceOverview";

type ActionNotice = {
  message: string;
  tone: "error" | "success";
};

type PairFormState = {
  deviceId: string;
  displayName: string;
  ttlHours: string;
  userId: string;
};

const defaultPairForm: PairFormState = {
  deviceId: "",
  displayName: "",
  ttlHours: "24",
  userId: "",
};

const inputClassName =
  "h-11 w-full rounded-[14px] border border-skin bg-white px-3 text-sm text-ink outline-none placeholder:text-[#98a2b3]";

function formatDateTime(value?: string) {
  if (!value) return "未记录";

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;

  return date.toLocaleString("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  });
}

function getErrorMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  return "请求失败";
}

function getAdapterRuntimeLabel(
  channel: ChannelRecord,
  liveEnabled: boolean,
  liveRunning: boolean,
  liveHealthy: boolean,
) {
  if (liveRunning && liveHealthy) return "运行中";
  if (liveEnabled && !liveHealthy) return "已启用，待恢复";
  if (liveEnabled) return "已启用";
  if (channel.configured) return "已配置，待启用";
  return "规划中";
}

function getAdapterRuntimeTone(
  channel: ChannelRecord,
  liveEnabled: boolean,
  liveRunning: boolean,
  liveHealthy: boolean,
): StatusBadgeTone {
  if (liveRunning && liveHealthy) return "success";
  if (liveEnabled && !liveHealthy) return "warning";
  if (liveEnabled || channel.configured) return "info";
  return "default";
}

type ChannelsControlConsoleProps = {
  selectedChannel: ChannelRecord | null;
};

export function ChannelsControlConsole({ selectedChannel }: ChannelsControlConsoleProps) {
  const {
    adapterStatusesError,
    mentionGate,
    mentionGateError,
    pairDevice,
    pairDevicePending,
    pairingEnabled,
    pairingError,
    presenceError,
    selectedAdapterStatus,
    selectedContacts,
    selectedPairings,
    selectedPresence,
    toggleMentionGate,
    toggleMentionGatePending,
    unpairDevice,
    unpairDevicePending,
  } = useChannelControl(selectedChannel?.slug ?? null);
  const [pairForm, setPairForm] = useState<PairFormState>(defaultPairForm);
  const [actionNotice, setActionNotice] = useState<ActionNotice | null>(null);

  useEffect(() => {
    setPairForm(defaultPairForm);
    setActionNotice(null);
  }, [selectedChannel?.slug]);

  if (!selectedChannel) {
    return <BackendEmptyState icon={Waypoints} title="等待选择渠道" />;
  }

  const channelName = selectedChannel.name;
  const fallbackEnabled = selectedChannel.enabled;
  const fallbackRunning = selectedChannel.running;
  const fallbackHealthy = selectedChannel.healthy;
  const effectiveEnabled = selectedAdapterStatus?.enabled ?? fallbackEnabled;
  const effectiveRunning = selectedAdapterStatus?.running ?? fallbackRunning;
  const effectiveHealthy = selectedAdapterStatus?.healthy ?? fallbackHealthy;
  const runtimeLabel = getAdapterRuntimeLabel(
    selectedChannel,
    effectiveEnabled,
    effectiveRunning,
    effectiveHealthy,
  );
  const runtimeTone = getAdapterRuntimeTone(
    selectedChannel,
    effectiveEnabled,
    effectiveRunning,
    effectiveHealthy,
  );
  const sortedPresence = [...selectedPresence].sort((left, right) => left.user_id.localeCompare(right.user_id));
  const sortedPairings = [...selectedPairings].sort((left, right) => right.last_seen.localeCompare(left.last_seen));
  const sortedContacts = [...selectedContacts].sort((left, right) =>
    (left.display_name || left.username || left.user_id).localeCompare(
      right.display_name || right.username || right.user_id,
    ),
  );

  async function handleMentionGateToggle() {
    if (!mentionGate) return;

    try {
      await toggleMentionGate(!mentionGate.enabled);
      setActionNotice({
        message: mentionGate.enabled ? "提及门已关闭" : "提及门已开启",
        tone: "success",
      });
    } catch (error) {
      setActionNotice({ message: getErrorMessage(error), tone: "error" });
    }
  }

  async function handlePairSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const ttlHours = Number(pairForm.ttlHours || "24");
    const normalizedTtlHours = Number.isFinite(ttlHours) ? ttlHours : 24;
    const userId = pairForm.userId.trim();
    const deviceId = pairForm.deviceId.trim();
    const displayName = pairForm.displayName.trim();

    if (userId === "" || deviceId === "") {
      setActionNotice({ message: "用户 ID 和设备 ID 不能为空", tone: "error" });
      return;
    }

    try {
      await pairDevice({
        device_id: deviceId,
        display_name: displayName,
        ttl_seconds: Math.max(1, normalizedTtlHours) * 3600,
        user_id: userId,
      });
      setPairForm(defaultPairForm);
      setActionNotice({ message: `${channelName} 新增配对成功`, tone: "success" });
    } catch (error) {
      setActionNotice({ message: getErrorMessage(error), tone: "error" });
    }
  }

  async function handleUnpair(userId: string, deviceId: string, channel: string) {
    try {
      await unpairDevice({ channel, device_id: deviceId, user_id: userId });
      setActionNotice({ message: "设备配对已移除", tone: "success" });
    } catch (error) {
      setActionNotice({ message: getErrorMessage(error), tone: "error" });
    }
  }

  return (
    <div className="space-y-5">
      {actionNotice ? (
        <div
          className={[
            "rounded-[18px] border px-4 py-3 text-sm",
            actionNotice.tone === "success"
              ? "border-[#d7eada] bg-[#f3fbf4] text-[#3f7a59]"
              : "border-[#f0ded6] bg-[#fff7f3] text-[#8a6135]",
          ].join(" ")}
        >
          {actionNotice.message}
        </div>
      ) : null}

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <div className="space-y-5">
          <BackendDetailSection title="实时适配器状态">
            <BackendSummaryStrip
              items={[
                {
                  active: Boolean(selectedAdapterStatus?.running),
                  label: "运行状态",
                  value: (
                    <span className="inline-flex items-center gap-2">
                      <StatusBadge label={runtimeLabel} tone={runtimeTone} />
                    </span>
                  ),
                },
                {
                  label: "健康状态",
                  value: effectiveHealthy ? "健康" : effectiveEnabled ? "待恢复" : selectedChannel.configured ? "待启用" : "未接入",
                },
                {
                  label: "最近活动",
                  value: formatDateTime(selectedAdapterStatus?.last_activity),
                },
                {
                  label: "最近错误",
                  value: selectedAdapterStatus?.last_error || "无",
                },
              ]}
            />

            <div className="mt-4">
              <BackendPropertyList
                items={[
                  { label: "当前渠道", value: channelName },
                  { label: "渠道标识", value: selectedChannel.slug },
                  { label: "适配器名称", value: selectedAdapterStatus?.name || "未发现" },
                  { label: "数据来源", value: selectedAdapterStatus ? "/channels" : "规划数据" },
                ]}
              />
            </div>

            {adapterStatusesError ? (
              <div className="mt-4 rounded-[16px] border border-[#f0ded6] bg-[#fff7f3] px-4 py-3 text-sm text-[#8a6135]">
                实时状态接口暂不可用：{getErrorMessage(adapterStatusesError)}
              </div>
            ) : null}
          </BackendDetailSection>

          <BackendDetailSection title="设备配对">
            <BackendSummaryStrip
              items={[
                {
                  active: pairingEnabled,
                  label: "配对服务",
                  value: pairingEnabled ? "已开启" : "未开启",
                },
                { label: "当前渠道配对", value: `${sortedPairings.length} 个` },
                { label: "在线状态", value: `${sortedPresence.length} 个` },
              ]}
            />

            <form className="mt-4 grid gap-3 md:grid-cols-2" onSubmit={handlePairSubmit}>
              <input
                className={inputClassName}
                placeholder="用户 ID"
                required
                value={pairForm.userId}
                onChange={(event) => setPairForm((current) => ({ ...current, userId: event.target.value }))}
              />
              <input
                className={inputClassName}
                placeholder="设备 ID"
                required
                value={pairForm.deviceId}
                onChange={(event) => setPairForm((current) => ({ ...current, deviceId: event.target.value }))}
              />
              <input
                className={inputClassName}
                placeholder="显示名称"
                value={pairForm.displayName}
                onChange={(event) => setPairForm((current) => ({ ...current, displayName: event.target.value }))}
              />
              <input
                className={inputClassName}
                min="1"
                placeholder="有效期（小时）"
                required
                type="number"
                value={pairForm.ttlHours}
                onChange={(event) => setPairForm((current) => ({ ...current, ttlHours: event.target.value }))}
              />
              <div className="flex flex-wrap gap-3 md:col-span-2">
                <button
                  className="shell-button h-11 justify-center px-4 text-sm font-medium disabled:opacity-60"
                  disabled={pairDevicePending || !pairingEnabled}
                  type="submit"
                >
                  {pairDevicePending ? "正在新增配对..." : "新增配对"}
                </button>
                {!pairingEnabled ? (
                  <div className="rounded-[14px] bg-[#f8fafc] px-4 py-3 text-sm text-mute">
                    当前网关的配对服务未开启。
                  </div>
                ) : null}
              </div>
            </form>

            {pairingError ? (
              <div className="mt-4 rounded-[16px] border border-[#f0ded6] bg-[#fff7f3] px-4 py-3 text-sm text-[#8a6135]">
                配对接口暂不可用：{getErrorMessage(pairingError)}
              </div>
            ) : null}

            <div className="mt-4 overflow-hidden rounded-[18px] border border-skin bg-[#fbfcfe]">
              {sortedPairings.length > 0 ? (
                sortedPairings.map((record) => (
                  <div
                    key={`${record.channel}:${record.user_id}:${record.device_id}`}
                    className="flex flex-col gap-3 border-b border-skin px-4 py-4 last:border-b-0 md:flex-row md:items-center md:justify-between"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <div className="text-sm font-semibold text-ink">{record.display_name || record.device_id}</div>
                        <StatusBadge label={record.channel} tone="info" />
                      </div>
                      <div className="mt-1 text-sm text-[#607699]">
                        {record.user_id} / {record.device_id}
                      </div>
                      <div className="mt-2 text-xs text-mute">
                        最近活跃：{formatDateTime(record.last_seen)} / 过期：{formatDateTime(record.expires_at)}
                      </div>
                    </div>
                    <button
                      className="rounded-[10px] bg-[#f5f7fb] px-3 py-2 text-sm font-medium text-[#5b6f8b] transition-colors duration-150 hover:bg-[#edf2fa] hover:text-ink disabled:opacity-60"
                      disabled={unpairDevicePending}
                      onClick={() => handleUnpair(record.user_id, record.device_id, record.channel)}
                      type="button"
                    >
                      移除配对
                    </button>
                  </div>
                ))
              ) : (
                <div className="px-4 py-5 text-sm text-mute">当前渠道还没有已配对设备。</div>
              )}
            </div>
          </BackendDetailSection>
        </div>

        <div className="space-y-5">
          <BackendDetailSection title="提及门">
            <BackendSummaryStrip
              items={[
                {
                  active: mentionGate?.enabled,
                  label: "当前状态",
                  value: mentionGate ? (mentionGate.enabled ? "已开启" : "已关闭") : "未知",
                },
                { label: "作用范围", value: "全局渠道入口" },
              ]}
            />

            <div className="mt-4">
              <BackendPropertyList
                items={[
                  { label: "当前查看", value: channelName },
                  { label: "开关来源", value: "/channel/mention-gate" },
                ]}
              />
            </div>

            {mentionGateError ? (
              <div className="mt-4 rounded-[16px] border border-[#f0ded6] bg-[#fff7f3] px-4 py-3 text-sm text-[#8a6135]">
                提及门接口暂不可用：{getErrorMessage(mentionGateError)}
              </div>
            ) : null}

            <div className="mt-4 flex flex-wrap gap-3">
              <button
                className="shell-button h-11 justify-center px-4 text-sm font-medium disabled:opacity-60"
                disabled={!mentionGate || toggleMentionGatePending}
                onClick={handleMentionGateToggle}
                type="button"
              >
                {toggleMentionGatePending ? "正在更新..." : mentionGate?.enabled ? "关闭提及门" : "开启提及门"}
              </button>
            </div>
          </BackendDetailSection>

          <BackendDetailSection title="在线状态">
            {presenceError ? (
              <div className="rounded-[16px] border border-[#f0ded6] bg-[#fff7f3] px-4 py-3 text-sm text-[#8a6135]">
                在线状态接口暂不可用：{getErrorMessage(presenceError)}
              </div>
            ) : sortedPresence.length > 0 ? (
              <div className="space-y-3">
                {sortedPresence.map((record) => (
                  <div
                    key={record.key}
                    className="flex items-start gap-3 rounded-[16px] border border-skin bg-[#fbfcfe] px-4 py-3"
                  >
                    <span className="mt-0.5 flex h-9 w-9 items-center justify-center rounded-[12px] bg-[#f3f6fb] text-[#607699]">
                      <Activity size={16} strokeWidth={2.1} />
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <div className="text-sm font-semibold text-ink">{record.user_id}</div>
                        <StatusBadge label={record.status} tone={getStatusTone(record.status)} />
                      </div>
                      <div className="mt-1 text-xs text-mute">更新于 {formatDateTime(record.last_update)}</div>
                      {record.activity ? <div className="mt-1 text-xs text-[#607699]">{record.activity}</div> : null}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="rounded-[16px] border border-skin bg-[#fbfcfe] px-4 py-4 text-sm text-mute">
                当前渠道还没有在线状态记录。
              </div>
            )}
          </BackendDetailSection>

          <BackendDetailSection title="联系人">
            <BackendSummaryStrip
              items={[
                { label: "已加载", value: `${sortedContacts.length} 个` },
                { label: "来源接口", value: "/channel/contacts" },
              ]}
            />

            <div className="mt-4">
              {sortedContacts.length > 0 ? (
                <div className="space-y-3">
                  {sortedContacts.slice(0, 8).map((contact) => (
                    <div
                      key={`${contact.channel}:${contact.user_id}`}
                      className="flex items-start gap-3 rounded-[16px] border border-skin bg-[#fbfcfe] px-4 py-3"
                    >
                      <span className="mt-0.5 flex h-9 w-9 items-center justify-center rounded-[12px] bg-[#f3f6fb] text-[#607699]">
                        {contact.username ? <AtSign size={16} strokeWidth={2.1} /> : <Users size={16} strokeWidth={2.1} />}
                      </span>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold text-ink">
                          {contact.display_name || contact.username || contact.user_id}
                        </div>
                        <div className="mt-1 text-xs text-mute">
                          {contact.username ? `@${contact.username}` : contact.user_id}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="rounded-[16px] border border-skin bg-[#fbfcfe] px-4 py-4 text-sm text-mute">
                  当前渠道还没有联系人记录。
                </div>
              )}
            </div>
          </BackendDetailSection>
        </div>
      </div>
    </div>
  );
}
