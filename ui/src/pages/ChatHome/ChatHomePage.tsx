import { Check, CircleUserRound, Clock3, Gauge, LoaderCircle, ShieldAlert, Sparkles, X } from "lucide-react";
import { useEffect, useRef } from "react";
import { NavLink } from "react-router-dom";

import { MarkdownMessage } from "@/features/chat/MarkdownMessage";
import { type ChatApproval, useWebChat } from "@/features/chat/useWebChat";
import { Composer } from "@/features/composer/Composer";
import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

const tabs = [
  { label: "对话", to: "/" },
  { label: "工作室", to: "/studio" },
];

function formatMessageTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function ThinkingIndicator({ activeAgentLabel }: { activeAgentLabel: string }) {
  return (
    <div className="max-w-[680px] pt-1">
      <div className="flex flex-wrap items-center gap-2 text-[12px] text-[#98a2b3]">
        <span className="font-medium text-[#4b5563]">{activeAgentLabel}</span>
        <span className="inline-flex items-center gap-2 text-[#667085]">
          <Sparkles size={14} strokeWidth={2.1} />
          <span>正在思考</span>
          <span aria-hidden="true" className="thinking-dots">
            <span className="thinking-dot" />
            <span className="thinking-dot" />
            <span className="thinking-dot" />
          </span>
        </span>
      </div>

      <div aria-hidden="true" className="mt-4 space-y-2.5">
        <div className="thinking-line w-[24%]" />
        <div className="thinking-line w-[54%]" />
        <div className="thinking-line w-[34%]" />
      </div>
    </div>
  );
}

const approvalFieldLabels: Record<string, string> = {
  command: "命令",
  message: "请求",
  path: "路径",
  target: "目标",
  text: "内容",
  title: "任务",
  url: "地址",
  workspace: "工作区",
};

function formatApprovalTime(value?: string) {
  if (!value) return "";

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function summarizeApprovalValue(value: unknown): string | null {
  if (typeof value === "string") {
    const normalized = value.replace(/\s+/g, " ").trim();
    return normalized === "" ? null : normalized;
  }

  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }

  if (Array.isArray(value)) {
    const compact = value
      .map((item) => summarizeApprovalValue(item))
      .filter((item): item is string => Boolean(item))
      .slice(0, 3);

    return compact.length > 0 ? compact.join(", ") : null;
  }

  return null;
}

function buildApprovalDetails(approval: ChatApproval) {
  const payload = approval.payload && typeof approval.payload === "object" ? approval.payload : undefined;
  const source =
    payload?.args && typeof payload.args === "object"
      ? (payload.args as Record<string, unknown>)
      : payload;

  if (!source) return [];

  const preferredKeys = ["command", "path", "message", "title", "target", "url", "text", "workspace"];
  const preferredLines = preferredKeys
    .map((key) => {
      const value = summarizeApprovalValue(source[key]);
      if (!value) return null;
      return `${approvalFieldLabels[key] ?? key}: ${value}`;
    })
    .filter((line): line is string => line !== null);

  if (preferredLines.length > 0) {
    return preferredLines.slice(0, 2);
  }

  const fallbackEntry = Object.entries(source).find(([, value]) => summarizeApprovalValue(value));
  if (!fallbackEntry) return [];

  const [key, value] = fallbackEntry;
  const summary = summarizeApprovalValue(value);
  return summary ? [`${approvalFieldLabels[key] ?? key}: ${summary}`] : [];
}

function summarizeApprovalDetails(approval: ChatApproval) {
  const payload = approval.payload && typeof approval.payload === "object" ? approval.payload : undefined;
  const source =
    payload?.args && typeof payload.args === "object"
      ? (payload.args as Record<string, unknown>)
      : payload;

  if (!source) return [];

  const preferredKeys = ["command", "path", "message", "title", "target", "url", "text", "workspace"];
  const preferredLines = preferredKeys
    .map((key) => {
      const value = summarizeApprovalValue(source[key]);
      if (!value) return null;
      return `${approvalFieldLabels[key] ?? key}：${value}`;
    })
    .filter((line): line is string => line !== null);

  if (preferredLines.length > 0) {
    return preferredLines.slice(0, 2);
  }

  const fallbackEntry = Object.entries(source).find(([, value]) => summarizeApprovalValue(value));
  if (!fallbackEntry) return [];

  const [key, value] = fallbackEntry;
  const summary = summarizeApprovalValue(value);
  return summary ? [`${approvalFieldLabels[key] ?? key}：${summary}`] : [];
}

function ApprovalNotice({
  approvalActionId,
  approvals,
  onResolve,
}: {
  approvalActionId: string | null;
  approvals: ChatApproval[];
  onResolve: (approvalId: string, approved: boolean) => Promise<void>;
}) {
  if (approvals.length === 0) return null;

  const actionBusy = approvalActionId !== null;

  return (
    <div className="shrink-0 px-5 pb-3 pt-1 sm:px-6 lg:px-8">
      <div className="mx-auto w-full max-w-[980px] rounded-[24px] border border-[#e7ebf0] bg-white px-4 py-3 shadow-[0_10px_24px_rgba(15,23,42,0.04)]">
        <div className="flex items-center gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[#f5f7fa] text-[#344054]">
            <ShieldAlert size={17} strokeWidth={2.1} />
          </span>
          <div className="min-w-0">
            <div className="text-[14px] font-medium text-[#1f2937]">需要确认权限后才能继续</div>
            <div className="text-[12px] text-[#667085]">允许后会自动继续执行，拒绝后当前请求会停止。</div>
          </div>
        </div>

        <div className="mt-3 divide-y divide-[rgba(15,23,42,0.06)]">
          {approvals.map((approval) => {
            const details = buildApprovalDetails(approval);
            const isCurrentAction = approvalActionId === approval.id;

            return (
              <div className="flex flex-col gap-3 py-3 first:pt-0 last:pb-0 md:flex-row md:items-start md:justify-between" key={approval.id}>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2 text-[13px]">
                    <span className="font-medium text-[#1f2937]">{approval.tool_name}</span>
                    {approval.requested_at ? <span className="text-[#98a2b3]">{formatApprovalTime(approval.requested_at)}</span> : null}
                  </div>

                  {details.length > 0 ? (
                    <div className="mt-1.5 space-y-1 text-[13px] leading-6 text-[#667085]">
                      {details.map((detail) => (
                        <div className="truncate" key={detail} title={detail}>
                          {detail}
                        </div>
                      ))}
                    </div>
                  ) : null}
                </div>

                <div className="flex items-center gap-2 md:shrink-0">
                  <button
                    className="inline-flex h-9 items-center justify-center gap-1.5 rounded-full border border-[#dbe1e8] px-4 text-[13px] font-medium text-[#475467] transition-colors duration-150 hover:border-[#cfd6de] hover:text-[#1f2937] disabled:cursor-not-allowed disabled:opacity-60"
                    disabled={actionBusy}
                    onClick={() => void onResolve(approval.id, false)}
                    type="button"
                  >
                    {isCurrentAction ? <LoaderCircle className="animate-spin" size={14} strokeWidth={2.1} /> : <X size={14} strokeWidth={2.1} />}
                    <span>拒绝</span>
                  </button>

                  <button
                    className="inline-flex h-9 items-center justify-center gap-1.5 rounded-full bg-[#1f2430] px-4 text-[13px] font-medium text-white transition-colors duration-150 hover:bg-[#111827] disabled:cursor-not-allowed disabled:bg-[#c7ced8]"
                    disabled={actionBusy}
                    onClick={() => void onResolve(approval.id, true)}
                    type="button"
                  >
                    {isCurrentAction ? <LoaderCircle className="animate-spin" size={14} strokeWidth={2.1} /> : <Check size={14} strokeWidth={2.1} />}
                    <span>允许</span>
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export function ChatHomePage() {
  const { data } = useWorkspaceOverview();
  const openSettings = useShellStore((state) => state.openSettings);
  const activeAgent = data.localAgents.find((agent) => agent.active) ?? data.localAgents[0] ?? null;
  const defaultProvider = data.providers.find((provider) => provider.isDefault) ?? data.providers[0] ?? null;
  const activeAgentName = activeAgent?.name ?? data.runtimeProfile.name ?? null;
  const activeAgentLabel = activeAgentName || data.runtimeProfile.name || "AnyClaw";
  const modelLabel = defaultProvider?.model || data.runtimeProfile.model || "默认大模型";
  const {
    approvalActionId,
    draft,
    error,
    isSending,
    messages,
    pendingApprovals,
    resetConversation,
    resolveApproval,
    sessionId,
    sendMessage,
    setDraft,
  } = useWebChat(activeAgentName, data.runtimeProfile.workspace);
  const messagesViewportRef = useRef<HTMLDivElement | null>(null);
  const scrollFadeTimerRef = useRef<number | null>(null);

  useEffect(() => {
    const viewport = messagesViewportRef.current;
    if (!viewport) return;

    const markScrolling = () => {
      viewport.dataset.scrolling = "true";

      if (scrollFadeTimerRef.current !== null) {
        window.clearTimeout(scrollFadeTimerRef.current);
      }

      scrollFadeTimerRef.current = window.setTimeout(() => {
        delete viewport.dataset.scrolling;
        scrollFadeTimerRef.current = null;
      }, 720);
    };

    viewport.addEventListener("scroll", markScrolling, { passive: true });

    return () => {
      viewport.removeEventListener("scroll", markScrolling);

      if (scrollFadeTimerRef.current !== null) {
        window.clearTimeout(scrollFadeTimerRef.current);
        scrollFadeTimerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    const viewport = messagesViewportRef.current;
    if (!viewport) return;

    viewport.scrollTo({
      behavior: "smooth",
      top: viewport.scrollHeight,
    });
  }, [isSending, messages]);

  return (
    <div className="relative z-10 flex h-full min-h-0 flex-1 flex-col overflow-hidden bg-white lg:absolute lg:inset-0">
      <header className="shrink-0 px-5 pb-1 pt-4 sm:px-6 lg:px-8 lg:pt-5">
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-center gap-3">
            <div className="inline-flex rounded-full bg-[#f5f6f8] p-1">
              <nav className="flex items-center gap-1">
                {tabs.map((tab) => (
                  <NavLink
                    key={tab.label}
                    className={({ isActive }) =>
                      [
                        "rounded-full px-5 py-2.5 text-[15px] font-medium transition-colors duration-150",
                        isActive ? "bg-white text-[#111827] shadow-[0_2px_10px_rgba(15,23,42,0.05)]" : "text-[#667085] hover:text-[#1d1f25]",
                      ].join(" ")
                    }
                    end={tab.to === "/"}
                    to={tab.to}
                  >
                    {tab.label}
                  </NavLink>
                ))}
              </nav>
            </div>
          </div>

          <div className="flex items-center gap-3 text-sm text-[#475467]">
            <div className="hidden items-center gap-2 md:inline-flex">
              <Gauge size={17} strokeWidth={2.1} />
              <span>已装 {data.localSkills.length} 个 Skill</span>
            </div>
            <div className="hidden items-center gap-2 md:inline-flex">
              <Clock3 size={17} strokeWidth={2.1} />
              <span>{data.runtimeProfile.sessions} 个会话</span>
            </div>
            <button
              aria-label="更多设置"
              className="flex h-10 w-10 items-center justify-center rounded-full text-[#475467] transition-colors duration-150 hover:bg-[#f4f5f7] hover:text-[#111827]"
              onClick={() => openSettings("general")}
              type="button"
            >
              <CircleUserRound size={20} strokeWidth={2.1} />
            </button>
          </div>
        </div>
      </header>

      <section className="flex min-h-0 flex-1 flex-col overflow-hidden bg-white">
        <div className="min-h-0 flex-1 overflow-hidden px-5 pt-2 sm:px-6 lg:px-8 lg:pt-3">
          <div
            className="chat-scroll-area mx-auto flex h-full w-full max-w-[980px] flex-col overflow-y-auto pr-1 sm:pr-2"
            ref={messagesViewportRef}
          >
            {messages.length > 0 || isSending ? (
              <div className="space-y-8 pb-16 pt-3 sm:space-y-10">
                {messages.map((message, index) => {
                  const isUser = message.role === "user";

                  return (
                    <div
                      className={isUser ? "ml-auto flex max-w-[300px] flex-col items-end md:max-w-[360px]" : "max-w-[680px]"}
                      key={`${message.role}:${message.timestamp}:${index}`}
                    >
                      {isUser ? (
                        <div className="rounded-[18px] bg-[#fcf1f1] px-4 py-3 text-[15px] leading-7 text-[#1f2937]">
                          <div className="whitespace-pre-wrap break-words">{message.content}</div>
                        </div>
                      ) : (
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2 text-[12px] text-[#98a2b3]">
                            <span className="font-medium text-[#4b5563]">{message.agent_name || activeAgentLabel}</span>
                            <span>{formatMessageTime(message.timestamp)}</span>
                          </div>
                          <div className="mt-2.5 break-words text-[15px] leading-[1.9] text-[#1f2937]">
                            <MarkdownMessage content={message.content} />
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}

                {isSending ? <ThinkingIndicator activeAgentLabel={activeAgentLabel} /> : null}
              </div>
            ) : (
              <div className="flex-1" />
            )}
          </div>
        </div>

        <ApprovalNotice approvalActionId={approvalActionId} approvals={pendingApprovals} onResolve={resolveApproval} />

        <Composer
          activeAgentLabel={activeAgentLabel}
          canSend={Boolean(draft.trim()) && pendingApprovals.length === 0 && approvalActionId === null}
          draft={draft}
          error={error}
          isSending={isSending}
          modelLabel={modelLabel}
          onDraftChange={setDraft}
          onReset={resetConversation}
          onSend={sendMessage}
          sessionId={sessionId}
        />
      </section>
    </div>
  );
}
