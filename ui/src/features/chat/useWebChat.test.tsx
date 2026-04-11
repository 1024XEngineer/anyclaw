import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CHAT_STORAGE_KEY, useWebChat } from "./useWebChat";

const TEST_WORKSPACE_PATH = "D:\\workspace\\anyclaw\\workflows";

type HookProbeProps = {
  agentName?: string;
};

function HookProbe({ agentName = "binbin" }: HookProbeProps) {
  const { messages, resetConversation, selectSession, selectedSessionKey, sessionId } = useWebChat(
    agentName,
    TEST_WORKSPACE_PATH,
  );

  return (
    <div>
      <div data-testid="message-count">{messages.length}</div>
      <div data-testid="message-preview">{messages[0]?.content ?? ""}</div>
      <div data-testid="selected-session-key">{selectedSessionKey ?? ""}</div>
      <div data-testid="session-id">{sessionId ?? ""}</div>
      <button onClick={resetConversation} type="button">
        reset
      </button>
      <button onClick={() => selectSession("session-2")} type="button">
        select-second
      </button>
    </div>
  );
}

function ApprovalProbe() {
  const {
    draft,
    messages,
    pendingApprovals,
    resolveApproval,
    sendMessage,
    setDraft,
  } = useWebChat("binbin", TEST_WORKSPACE_PATH);

  return (
    <div>
      <input aria-label="draft" onChange={(event) => setDraft(event.target.value)} value={draft} />
      <button onClick={sendMessage} type="button">
        send
      </button>
      <div data-testid="approval-count">{pendingApprovals.length}</div>
      <div data-testid="approval-tool">{pendingApprovals[0]?.tool_name ?? ""}</div>
      <div data-testid="message-count">{messages.length}</div>
      <div data-testid="message-preview">{messages[messages.length - 1]?.content ?? ""}</div>
      <button
        disabled={pendingApprovals.length === 0}
        onClick={() => void resolveApproval(pendingApprovals[0]?.id ?? "", true)}
        type="button"
      >
        approve
      </button>
    </div>
  );
}

function jsonResponse(payload: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : "ERROR",
    text: async () => JSON.stringify(payload),
  } as Response);
}

describe("useWebChat persistence", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("migrates legacy single-session storage into multi-session local storage", async () => {
    const legacyState = {
      messages: [
        {
          content: "你好",
          role: "user" as const,
          timestamp: "2026-04-11T12:00:00.000Z",
        },
      ],
      sessionId: "sess_123",
    };

    window.sessionStorage.setItem(CHAT_STORAGE_KEY, JSON.stringify(legacyState));

    render(<HookProbe />);

    expect(screen.getByTestId("message-count")).toHaveTextContent("1");
    expect(screen.getByTestId("session-id")).toHaveTextContent("sess_123");

    await waitFor(() => {
      const persisted = JSON.parse(window.localStorage.getItem(CHAT_STORAGE_KEY) || "null");

      expect(persisted.version).toBe(2);
      expect(persisted.selectedSessionKey).toBe("sess_123");
      expect(persisted.sessions).toHaveLength(1);
      expect(persisted.sessions[0].remoteSessionId).toBe("sess_123");
      expect(window.sessionStorage.getItem(CHAT_STORAGE_KEY)).toBeNull();
    });
  });

  it("keeps previous sessions in history when starting a new conversation", async () => {
    window.localStorage.setItem(
      CHAT_STORAGE_KEY,
      JSON.stringify({
        selectedSessionKey: "session-1",
        sessions: [
          {
            agentName: "binbin",
            createdAt: "2026-04-11T12:00:00.000Z",
            key: "session-1",
            messages: [
              {
                content: "旧会话",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_1",
            title: "旧会话",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    render(<HookProbe />);

    fireEvent.click(screen.getByRole("button", { name: "reset" }));

    expect(screen.getByTestId("message-count")).toHaveTextContent("0");
    expect(screen.getByTestId("selected-session-key")).toHaveTextContent("");

    await waitFor(() => {
      const persisted = JSON.parse(window.localStorage.getItem(CHAT_STORAGE_KEY) || "null");

      expect(persisted.selectedSessionKey).toBeNull();
      expect(persisted.sessions).toHaveLength(1);
      expect(persisted.sessions[0].title).toBe("旧会话");
    });
  });

  it("can switch to another stored conversation", async () => {
    window.localStorage.setItem(
      CHAT_STORAGE_KEY,
      JSON.stringify({
        selectedSessionKey: "session-1",
        sessions: [
          {
            agentName: "binbin",
            createdAt: "2026-04-11T12:00:00.000Z",
            key: "session-1",
            messages: [
              {
                content: "第一条会话",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_1",
            title: "第一条会话",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
          {
            agentName: "binbin",
            createdAt: "2026-04-11T13:00:00.000Z",
            key: "session-2",
            messages: [
              {
                content: "第二条会话",
                role: "user",
                timestamp: "2026-04-11T13:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_2",
            title: "第二条会话",
            updatedAt: "2026-04-11T13:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    render(<HookProbe />);

    fireEvent.click(screen.getByRole("button", { name: "select-second" }));

    expect(screen.getByTestId("selected-session-key")).toHaveTextContent("session-2");
    expect(screen.getByTestId("session-id")).toHaveTextContent("sess_2");
    expect(screen.getByTestId("message-preview")).toHaveTextContent("第二条会话");
  });

  it("prefers the current agent's latest session on first load", async () => {
    window.localStorage.setItem(
      CHAT_STORAGE_KEY,
      JSON.stringify({
        selectedSessionKey: "agent-other",
        sessions: [
          {
            agentName: "other-agent",
            createdAt: "2026-04-11T12:00:00.000Z",
            key: "agent-other",
            messages: [
              {
                content: "其他 agent",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_other",
            title: "其他 agent",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
          {
            agentName: "binbin",
            createdAt: "2026-04-11T13:00:00.000Z",
            key: "agent-binbin",
            messages: [
              {
                content: "当前 agent",
                role: "user",
                timestamp: "2026-04-11T13:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_binbin",
            title: "当前 agent",
            updatedAt: "2026-04-11T13:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    render(<HookProbe agentName="binbin" />);

    await waitFor(() => {
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("agent-binbin");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_binbin");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("当前 agent");
    });
  });

  it("shows pending approvals and resumes the chat after approval", async () => {
    const approval = {
      action: "tool_call",
      id: "approval_1",
      payload: {
        args: {
          command: "mkdir C:\\Users\\TestUser\\Desktop\\Hello",
        },
      },
      requested_at: "2026-04-11T12:00:10.000Z",
      session_id: "sess_approval",
      status: "pending",
      tool_name: "run_command",
    };

    let approvalPending = true;
    let sessionMessages = [
      {
        content: "请创建桌面文件夹",
        created_at: "2026-04-11T12:00:00.000Z",
        role: "user",
      },
    ];

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;

      if (url.startsWith("/chat?workspace=")) {
        return jsonResponse({
          approvals: [approval],
          session: {
            agent: "binbin",
            id: "sess_approval",
            messages: sessionMessages,
            presence: "waiting_approval",
            typing: false,
          },
          status: "waiting_approval",
        });
      }

      if (url === "/approvals?status=pending") {
        return jsonResponse(approvalPending ? [approval] : []);
      }

      if (url === "/approvals/approval_1/resolve") {
        expect(init?.method).toBe("POST");
        approvalPending = false;
        sessionMessages = [
          ...sessionMessages,
          {
            content: "已创建桌面文件夹。",
            created_at: "2026-04-11T12:00:20.000Z",
            role: "assistant",
          },
        ];

        return jsonResponse({
          ...approval,
          status: "approved",
        });
      }

      if (url === "/sessions/sess_approval") {
        return jsonResponse({
          agent: "binbin",
          id: "sess_approval",
          messages: sessionMessages,
          presence: approvalPending ? "waiting_approval" : "idle",
          typing: false,
        });
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<ApprovalProbe />);

    fireEvent.change(screen.getByLabelText("draft"), {
      target: { value: "请创建桌面文件夹" },
    });
    fireEvent.click(screen.getByRole("button", { name: "send" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("1");
      expect(screen.getByTestId("approval-tool")).toHaveTextContent("run_command");
    });

    fireEvent.click(screen.getByRole("button", { name: "approve" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("0");
      expect(screen.getByTestId("message-count")).toHaveTextContent("2");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("已创建桌面文件夹。");
    }, { timeout: 4000 });
  }, 10000);
});
