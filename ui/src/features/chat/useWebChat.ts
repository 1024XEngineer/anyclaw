import { useEffect, useRef, useState } from "react";

export type ChatMessage = {
  agent_name?: string;
  content: string;
  role: "assistant" | "user";
  timestamp: string;
};

export type ChatApproval = {
  action?: string;
  id: string;
  payload?: Record<string, unknown>;
  requested_at?: string;
  session_id?: string;
  status?: string;
  tool_name: string;
};

export type StoredChatSession = {
  agentName: string | null;
  createdAt: string;
  key: string;
  messages: ChatMessage[];
  remoteSessionId: string | null;
  title: string;
  updatedAt: string;
};

export type PersistedChatState = {
  selectedSessionKey: string | null;
  sessions: StoredChatSession[];
  version: 2;
};

export type ChatSyncPayload = PersistedChatState & {
  sendingSessionKey: string | null;
};

type ChatState = {
  messages: ChatMessage[];
  selectedSessionKey: string | null;
  sessionId: string | null;
  sessions: StoredChatSession[];
};

type LegacyPersistedChatState = {
  messages?: unknown;
  sessionId?: unknown;
};

type GatewaySessionMessage = {
  content?: string;
  created_at?: string;
  role?: "assistant" | "user";
};

type GatewaySession = {
  agent?: string;
  id: string;
  messages?: GatewaySessionMessage[];
  presence?: string;
  typing?: boolean;
};

type GatewayApproval = {
  action?: string;
  id?: string;
  payload?: Record<string, unknown>;
  requested_at?: string;
  session_id?: string;
  status?: string;
  tool_name?: string;
};

type ChatResponse = {
  approvals?: GatewayApproval[];
  response?: string;
  session?: GatewaySession;
  status?: string;
};

type GatewayStatus = {
  working_dir?: string;
};

type ChatSelectDetail = {
  sessionKey?: string;
};

export const CHAT_STORAGE_KEY = "anyclaw-control-ui-chat-v1";
export const CHAT_SYNC_EVENT = "anyclaw:chat-sync";
export const CHAT_RESET_EVENT = "anyclaw:chat-reset";
export const CHAT_SELECT_EVENT = "anyclaw:chat-select";

const STORAGE_VERSION = 2;
const DEFAULT_WORKSPACE_ID = "workspace-default";
const APPROVAL_POLL_ATTEMPTS = 18;
const APPROVAL_POLL_INTERVAL_MS = 900;
const EMPTY_PERSISTED_STATE: PersistedChatState = {
  selectedSessionKey: null,
  sessions: [],
  version: STORAGE_VERSION,
};

function getErrorMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  return "发送失败，请稍后重试。";
}

async function requestJSON<T>(input: string, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });

  const text = await response.text();
  const payload = text === "" ? null : (JSON.parse(text) as T | { error?: string });

  if (!response.ok) {
    const message =
      payload && typeof payload === "object" && "error" in payload && typeof payload.error === "string"
        ? payload.error
        : `${response.status} ${response.statusText}`.trim();

    throw new Error(message);
  }

  return payload as T;
}

function deriveWorkspaceId(workingDir: string | null | undefined) {
  const source = (workingDir ?? "").trim().toLowerCase();
  if (source === "") return DEFAULT_WORKSPACE_ID;

  const clean = source.replace(/[:\\/ ]/g, "-").replace(/^[-.]+|[-.]+$/g, "");
  return clean === "" ? DEFAULT_WORKSPACE_ID : `ws-${clean}`;
}

function looksAbsoluteWorkspacePath(workingDir: string | null | undefined) {
  const value = (workingDir ?? "").trim();
  return /^[a-z]:[\\/]/i.test(value) || value.startsWith("\\\\") || value.startsWith("/");
}

function normalizeAgentName(value: string | null | undefined) {
  const normalized = (value ?? "").trim();
  return normalized === "" ? null : normalized;
}

function isSameAgentName(left: string | null | undefined, right: string | null | undefined) {
  return normalizeAgentName(left)?.toLowerCase() === normalizeAgentName(right)?.toLowerCase();
}

function createSessionKey() {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `chat-${crypto.randomUUID()}`;
  }

  return `chat-${Date.now()}-${Math.random().toString(16).slice(2, 10)}`;
}

function truncateText(value: string, maxLength = 36) {
  const compact = value.replace(/\s+/g, " ").trim();
  if (compact.length <= maxLength) return compact;
  return `${compact.slice(0, maxLength).trimEnd()}...`;
}

function normalizeMessage(input: unknown): ChatMessage | null {
  if (!input || typeof input !== "object") return null;

  const message = input as Partial<ChatMessage>;
  if (typeof message.content !== "string") return null;
  if (message.role !== "assistant" && message.role !== "user") return null;

  return {
    agent_name: normalizeAgentName(message.agent_name) ?? undefined,
    content: message.content,
    role: message.role,
    timestamp: typeof message.timestamp === "string" && message.timestamp.trim() !== ""
      ? message.timestamp
      : new Date().toISOString(),
  };
}

function normalizeMessages(input: unknown) {
  if (!Array.isArray(input)) return [];
  return input.map((message) => normalizeMessage(message)).filter((message): message is ChatMessage => message !== null);
}

function normalizeApproval(input: unknown): ChatApproval | null {
  if (!input || typeof input !== "object") return null;

  const approval = input as GatewayApproval;
  if (typeof approval.id !== "string" || approval.id.trim() === "") return null;
  if (typeof approval.tool_name !== "string" || approval.tool_name.trim() === "") return null;

  return {
    action: typeof approval.action === "string" ? approval.action : undefined,
    id: approval.id,
    payload: approval.payload && typeof approval.payload === "object" ? approval.payload : undefined,
    requested_at: typeof approval.requested_at === "string" ? approval.requested_at : undefined,
    session_id: typeof approval.session_id === "string" ? approval.session_id : undefined,
    status: typeof approval.status === "string" ? approval.status : undefined,
    tool_name: approval.tool_name,
  };
}

function normalizeApprovals(input: unknown) {
  if (!Array.isArray(input)) return [];
  return input.map((approval) => normalizeApproval(approval)).filter((approval): approval is ChatApproval => approval !== null);
}

function sleep(ms: number) {
  return new Promise<void>((resolve) => {
    setTimeout(resolve, ms);
  });
}

function isWaitingApprovalStatus(value: string | null | undefined) {
  const normalized = (value ?? "").trim().toLowerCase();
  return normalized === "waiting_approval" || normalized === "pending_approval";
}

function filterSessionApprovals(approvals: ChatApproval[], activeSessionId: string | null) {
  if (!activeSessionId) return [];
  return approvals.filter((approval) => approval.session_id === activeSessionId);
}

function inferAgentName(messages: ChatMessage[], fallbackAgentName?: string | null) {
  const assistantAgentName = messages.find((message) => normalizeAgentName(message.agent_name))?.agent_name;
  return normalizeAgentName(assistantAgentName) ?? normalizeAgentName(fallbackAgentName);
}

function deriveSessionTitle(messages: ChatMessage[]) {
  const titleSource =
    messages.find((message) => message.role === "user" && message.content.trim() !== "")?.content ??
    messages.find((message) => message.content.trim() !== "")?.content ??
    "新对话";

  return truncateText(titleSource);
}

function deriveCreatedAt(messages: ChatMessage[]) {
  return messages[0]?.timestamp ?? new Date().toISOString();
}

function deriveUpdatedAt(messages: ChatMessage[]) {
  return messages[messages.length - 1]?.timestamp ?? new Date().toISOString();
}

function buildStoredSession(params: {
  agentName: string | null | undefined;
  existing?: StoredChatSession;
  key?: string | null;
  messages: ChatMessage[];
  remoteSessionId: string | null | undefined;
}) {
  const normalizedMessages = normalizeMessages(params.messages);
  const createdAt = params.existing?.createdAt ?? deriveCreatedAt(normalizedMessages);

  return {
    agentName: inferAgentName(normalizedMessages, params.agentName),
    createdAt,
    key: params.key || params.existing?.key || params.remoteSessionId || createSessionKey(),
    messages: normalizedMessages,
    remoteSessionId: typeof params.remoteSessionId === "string" ? params.remoteSessionId : null,
    title: deriveSessionTitle(normalizedMessages),
    updatedAt: deriveUpdatedAt(normalizedMessages),
  } satisfies StoredChatSession;
}

function normalizeStoredSession(input: unknown): StoredChatSession | null {
  if (!input || typeof input !== "object") return null;

  const session = input as Partial<StoredChatSession> & {
    agent?: string;
    agent_name?: string;
    id?: string;
    sessionId?: string | null;
  };
  const messages = normalizeMessages(session.messages);
  if (messages.length === 0) return null;

  const remoteSessionId =
    typeof session.remoteSessionId === "string"
      ? session.remoteSessionId
      : typeof session.sessionId === "string"
        ? session.sessionId
        : null;

  const key =
    typeof session.key === "string" && session.key.trim() !== ""
      ? session.key
      : typeof session.id === "string" && session.id.trim() !== ""
        ? session.id
        : remoteSessionId || createSessionKey();

  return buildStoredSession({
    agentName: session.agentName ?? session.agent ?? session.agent_name ?? inferAgentName(messages),
    existing: {
      agentName: normalizeAgentName(session.agentName ?? session.agent ?? session.agent_name),
      createdAt:
        typeof session.createdAt === "string" && session.createdAt.trim() !== ""
          ? session.createdAt
          : deriveCreatedAt(messages),
      key,
      messages,
      remoteSessionId,
      title:
        typeof session.title === "string" && session.title.trim() !== ""
          ? session.title
          : deriveSessionTitle(messages),
      updatedAt:
        typeof session.updatedAt === "string" && session.updatedAt.trim() !== ""
          ? session.updatedAt
          : deriveUpdatedAt(messages),
    },
    key,
    messages,
    remoteSessionId,
  });
}

function normalizePersistedState(input: unknown): PersistedChatState | null {
  if (!input || typeof input !== "object") return null;

  const parsed = input as Partial<PersistedChatState>;
  if (!Array.isArray(parsed.sessions)) return null;

  const sessions = parsed.sessions
    .map((session) => normalizeStoredSession(session))
    .filter((session): session is StoredChatSession => session !== null)
    .sort((left, right) => new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime());

  const selectedSessionKey =
    typeof parsed.selectedSessionKey === "string" && sessions.some((session) => session.key === parsed.selectedSessionKey)
      ? parsed.selectedSessionKey
      : null;

  return {
    selectedSessionKey,
    sessions,
    version: STORAGE_VERSION,
  };
}

function migrateLegacyState(input: unknown): PersistedChatState | null {
  if (!input || typeof input !== "object") return null;

  const parsed = input as LegacyPersistedChatState;
  const messages = normalizeMessages(parsed.messages);
  if (messages.length === 0) return null;

  const remoteSessionId = typeof parsed.sessionId === "string" ? parsed.sessionId : null;
  const session = buildStoredSession({
    agentName: inferAgentName(messages),
    key: remoteSessionId || createSessionKey(),
    messages,
    remoteSessionId,
  });

  return {
    selectedSessionKey: session.key,
    sessions: [session],
    version: STORAGE_VERSION,
  };
}

function parsePersistedState(raw: string | null): PersistedChatState | null {
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw) as unknown;
    return normalizePersistedState(parsed) ?? migrateLegacyState(parsed);
  } catch {
    return null;
  }
}

function writePersistedState(state: PersistedChatState) {
  if (typeof window === "undefined") return;

  window.localStorage.setItem(CHAT_STORAGE_KEY, JSON.stringify(state));
  window.sessionStorage.removeItem(CHAT_STORAGE_KEY);
}

function emitChatSync(payload: ChatSyncPayload) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent(CHAT_SYNC_EVENT, { detail: payload }));
}

export function readPersistedChatState(): PersistedChatState {
  if (typeof window === "undefined") {
    return EMPTY_PERSISTED_STATE;
  }

  const localState = parsePersistedState(window.localStorage.getItem(CHAT_STORAGE_KEY));
  if (localState) {
    return localState;
  }

  const legacyState = parsePersistedState(window.sessionStorage.getItem(CHAT_STORAGE_KEY));
  if (legacyState) {
    writePersistedState(legacyState);
    return legacyState;
  }

  return EMPTY_PERSISTED_STATE;
}

function mapSessionMessages(session: GatewaySession | undefined, fallbackAgentName: string | null) {
  const agentName = normalizeAgentName(session?.agent) ?? normalizeAgentName(fallbackAgentName) ?? undefined;

  return (session?.messages ?? [])
    .filter(
      (message): message is GatewaySessionMessage & { content: string; role: "assistant" | "user" } =>
        typeof message.content === "string" &&
        (message.role === "assistant" || message.role === "user"),
    )
    .map((message) => ({
      agent_name: message.role === "assistant" ? agentName : undefined,
      content: message.content,
      role: message.role,
      timestamp: message.created_at || new Date().toISOString(),
    }));
}

function getSessionByKey(sessions: StoredChatSession[], sessionKey: string | null) {
  if (!sessionKey) return null;
  return sessions.find((session) => session.key === sessionKey) ?? null;
}

function findSessionByRemoteId(sessions: StoredChatSession[], remoteSessionId: string | null) {
  if (!remoteSessionId) return null;
  return sessions.find((session) => session.remoteSessionId === remoteSessionId) ?? null;
}

function findLatestSessionForAgent(sessions: StoredChatSession[], agentName: string | null) {
  const normalizedAgentName = normalizeAgentName(agentName);

  if (!normalizedAgentName) {
    return sessions[0] ?? null;
  }

  return sessions.find((session) => isSameAgentName(session.agentName, normalizedAgentName)) ?? null;
}

function sortSessions(sessions: StoredChatSession[]) {
  return [...sessions].sort((left, right) => new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime());
}

function createInitialChatState(agentName: string | null): ChatState {
  const persistedState = readPersistedChatState();
  const rememberedSession = getSessionByKey(persistedState.sessions, persistedState.selectedSessionKey);
  const selectedSession =
    rememberedSession && (!agentName || isSameAgentName(rememberedSession.agentName, agentName))
      ? rememberedSession
      : findLatestSessionForAgent(persistedState.sessions, agentName) ?? rememberedSession;

  return {
    messages: selectedSession?.messages ?? [],
    selectedSessionKey: selectedSession?.key ?? null,
    sessionId: selectedSession?.remoteSessionId ?? null,
    sessions: persistedState.sessions,
  };
}

function toPersistedState(state: ChatState): PersistedChatState {
  return {
    selectedSessionKey: state.selectedSessionKey,
    sessions: sortSessions(state.sessions),
    version: STORAGE_VERSION,
  };
}

function startNewConversation(state: ChatState): ChatState {
  return {
    ...state,
    messages: [],
    selectedSessionKey: null,
    sessionId: null,
  };
}

function hydrateSession(state: ChatState, sessionKey: string) {
  const session = getSessionByKey(state.sessions, sessionKey);
  if (!session) return state;

  return {
    ...state,
    messages: session.messages,
    selectedSessionKey: session.key,
    sessionId: session.remoteSessionId,
  };
}

function upsertConversation(
  state: ChatState,
  agentName: string | null,
  nextMessages: ChatMessage[],
  nextSessionId: string | null,
): ChatState {
  if (nextMessages.length === 0) {
    return {
      ...state,
      messages: [],
      sessionId: nextSessionId,
    };
  }

  const remoteSessionMatch = findSessionByRemoteId(state.sessions, nextSessionId);
  const sessionKey = remoteSessionMatch?.key ?? state.selectedSessionKey ?? createSessionKey();
  const existingSession = getSessionByKey(state.sessions, sessionKey) ?? remoteSessionMatch ?? undefined;
  const storedSession = buildStoredSession({
    agentName,
    existing: existingSession,
    key: sessionKey,
    messages: nextMessages,
    remoteSessionId: nextSessionId,
  });

  const sessions = sortSessions(
    state.sessions.filter(
      (session) =>
        session.key !== storedSession.key &&
        (!storedSession.remoteSessionId || session.remoteSessionId !== storedSession.remoteSessionId),
    ),
  );

  return {
    messages: storedSession.messages,
    selectedSessionKey: storedSession.key,
    sessionId: storedSession.remoteSessionId,
    sessions: sortSessions([storedSession, ...sessions]),
  };
}

export function useWebChat(agentName: string | null, workspacePath: string | null) {
  const [chatState, setChatState] = useState<ChatState>(() => createInitialChatState(agentName));
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSending, setIsSending] = useState(false);
  const [approvalActionId, setApprovalActionId] = useState<string | null>(null);
  const [pendingApprovals, setPendingApprovals] = useState<ChatApproval[]>([]);
  const [workspaceId, setWorkspaceId] = useState<string | null>(() =>
    looksAbsoluteWorkspacePath(workspacePath) ? deriveWorkspaceId(workspacePath) : null,
  );
  const previousAgentNameRef = useRef<string | null>(normalizeAgentName(agentName));

  const { messages, selectedSessionKey, sessionId, sessions } = chatState;

  useEffect(() => {
    const payload = toPersistedState(chatState);
    writePersistedState(payload);
    emitChatSync({
      ...payload,
      sendingSessionKey: isSending ? selectedSessionKey : null,
    });
  }, [chatState, isSending, selectedSessionKey]);

  useEffect(() => {
    setError(null);

    const normalizedAgentName = normalizeAgentName(agentName);
    if (previousAgentNameRef.current === normalizedAgentName) {
      return;
    }

    previousAgentNameRef.current = normalizedAgentName;
    setDraft("");

    setChatState((current) => {
      const currentSession = getSessionByKey(current.sessions, current.selectedSessionKey);
      if (currentSession && isSameAgentName(currentSession.agentName, normalizedAgentName)) {
        return current;
      }

      const fallbackSession = findLatestSessionForAgent(current.sessions, normalizedAgentName);
      if (!fallbackSession) {
        return startNewConversation(current);
      }

      return hydrateSession(current, fallbackSession.key);
    });
  }, [agentName]);

  useEffect(() => {
    if (typeof window === "undefined") return;

    const onReset = () => {
      setDraft("");
      setError(null);
      setApprovalActionId(null);
      setPendingApprovals([]);
      setChatState((current) => startNewConversation(current));
    };

    const onSelect = (event: Event) => {
      const detail = (event as CustomEvent<ChatSelectDetail>).detail;
      if (!detail?.sessionKey) return;

      setDraft("");
      setError(null);
      setApprovalActionId(null);
      setPendingApprovals([]);
      setChatState((current) => hydrateSession(current, detail.sessionKey!));
    };

    window.addEventListener(CHAT_RESET_EVENT, onReset);
    window.addEventListener(CHAT_SELECT_EVENT, onSelect as EventListener);

    return () => {
      window.removeEventListener(CHAT_RESET_EVENT, onReset);
      window.removeEventListener(CHAT_SELECT_EVENT, onSelect as EventListener);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function syncWorkspaceId() {
      if (looksAbsoluteWorkspacePath(workspacePath)) {
        if (!cancelled) {
          setWorkspaceId(deriveWorkspaceId(workspacePath));
        }
        return;
      }

      try {
        const status = await requestJSON<GatewayStatus>("/status");
        if (!cancelled) {
          setWorkspaceId(deriveWorkspaceId(status.working_dir || workspacePath));
        }
      } catch {
        if (!cancelled) {
          setWorkspaceId(deriveWorkspaceId(workspacePath));
        }
      }
    }

    void syncWorkspaceId();

    return () => {
      cancelled = true;
    };
  }, [workspacePath]);

  async function resolveWorkspaceId(forceRefresh = false) {
    if (!forceRefresh && workspaceId) {
      return workspaceId;
    }

    try {
      const status = await requestJSON<GatewayStatus>("/status");
      const resolvedId = deriveWorkspaceId(status.working_dir || workspacePath);
      setWorkspaceId(resolvedId);
      return resolvedId;
    } catch {
      const fallbackId = deriveWorkspaceId(workspacePath);
      setWorkspaceId(fallbackId);
      return fallbackId;
    }
  }

  async function postMessage(message: string, activeSessionId: string | null, activeWorkspaceId: string) {
    return requestJSON<ChatResponse>(`/chat?workspace=${encodeURIComponent(activeWorkspaceId)}`, {
      body: JSON.stringify({
        agent: agentName ?? undefined,
        message,
        session_id: activeSessionId ?? undefined,
        title: activeSessionId ? undefined : "Web Chat",
      }),
      method: "POST",
    });
  }

  async function fetchSessionSnapshot(activeSessionId: string | null) {
    if (!activeSessionId) return null;
    return requestJSON<GatewaySession>(`/sessions/${encodeURIComponent(activeSessionId)}`);
  }

  async function fetchPendingApprovals(activeSessionId: string | null) {
    if (!activeSessionId) return [];
    const approvals = normalizeApprovals(await requestJSON<GatewayApproval[]>("/approvals?status=pending"));
    return filterSessionApprovals(approvals, activeSessionId);
  }

  function applySessionSnapshot(session: GatewaySession | null) {
    if (!session) return;

    const mappedMessages = mapSessionMessages(session, agentName);
    if (mappedMessages.length === 0) return;

    setChatState((current) =>
      upsertConversation(current, agentName, mappedMessages, session.id ?? current.sessionId),
    );
  }

  async function syncSessionState(activeSessionId: string | null, options?: { skipSession?: boolean }) {
    if (!activeSessionId) {
      setPendingApprovals([]);
      return { approvals: [] as ChatApproval[], session: null as GatewaySession | null };
    }

    const session = options?.skipSession ? null : await fetchSessionSnapshot(activeSessionId);
    if (session) {
      applySessionSnapshot(session);
    }

    const approvals = await fetchPendingApprovals(session?.id ?? activeSessionId);
    setPendingApprovals(approvals);

    return { approvals, session };
  }

  async function waitForApprovalResult(activeSessionId: string, baselineMessageCount: number) {
    let idleStreak = 0;

    for (let attempt = 0; attempt < APPROVAL_POLL_ATTEMPTS; attempt += 1) {
      await sleep(APPROVAL_POLL_INTERVAL_MS);

      try {
        const session = await fetchSessionSnapshot(activeSessionId);
        applySessionSnapshot(session);

        const approvals = await fetchPendingApprovals(session?.id ?? activeSessionId);
        setPendingApprovals(approvals);

        const messageCount = session?.messages?.length ?? baselineMessageCount;
        const waitingApproval = isWaitingApprovalStatus(session?.presence);
        const typing = Boolean(session?.typing);

        if (!waitingApproval && !typing) {
          idleStreak += 1;
        } else {
          idleStreak = 0;
        }

        if (approvals.length > 0) return;
        if (messageCount > baselineMessageCount && idleStreak >= 1) return;
        if (idleStreak >= 2) return;
      } catch (pollError) {
        if (getErrorMessage(pollError).includes("session not found")) {
          setPendingApprovals([]);
          return;
        }
      }
    }
  }

  async function resolveApproval(approvalId: string, approved: boolean, comment?: string) {
    const activeSessionId = sessionId;
    if (!approvalId || approvalActionId) return;

    setApprovalActionId(approvalId);
    setError(null);

    try {
      await requestJSON<ChatApproval>(`/approvals/${encodeURIComponent(approvalId)}/resolve`, {
        body: JSON.stringify({
          approved,
          comment: comment ?? "",
        }),
        method: "POST",
      });

      setPendingApprovals((current) => current.filter((approval) => approval.id !== approvalId));

      if (!activeSessionId) return;

      if (approved) {
        await waitForApprovalResult(activeSessionId, messages.length);
        return;
      }

      await syncSessionState(activeSessionId);
      setError("已拒绝本次权限请求。");
    } catch (resolveError) {
      setError(getErrorMessage(resolveError));
    } finally {
      setApprovalActionId(null);
    }
  }

  function resetConversation() {
    setDraft("");
    setError(null);
    setApprovalActionId(null);
    setPendingApprovals([]);
    setChatState((current) => startNewConversation(current));
  }

  function selectSession(sessionKey: string) {
    setDraft("");
    setError(null);
    setApprovalActionId(null);
    setPendingApprovals([]);
    setChatState((current) => hydrateSession(current, sessionKey));
  }

  useEffect(() => {
    let cancelled = false;

    async function syncCurrentSession() {
      if (!sessionId) {
        setPendingApprovals([]);
        return;
      }

      try {
        const session = await fetchSessionSnapshot(sessionId);
        if (cancelled) return;

        applySessionSnapshot(session);

        const approvals = await fetchPendingApprovals(session?.id ?? sessionId);
        if (cancelled) return;

        setPendingApprovals(approvals);
      } catch (syncError) {
        if (cancelled) return;
        if (getErrorMessage(syncError).includes("session not found")) {
          setPendingApprovals([]);
        }
      }
    }

    void syncCurrentSession();

    return () => {
      cancelled = true;
    };
  }, [agentName, sessionId]);

  async function sendMessage() {
    const message = draft.trim();
    const currentSessionId = sessionId;
    if (message === "" || isSending || pendingApprovals.length > 0) return;

    const optimisticMessage: ChatMessage = {
      content: message,
      role: "user",
      timestamp: new Date().toISOString(),
    };

    setDraft("");
    setError(null);
    setIsSending(true);
    setChatState((current) => upsertConversation(current, agentName, [...current.messages, optimisticMessage], current.sessionId));

    try {
      let activeWorkspaceId = await resolveWorkspaceId();
      let response: ChatResponse;

      try {
        response = await postMessage(message, currentSessionId, activeWorkspaceId);
      } catch (error) {
        const messageText = getErrorMessage(error);

        if (currentSessionId && messageText.includes("session not found")) {
          response = await postMessage(message, null, activeWorkspaceId);
        } else if (messageText.includes("workspace not found") || messageText.includes("workspace is required")) {
          activeWorkspaceId = await resolveWorkspaceId(true);
          response = await postMessage(message, currentSessionId, activeWorkspaceId);
        } else {
          throw error;
        }
      }

      const resolvedSessionId = response.session?.id ?? currentSessionId;
      const mappedMessages = mapSessionMessages(response.session, agentName);
      if (mappedMessages.length > 0) {
        setChatState((current) =>
          upsertConversation(current, agentName, mappedMessages, resolvedSessionId ?? current.sessionId),
        );
      } else if (response.response) {
        const assistantMessage: ChatMessage = {
          agent_name: normalizeAgentName(agentName ?? response.session?.agent) ?? undefined,
          content: response.response,
          role: "assistant",
          timestamp: new Date().toISOString(),
        };

        setChatState((current) =>
          upsertConversation(current, agentName, [...current.messages, assistantMessage], resolvedSessionId ?? current.sessionId),
        );
      }

      if (response.status === "waiting_approval") {
        const immediateApprovals = filterSessionApprovals(normalizeApprovals(response.approvals), resolvedSessionId);

        if (immediateApprovals.length > 0) {
          setPendingApprovals(immediateApprovals);
        } else {
          await syncSessionState(resolvedSessionId, { skipSession: true });
        }
        setError("当前请求正在等待审批，批准后会继续执行。");
      }
      if (response.status === "waiting_approval") {
        setError(null);
      } else {
        setPendingApprovals([]);
      }
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setIsSending(false);
    }
  }

  return {
    approvalActionId,
    draft,
    error,
    isSending,
    messages,
    pendingApprovals,
    resetConversation,
    resolveApproval,
    selectedSessionKey,
    selectSession,
    sessionId,
    sessions,
    sendMessage,
    setDraft,
  };
}
