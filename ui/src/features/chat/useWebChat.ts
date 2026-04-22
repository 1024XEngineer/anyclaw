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
  created_at?: string;
  id: string;
  messages?: GatewaySessionMessage[];
  presence?: string;
  title?: string;
  typing?: boolean;
  updated_at?: string;
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

type ChatDeleteDetail = {
  sessionKey?: string;
};

export const CHAT_STORAGE_KEY = "anyclaw-control-ui-chat-v1";
export const CHAT_SYNC_EVENT = "anyclaw:chat-sync";
export const CHAT_RESET_EVENT = "anyclaw:chat-reset";
export const CHAT_SELECT_EVENT = "anyclaw:chat-select";
export const CHAT_DELETE_EVENT = "anyclaw:chat-delete";

const STORAGE_VERSION = 2;
const DEFAULT_WORKSPACE_ID = "workspace-default";
const APPROVAL_POLL_ATTEMPTS = 18;
const APPROVAL_POLL_INTERVAL_MS = 900;
const REMOTE_SESSION_SYNC_INTERVAL_MS = 4000;
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
  let payload: T | { error?: string } | string | null = null;
  if (text !== "") {
    try {
      payload = JSON.parse(text) as T | { error?: string };
    } catch {
      payload = text;
    }
  }

  if (!response.ok) {
    const message =
      typeof payload === "string" && payload.trim() !== ""
        ? payload.trim()
        : payload && typeof payload === "object" && "error" in payload && typeof payload.error === "string"
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

function normalizeTextValue(value: string | null | undefined) {
  const normalized = (value ?? "").trim();
  return normalized === "" ? null : normalized;
}

function normalizeTimestamp(value: string | null | undefined) {
  const normalized = normalizeTextValue(value);
  if (!normalized) return null;
  return Number.isNaN(new Date(normalized).getTime()) ? null : normalized;
}

function getTimeValue(value: string | null | undefined) {
  const normalized = normalizeTimestamp(value);
  if (!normalized) return 0;
  return new Date(normalized).getTime();
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

function resolveStoredSessionTitle(messages: ChatMessage[], fallbackTitle?: string | null) {
  const titleSource =
    messages.find((message) => message.role === "user" && message.content.trim() !== "")?.content ??
    messages.find((message) => message.content.trim() !== "")?.content ??
    normalizeTextValue(fallbackTitle) ??
    "新对话";

  return truncateText(titleSource);
}

function resolveStoredSessionCreatedAt(
  messages: ChatMessage[],
  explicitCreatedAt?: string | null,
  fallbackCreatedAt?: string | null,
) {
  return normalizeTimestamp(explicitCreatedAt) ?? normalizeTimestamp(fallbackCreatedAt) ?? deriveCreatedAt(messages);
}

function resolveStoredSessionUpdatedAt(
  messages: ChatMessage[],
  explicitUpdatedAt?: string | null,
  fallbackUpdatedAt?: string | null,
) {
  const explicit = normalizeTimestamp(explicitUpdatedAt);
  if (explicit) return explicit;

  const fallback = normalizeTimestamp(fallbackUpdatedAt);
  const derived = deriveUpdatedAt(messages);
  if (!fallback) return derived;

  return getTimeValue(derived) > getTimeValue(fallback) ? derived : fallback;
}

function buildStoredSession(params: {
  agentName: string | null | undefined;
  createdAt?: string | null;
  existing?: StoredChatSession;
  key?: string | null;
  messages: ChatMessage[];
  remoteSessionId: string | null | undefined;
  title?: string | null;
  updatedAt?: string | null;
}) {
  const normalizedMessages = normalizeMessages(params.messages);
  const createdAt = resolveStoredSessionCreatedAt(normalizedMessages, params.createdAt, params.existing?.createdAt);
  const updatedAt = resolveStoredSessionUpdatedAt(normalizedMessages, params.updatedAt, params.existing?.updatedAt);
  const title = resolveStoredSessionTitle(normalizedMessages, params.title ?? params.existing?.title);

  return {
    agentName: inferAgentName(normalizedMessages, params.agentName ?? params.existing?.agentName),
    createdAt,
    key: params.key || params.existing?.key || params.remoteSessionId || createSessionKey(),
    messages: normalizedMessages,
    remoteSessionId: typeof params.remoteSessionId === "string" ? params.remoteSessionId : null,
    title,
    updatedAt,
  } satisfies StoredChatSession;
}

function normalizeStoredSession(input: unknown): StoredChatSession | null {
  if (!input || typeof input !== "object") return null;

  const session = input as Partial<StoredChatSession> & {
    agent?: string;
    agent_name?: string;
    created_at?: string;
    id?: string;
    sessionId?: string | null;
    updated_at?: string;
  };
  const messages = normalizeMessages(session.messages);

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
  const providedTitle = normalizeTextValue(session.title);

  if (messages.length === 0 && !remoteSessionId && !providedTitle) return null;

  return buildStoredSession({
    agentName: session.agentName ?? session.agent ?? session.agent_name ?? inferAgentName(messages),
    createdAt:
      typeof session.createdAt === "string" && session.createdAt.trim() !== ""
        ? session.createdAt
        : typeof session.created_at === "string" && session.created_at.trim() !== ""
          ? session.created_at
          : null,
    existing: {
      agentName: normalizeAgentName(session.agentName ?? session.agent ?? session.agent_name),
      createdAt:
        typeof session.createdAt === "string" && session.createdAt.trim() !== ""
          ? session.createdAt
          : typeof session.created_at === "string" && session.created_at.trim() !== ""
            ? session.created_at
          : deriveCreatedAt(messages),
      key,
      messages,
      remoteSessionId,
      title: providedTitle ?? resolveStoredSessionTitle(messages),
      updatedAt:
        typeof session.updatedAt === "string" && session.updatedAt.trim() !== ""
          ? session.updatedAt
          : typeof session.updated_at === "string" && session.updated_at.trim() !== ""
            ? session.updated_at
          : deriveUpdatedAt(messages),
    },
    key,
    messages,
    remoteSessionId,
    title: providedTitle,
    updatedAt:
      typeof session.updatedAt === "string" && session.updatedAt.trim() !== ""
        ? session.updatedAt
        : typeof session.updated_at === "string" && session.updated_at.trim() !== ""
          ? session.updated_at
          : null,
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

function mapGatewaySessionToStoredSession(session: GatewaySession, existing?: StoredChatSession | null) {
  if (!session?.id) return null;

  return buildStoredSession({
    agentName: session.agent ?? existing?.agentName,
    createdAt: session.created_at ?? existing?.createdAt ?? null,
    existing: existing ?? undefined,
    key: existing?.key ?? session.id,
    messages: mapSessionMessages(session, existing?.agentName ?? session.agent ?? null),
    remoteSessionId: session.id,
    title: session.title ?? existing?.title ?? null,
    updatedAt: session.updated_at ?? existing?.updatedAt ?? null,
  });
}

function pickPreferredSessionVersion(
  localSession: StoredChatSession | null,
  remoteSession: StoredChatSession,
  selectedSessionKey: string | null,
  isSending: boolean,
) {
  if (!localSession) {
    return remoteSession;
  }

  const localUpdatedAt = getTimeValue(localSession.updatedAt);
  const remoteUpdatedAt = getTimeValue(remoteSession.updatedAt);
  const shouldKeepLocalDraft =
    isSending &&
    selectedSessionKey === localSession.key &&
    localSession.messages.length >= remoteSession.messages.length &&
    localUpdatedAt >= remoteUpdatedAt;

  if (shouldKeepLocalDraft || localUpdatedAt > remoteUpdatedAt) {
    return {
      ...localSession,
      key: localSession.key,
      remoteSessionId: remoteSession.remoteSessionId,
    };
  }

  return {
    ...remoteSession,
    key: localSession.key,
  };
}

function mergeRemoteSessions(
  state: ChatState,
  agentName: string | null,
  remoteSessions: GatewaySession[],
  isSending: boolean,
): ChatState {
  const remoteSessionIDs = new Set(remoteSessions.map((session) => session.id));
  const localSessionsByRemoteId = new Map(
    state.sessions
      .filter((session) => session.remoteSessionId)
      .map((session) => [session.remoteSessionId!, session]),
  );

  const mergedRemoteSessions = remoteSessions
    .map((session) => {
      const existing = localSessionsByRemoteId.get(session.id) ?? null;
      const mapped = mapGatewaySessionToStoredSession(session, existing);
      if (!mapped) return null;

      return pickPreferredSessionVersion(existing, mapped, state.selectedSessionKey, isSending);
    })
    .filter((session): session is StoredChatSession => session !== null);

  const localOnlySessions = state.sessions.filter((session) => !session.remoteSessionId);
  const retainedRemoteSessions = state.sessions.filter(
    (session) => session.remoteSessionId && !remoteSessionIDs.has(session.remoteSessionId),
  );

  const sessions = sortSessions([...localOnlySessions, ...mergedRemoteSessions, ...retainedRemoteSessions]);
  const keepBlankConversation =
    state.selectedSessionKey === null &&
    state.sessionId === null &&
    state.messages.length === 0 &&
    state.sessions.length > 0;

  if (keepBlankConversation) {
    return {
      ...state,
      sessions,
    };
  }

  const currentSelection =
    getSessionByKey(sessions, state.selectedSessionKey) ??
    findSessionByRemoteId(sessions, state.sessionId) ??
    null;

  if (currentSelection) {
    return {
      messages: currentSelection.messages,
      selectedSessionKey: currentSelection.key,
      sessionId: currentSelection.remoteSessionId,
      sessions,
    };
  }

  const fallbackSession = findLatestSessionForAgent(sessions, agentName);
  if (!fallbackSession) {
    return {
      messages: [],
      selectedSessionKey: null,
      sessionId: null,
      sessions,
    };
  }

  return {
    messages: fallbackSession.messages,
    selectedSessionKey: fallbackSession.key,
    sessionId: fallbackSession.remoteSessionId,
    sessions,
  };
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

function bindRemoteSession(
  state: ChatState,
  agentName: string | null,
  remoteSessionId: string,
  messages: ChatMessage[],
  options?: {
    createdAt?: string | null;
    title?: string | null;
    updatedAt?: string | null;
  },
) {
  if (!remoteSessionId) return state;

  const existingSession = findSessionByRemoteId(state.sessions, remoteSessionId) ?? undefined;
  const storedSession = buildStoredSession({
    agentName,
    createdAt: options?.createdAt ?? existingSession?.createdAt ?? null,
    existing: existingSession,
    key: existingSession?.key ?? remoteSessionId,
    messages,
    remoteSessionId,
    title: options?.title ?? existingSession?.title ?? null,
    updatedAt: options?.updatedAt ?? existingSession?.updatedAt ?? null,
  });

  const sessions = sortSessions(
    state.sessions.filter((session) => session.key !== storedSession.key && session.remoteSessionId !== storedSession.remoteSessionId),
  );

  return {
    messages: storedSession.messages,
    selectedSessionKey: storedSession.key,
    sessionId: storedSession.remoteSessionId,
    sessions: sortSessions([storedSession, ...sessions]),
  };
}

function removeConversation(state: ChatState, agentName: string | null, sessionKey: string): ChatState {
  const sessions = sortSessions(state.sessions.filter((session) => session.key !== sessionKey));
  if (state.selectedSessionKey !== sessionKey) {
    return {
      ...state,
      sessions,
    };
  }

  const fallbackSession = findLatestSessionForAgent(sessions, agentName);
  if (!fallbackSession) {
    return {
      messages: [],
      selectedSessionKey: null,
      sessionId: null,
      sessions,
    };
  }

  return {
    messages: fallbackSession.messages,
    selectedSessionKey: fallbackSession.key,
    sessionId: fallbackSession.remoteSessionId,
    sessions,
  };
}

function removeConversationByRemoteId(state: ChatState, agentName: string | null, remoteSessionId: string) {
  const session = findSessionByRemoteId(state.sessions, remoteSessionId);
  if (!session) return state;
  return removeConversation(state, agentName, session.key);
}

export function useWebChat(agentName: string | null, workspacePath: string | null) {
  const [chatState, setChatState] = useState<ChatState>(() => createInitialChatState(agentName));
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSending, setIsSending] = useState(false);
  const [approvalActionId, setApprovalActionId] = useState<string | null>(null);
  const [allPendingApprovals, setAllPendingApprovals] = useState<ChatApproval[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<ChatApproval[]>([]);
  const [workspaceId, setWorkspaceId] = useState<string | null>(() =>
    looksAbsoluteWorkspacePath(workspacePath) ? deriveWorkspaceId(workspacePath) : null,
  );
  const previousAgentNameRef = useRef<string | null>(normalizeAgentName(agentName));

  const { messages, selectedSessionKey, sessionId, sessions } = chatState;
  const approvalNoticeApprovals = pendingApprovals.length > 0 ? pendingApprovals : allPendingApprovals;

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

    const onDelete = (event: Event) => {
      const detail = (event as CustomEvent<ChatDeleteDetail>).detail;
      if (!detail?.sessionKey) return;
      void deleteSession(detail.sessionKey);
    };

    window.addEventListener(CHAT_RESET_EVENT, onReset);
    window.addEventListener(CHAT_SELECT_EVENT, onSelect as EventListener);
    window.addEventListener(CHAT_DELETE_EVENT, onDelete as EventListener);

    return () => {
      window.removeEventListener(CHAT_RESET_EVENT, onReset);
      window.removeEventListener(CHAT_SELECT_EVENT, onSelect as EventListener);
      window.removeEventListener(CHAT_DELETE_EVENT, onDelete as EventListener);
    };
  }, [sessions, selectedSessionKey, agentName]);

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

  async function fetchSessionsList(activeWorkspaceId: string) {
    return requestJSON<GatewaySession[]>(`/sessions?workspace=${encodeURIComponent(activeWorkspaceId)}`);
  }

  async function fetchSessionSnapshot(activeSessionId: string | null) {
    if (!activeSessionId) return null;
    return requestJSON<GatewaySession>(`/sessions/${encodeURIComponent(activeSessionId)}`);
  }

  async function fetchAllPendingApprovals() {
    return normalizeApprovals(await requestJSON<GatewayApproval[]>("/approvals?status=pending"));
  }

  function applySessionSnapshot(session: GatewaySession | null) {
    if (!session) return;

    const mappedMessages = mapSessionMessages(session, agentName);
    if (mappedMessages.length === 0) return;

    setChatState((current) => {
      if (session.id) {
        return bindRemoteSession(current, agentName, session.id, mappedMessages, {
          createdAt: session.created_at ?? null,
          title: session.title ?? null,
          updatedAt: session.updated_at ?? null,
        });
      }

      return upsertConversation(current, agentName, mappedMessages, current.sessionId);
    });
  }

  function applyApprovalSnapshot(approvals: ChatApproval[], activeSessionId: string | null) {
    setAllPendingApprovals(approvals);
    setPendingApprovals(filterSessionApprovals(approvals, activeSessionId));
  }

  async function focusApprovalSession(targetSessionId: string) {
    let baselineMessageCount = findSessionByRemoteId(sessions, targetSessionId)?.messages.length ?? 0;

    try {
      const session = await fetchSessionSnapshot(targetSessionId);
      if (!session) return baselineMessageCount;

      const mappedMessages = mapSessionMessages(session, agentName);
      baselineMessageCount = mappedMessages.length;

      setChatState((current) =>
        bindRemoteSession(current, agentName, targetSessionId, mappedMessages, {
          createdAt: session.created_at ?? null,
          title: session.title ?? null,
          updatedAt: session.updated_at ?? null,
        }),
      );
      return baselineMessageCount;
    } catch {
      const existingSession = findSessionByRemoteId(sessions, targetSessionId);
      if (existingSession) {
        setChatState((current) => hydrateSession(current, existingSession.key));
      }
      return baselineMessageCount;
    }
  }

  async function syncSessionState(activeSessionId: string | null, options?: { skipSession?: boolean }) {
    if (!activeSessionId) {
      applyApprovalSnapshot(await fetchAllPendingApprovals(), null);
      return { approvals: [] as ChatApproval[], session: null as GatewaySession | null };
    }

    const session = options?.skipSession ? null : await fetchSessionSnapshot(activeSessionId);
    if (session) {
      applySessionSnapshot(session);
    }

    const allApprovals = await fetchAllPendingApprovals();
    const approvals = filterSessionApprovals(allApprovals, session?.id ?? activeSessionId);
    applyApprovalSnapshot(allApprovals, session?.id ?? activeSessionId);

    return { approvals, session };
  }

  useEffect(() => {
    if (typeof window === "undefined" || typeof fetch !== "function") return;

    let cancelled = false;

    async function syncRemoteSessions(forceRefresh = false) {
      try {
        const activeWorkspaceId = await resolveWorkspaceId(forceRefresh);
        if (cancelled) return;

        const remoteSessions = await fetchSessionsList(activeWorkspaceId);
        if (cancelled) return;

        setChatState((current) => mergeRemoteSessions(current, agentName, remoteSessions, isSending));
      } catch {
        // Keep local sessions when the gateway list is temporarily unavailable.
      }
    }

    void syncRemoteSessions();

    const intervalId = window.setInterval(() => {
      void syncRemoteSessions();
    }, REMOTE_SESSION_SYNC_INTERVAL_MS);

    const onFocus = () => {
      void syncRemoteSessions(true);
    };

    const onVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        void syncRemoteSessions(true);
      }
    };

    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisibilityChange);

    return () => {
      cancelled = true;
      window.clearInterval(intervalId);
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, [agentName, isSending, workspaceId, workspacePath]);

  async function waitForApprovalResult(activeSessionId: string, baselineMessageCount: number) {
    let idleStreak = 0;

    for (let attempt = 0; attempt < APPROVAL_POLL_ATTEMPTS; attempt += 1) {
      await sleep(APPROVAL_POLL_INTERVAL_MS);

      try {
        const session = await fetchSessionSnapshot(activeSessionId);
        applySessionSnapshot(session);

        const allApprovals = await fetchAllPendingApprovals();
        const approvals = filterSessionApprovals(allApprovals, session?.id ?? activeSessionId);
        applyApprovalSnapshot(allApprovals, session?.id ?? activeSessionId);

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
          applyApprovalSnapshot([], null);
          setChatState((current) => removeConversationByRemoteId(current, agentName, activeSessionId));
          return;
        }
      }
    }
  }

  async function resolveApproval(approvalId: string, approved: boolean, comment?: string) {
    const activeSessionId = sessionId;
    const targetApproval =
      allPendingApprovals.find((approval) => approval.id === approvalId) ??
      pendingApprovals.find((approval) => approval.id === approvalId) ??
      null;
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
      setAllPendingApprovals((current) => current.filter((approval) => approval.id !== approvalId));
      const targetSessionId = targetApproval?.session_id ?? activeSessionId;
      if (!targetSessionId) return;
      if (approved) {
        const baselineMessageCount =
          targetSessionId === activeSessionId ? messages.length : await focusApprovalSession(targetSessionId);
        await waitForApprovalResult(targetSessionId, baselineMessageCount);
        return;
      }
      await syncSessionState(targetSessionId);
      setError("Approval request was rejected.");
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
    setAllPendingApprovals([]);
    setPendingApprovals([]);
    setChatState((current) => startNewConversation(current));
  }

  function selectSession(sessionKey: string) {
    setDraft("");
    setError(null);
    setApprovalActionId(null);
    setAllPendingApprovals([]);
    setPendingApprovals([]);
    setChatState((current) => hydrateSession(current, sessionKey));
  }

  async function deleteSession(sessionKey: string) {
    const sessionToDelete = getSessionByKey(sessions, sessionKey);
    if (!sessionToDelete) return;

    const deletingSelectedSession = selectedSessionKey === sessionKey;
    setError(null);

    if (sessionToDelete.remoteSessionId) {
      try {
        await requestJSON<{ status: string }>(`/sessions/${encodeURIComponent(sessionToDelete.remoteSessionId)}`, {
          method: "DELETE",
        });
      } catch (deleteError) {
        if (!getErrorMessage(deleteError).includes("session not found")) {
          setError(getErrorMessage(deleteError));
          return;
        }
      }
    }

    if (deletingSelectedSession) {
      setDraft("");
      setApprovalActionId(null);
      setAllPendingApprovals([]);
      setPendingApprovals([]);
    }

    setChatState((current) => removeConversation(current, agentName, sessionKey));
  }

  useEffect(() => {
    let cancelled = false;

    async function syncCurrentSession() {
      if (!sessionId) {
        applyApprovalSnapshot(await fetchAllPendingApprovals(), null);
        return;
      }

      try {
        const session = await fetchSessionSnapshot(sessionId);
        if (cancelled) return;

        applySessionSnapshot(session);

        const allApprovals = await fetchAllPendingApprovals();
        if (cancelled) return;

        applyApprovalSnapshot(allApprovals, session?.id ?? sessionId);
      } catch (syncError) {
        if (cancelled) return;
        if (getErrorMessage(syncError).includes("session not found")) {
          applyApprovalSnapshot([], null);
          setChatState((current) => removeConversationByRemoteId(current, agentName, sessionId));
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
        setChatState((current) => {
          if (response.session?.id) {
            return bindRemoteSession(current, agentName, response.session.id, mappedMessages, {
              createdAt: response.session.created_at ?? null,
              title: response.session.title ?? null,
              updatedAt: response.session.updated_at ?? null,
            });
          }

          return upsertConversation(current, agentName, mappedMessages, resolvedSessionId ?? current.sessionId);
        });
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
      } else if (resolvedSessionId) {
        // A brand-new remote session can enter waiting_approval before the backend
        // has persisted any messages. Bind the remote session id now so approval
        // resolution can poll and resume against the correct session.
        setChatState((current) =>
          upsertConversation(
            current,
            agentName,
            current.messages.length > 0 ? current.messages : [optimisticMessage],
            resolvedSessionId,
          ),
        );
      }

      if (response.status === "waiting_approval") {
        const immediateApprovals = filterSessionApprovals(normalizeApprovals(response.approvals), resolvedSessionId);
        if (immediateApprovals.length > 0) {
          setAllPendingApprovals(immediateApprovals);
          setPendingApprovals(immediateApprovals);
        } else {
          await syncSessionState(resolvedSessionId, { skipSession: true });
        }
      }
      if (response.status === "waiting_approval") {
        setError(null);
      } else {
        await syncSessionState(resolvedSessionId ?? currentSessionId, { skipSession: true });
      }
    } catch (error) {
      setError(getErrorMessage(error));
    } finally {
      setIsSending(false);
    }
  }

  return {
    approvalActionId,
    approvalNoticeApprovals,
    deleteSession,
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
