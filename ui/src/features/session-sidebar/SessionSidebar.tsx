import { LoaderCircle, MessageSquareText, Plus, Search } from "lucide-react";
import { type MouseEvent, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

import {
  CHAT_DELETE_EVENT,
  CHAT_RESET_EVENT,
  CHAT_SELECT_EVENT,
  CHAT_SYNC_EVENT,
  type ChatSyncPayload,
  type PersistedChatState,
  type StoredChatSession,
  readPersistedChatState,
} from "@/features/chat/useWebChat";
import { ChannelsSidebar } from "@/features/channels/ChannelsSidebar";
import { DiscoverySidebar } from "@/features/discovery/DiscoverySidebar";
import { MarketSidebar } from "@/features/market/MarketSidebar";
import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

function normalizeAgentName(value: string | null | undefined) {
  const normalized = (value ?? "").trim();
  return normalized === "" ? null : normalized;
}

function isSameAgentName(left: string | null | undefined, right: string | null | undefined) {
  return normalizeAgentName(left)?.toLowerCase() === normalizeAgentName(right)?.toLowerCase();
}

function formatSessionTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";

  const now = new Date();
  const diffMinutes = Math.floor((now.getTime() - date.getTime()) / 60000);

  if (diffMinutes < 1) return "刚刚";
  if (diffMinutes < 60) return `${diffMinutes} 分钟前`;

  const sameDay = now.toDateString() === date.toDateString();
  if (sameDay) {
    return date.toLocaleTimeString("zh-CN", {
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  return date.toLocaleDateString("zh-CN", {
    month: "numeric",
    day: "numeric",
  });
}

function getLastPreview(session: StoredChatSession) {
  const lastMessage = [...session.messages]
    .reverse()
    .find((message) => message.role === "user" || message.role === "assistant");

  return lastMessage?.content.trim() || "继续这个会话";
}

function matchesQuery(session: StoredChatSession, query: string) {
  const keyword = query.trim().toLowerCase();
  if (keyword === "") return true;

  return [session.title, getLastPreview(session)]
    .join(" ")
    .toLowerCase()
    .includes(keyword);
}

function ChatSessionSidebar() {
  const { data } = useWorkspaceOverview();
  const navigate = useNavigate();
  const openSettings = useShellStore((state) => state.openSettings);
  const [query, setQuery] = useState("");
  const [chatState, setChatState] = useState<PersistedChatState>(() => readPersistedChatState());
  const [contextMenu, setContextMenu] = useState<{ sessionKey: string; x: number; y: number } | null>(null);
  const [sendingSessionKey, setSendingSessionKey] = useState<string | null>(null);
  const defaultProvider = data.providers.find((provider) => provider.isDefault) ?? data.providers[0] ?? null;
  const activeAgent = data.localAgents.find((agent) => agent.active) ?? data.localAgents[0] ?? null;

  useEffect(() => {
    const sync = (event?: Event) => {
      const detail = (event as CustomEvent<ChatSyncPayload> | undefined)?.detail;
      if (detail && Array.isArray(detail.sessions)) {
        setChatState({
          selectedSessionKey: detail.selectedSessionKey,
          sessions: detail.sessions,
          version: detail.version,
        });
        setSendingSessionKey(typeof detail.sendingSessionKey === "string" ? detail.sendingSessionKey : null);
        return;
      }

      setChatState(readPersistedChatState());
      setSendingSessionKey(null);
    };

    window.addEventListener(CHAT_SYNC_EVENT, sync as EventListener);
    window.addEventListener("storage", sync);

    return () => {
      window.removeEventListener(CHAT_SYNC_EVENT, sync as EventListener);
      window.removeEventListener("storage", sync);
    };
  }, []);

  useEffect(() => {
    if (!contextMenu) return;

    const closeMenu = () => {
      setContextMenu(null);
    };

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setContextMenu(null);
      }
    };

    window.addEventListener("click", closeMenu);
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("resize", closeMenu);
    window.addEventListener("scroll", closeMenu, true);

    return () => {
      window.removeEventListener("click", closeMenu);
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("resize", closeMenu);
      window.removeEventListener("scroll", closeMenu, true);
    };
  }, [contextMenu]);

  const activeSessions = useMemo(() => {
    return chatState.sessions
      .filter((session) => {
        if (!activeAgent) return true;
        return isSameAgentName(session.agentName, activeAgent.name);
      })
      .filter((session) => matchesQuery(session, query));
  }, [activeAgent, chatState.sessions, query]);

  function handleOpenConversation(sessionKey: string) {
    setContextMenu(null);
    window.dispatchEvent(new CustomEvent(CHAT_SELECT_EVENT, { detail: { sessionKey } }));
    navigate("/");
  }

  function handleNewConversation() {
    setContextMenu(null);
    window.dispatchEvent(new Event(CHAT_RESET_EVENT));
    navigate("/");
  }

  function handleSessionContextMenu(event: MouseEvent<HTMLDivElement>, sessionKey: string) {
    event.preventDefault();
    setContextMenu({
      sessionKey,
      x: event.clientX,
      y: event.clientY,
    });
  }

  function handleDeleteConversation(sessionKey: string) {
    setContextMenu(null);
    window.dispatchEvent(new CustomEvent(CHAT_DELETE_EVENT, { detail: { sessionKey } }));
    navigate("/");
  }

  return (
    <aside className="relative z-10 flex w-full shrink-0 flex-col border-b border-[rgba(15,23,42,0.06)] bg-[#fbfbfc] px-4 py-5 lg:fixed lg:inset-y-0 lg:left-[104px] lg:w-[352px] lg:min-w-[352px] lg:border-b-0 lg:border-r lg:px-5 lg:py-6">
      <div className="shrink-0">
        <label className="flex items-center gap-3 rounded-[22px] border border-[rgba(15,23,42,0.08)] bg-white px-4 py-3 shadow-[0_8px_24px_rgba(15,23,42,0.04)]">
          <Search className="text-[#98a2b3]" size={18} strokeWidth={2} />
          <input
            className="w-full bg-transparent text-[15px] text-[#1d1f25] outline-none placeholder:text-[#a0a9b7]"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索会话"
            value={query}
          />
        </label>

        <button
          className="mt-5 inline-flex h-14 w-full items-center justify-center gap-2 rounded-[22px] border border-[rgba(15,23,42,0.08)] bg-white text-[16px] font-medium text-[#1d1f25] shadow-[0_10px_24px_rgba(15,23,42,0.04)] transition-colors duration-150 hover:bg-[#fafafa]"
          onClick={() => openSettings("agents")}
          type="button"
        >
          <Plus size={20} strokeWidth={2.1} />
          <span>新建 Agent</span>
        </button>

        {activeAgent ? (
          <button
            className="mt-5 flex w-full items-start gap-4 rounded-[24px] bg-white p-4 text-left shadow-[0_14px_30px_rgba(15,23,42,0.06)] transition-transform duration-150 hover:-translate-y-0.5"
            onClick={() => openSettings("agents")}
            type="button"
          >
            <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-[radial-gradient(circle_at_30%_28%,#ffe5bd,transparent_34%),linear-gradient(145deg,#ff9a62,#ff6d3b)] text-lg font-semibold text-white shadow-[0_12px_24px_rgba(255,121,67,0.24)]">
              {activeAgent.name.slice(0, 1).toUpperCase()}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[18px] font-semibold tracking-[-0.03em] text-[#1d1f25]">
                {activeAgent.name}
              </span>
              <span className="mt-2 block text-sm leading-6 text-[#667085]">{activeAgent.summary}</span>
            </span>
          </button>
        ) : null}
      </div>

      <div className="mt-6 min-h-0 flex-1 overflow-y-auto">
        <div className="flex items-center justify-between px-1">
          <div className="text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">
            {activeAgent ? `${activeAgent.name} 会话` : "会话"}
          </div>
          <button
            className="text-xs font-medium text-[#667085] transition-colors duration-150 hover:text-[#111827]"
            onClick={handleNewConversation}
            type="button"
          >
            新建
          </button>
        </div>

        {activeSessions.length > 0 ? (
          <div className="mt-3 overflow-hidden rounded-[22px] border border-[rgba(15,23,42,0.08)] bg-white shadow-[0_12px_24px_rgba(15,23,42,0.05)]">
            {activeSessions.map((session, index) => {
              const isActive = chatState.selectedSessionKey === session.key;
              const isThinking = sendingSessionKey === session.key;

              return (
                <div
                  className={[
                    isActive ? "bg-[#f8fafc]" : "bg-white",
                    index === 0 ? "" : "border-t border-[rgba(15,23,42,0.06)]",
                  ].join(" ")}
                  key={session.key}
                  onContextMenu={(event) => handleSessionContextMenu(event, session.key)}
                >
                  <button
                    className={[
                      "flex w-full items-start gap-3 px-4 py-3 text-left transition-colors duration-150 hover:bg-[#f8fafc]",
                      isActive ? "bg-[#f8fafc]" : "bg-white",
                    ].join(" ")}
                    onClick={() => handleOpenConversation(session.key)}
                    type="button"
                  >
                    <span
                      className={[
                        "mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-[14px]",
                        isThinking ? "bg-[#eef4ff] text-[#2563eb]" : "bg-[#f4f5f7] text-[#667085]",
                      ].join(" ")}
                    >
                      {isThinking ? (
                        <LoaderCircle className="animate-spin" size={17} strokeWidth={2.1} />
                      ) : (
                        <MessageSquareText size={17} strokeWidth={2.1} />
                      )}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center justify-between gap-3">
                        <span className="truncate text-[14px] font-medium text-[#1d1f25]">{session.title}</span>
                        <span className="shrink-0 text-[11px] text-[#98a2b3]">
                          {isThinking ? "思考中" : formatSessionTime(session.updatedAt)}
                        </span>
                      </span>
                      <span className="mt-1 block truncate text-sm text-[#667085]">{getLastPreview(session)}</span>
                    </span>
                  </button>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="mt-3 rounded-[22px] border border-dashed border-[rgba(15,23,42,0.1)] bg-white px-4 py-5 text-sm leading-6 text-[#667085]">
            {query.trim() === ""
              ? activeAgent
                ? `当前还没有 ${activeAgent.name} 的历史会话，点击右上角“新建”即可开始。`
                : "当前还没有历史会话。"
              : "没有找到匹配的会话。"}
          </div>
        )}
      </div>

      {contextMenu ? (
        <div
          className="fixed z-50 min-w-[156px] rounded-[18px] border border-[rgba(15,23,42,0.08)] bg-white p-2 shadow-[0_18px_48px_rgba(15,23,42,0.18)]"
          onClick={(event) => event.stopPropagation()}
          onContextMenu={(event) => event.preventDefault()}
          style={{
            left: `${contextMenu.x}px`,
            top: `${contextMenu.y}px`,
          }}
        >
          <button
            className="flex w-full items-center rounded-[12px] px-3 py-2 text-left text-sm font-medium text-[#b42318] transition-colors duration-150 hover:bg-[#fff2f0]"
            onClick={() => handleDeleteConversation(contextMenu.sessionKey)}
            type="button"
          >
            删除对话
          </button>
        </div>
      ) : null}

      <div className="mt-6 shrink-0 border-t border-[rgba(15,23,42,0.06)] px-1 pb-1 pt-5">
        <div className="text-sm font-medium text-[#1d1f25]">{activeAgent?.name || "AnyClaw"}</div>
        <div className="mt-1 text-xs text-[#667085]">
          {defaultProvider?.model || data.runtimeProfile.model || data.runtimeProfile.providerLabel || "默认模型"}
        </div>
      </div>
    </aside>
  );
}

export function SessionSidebar() {
  const { pathname } = useLocation();

  if (pathname.startsWith("/market")) {
    return <MarketSidebar />;
  }

  if (pathname.startsWith("/channels")) {
    return <ChannelsSidebar />;
  }

  if (pathname.startsWith("/discovery")) {
    return <DiscoverySidebar />;
  }

  return <ChatSessionSidebar />;
}
