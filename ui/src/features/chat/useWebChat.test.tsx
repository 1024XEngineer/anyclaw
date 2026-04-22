import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CHAT_STORAGE_KEY, useWebChat } from "./useWebChat";

const TEST_WORKSPACE_PATH = "D:\\workspace\\anyclaw\\workflows";

type HookProbeProps = {
  agentName?: string;
};

function HookProbe({ agentName = "binbin" }: HookProbeProps) {
  const { deleteSession, error, messages, resetConversation, selectSession, selectedSessionKey, sessionId } = useWebChat(
    agentName,
    TEST_WORKSPACE_PATH,
  );

  return (
    <div>
      <div data-testid="error-message">{error ?? ""}</div>
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
      <button
        onClick={() => {
          if (selectedSessionKey) {
            void deleteSession(selectedSessionKey);
          }
        }}
        type="button"
      >
        delete-selected
      </button>
    </div>
  );
}

function ApprovalProbe() {
  const {
    approvalNoticeApprovals,
    draft,
    messages,
    pendingApprovals,
    resolveApproval,
    sessionId,
    sendMessage,
    setDraft,
  } = useWebChat("binbin", TEST_WORKSPACE_PATH);

  return (
    <div>
      <input aria-label="draft" onChange={(event) => setDraft(event.target.value)} value={draft} />
      <button onClick={sendMessage} type="button">
        send
      </button>
      <div data-testid="approval-count">{approvalNoticeApprovals.length}</div>
      <div data-testid="approval-tool">{approvalNoticeApprovals[0]?.tool_name ?? ""}</div>
      <div data-testid="session-approval-count">{pendingApprovals.length}</div>
      <div data-testid="message-count">{messages.length}</div>
      <div data-testid="message-preview">{messages[messages.length - 1]?.content ?? ""}</div>
      <div data-testid="session-id">{sessionId ?? ""}</div>
      <button
        disabled={approvalNoticeApprovals.length === 0}
        onClick={() => void resolveApproval(approvalNoticeApprovals[0]?.id ?? "", true)}
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

function requestURL(input: RequestInfo | URL) {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.toString();
  return input.url;
}

describe("useWebChat persistence", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();

    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestURL(input);

      if (url === "/approvals?status=pending") {
        return jsonResponse([]);
      }

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([]);
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);
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
          content: "hello",
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
                content: "older session",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_1",
            title: "older session",
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
      expect(persisted.sessions[0].title).toBe("older session");
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
                content: "first session",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_1",
            title: "first session",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
          {
            agentName: "binbin",
            createdAt: "2026-04-11T13:00:00.000Z",
            key: "session-2",
            messages: [
              {
                content: "second session",
                role: "user",
                timestamp: "2026-04-11T13:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_2",
            title: "second session",
            updatedAt: "2026-04-11T13:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    render(<HookProbe />);

    fireEvent.click(screen.getByRole("button", { name: "select-second" }));

    await waitFor(() => {
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("session-2");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_2");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("second session");
    });
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
                content: "other agent",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_other",
            title: "other agent",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
          {
            agentName: "binbin",
            createdAt: "2026-04-11T13:00:00.000Z",
            key: "agent-binbin",
            messages: [
              {
                content: "current agent",
                role: "user",
                timestamp: "2026-04-11T13:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_binbin",
            title: "current agent",
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
      expect(screen.getByTestId("message-preview")).toHaveTextContent("current agent");
    });
  });

  it("hydrates remote sessions from the gateway list so browser and desktop history can sync", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestURL(input);

      if (url === "/approvals?status=pending") {
        return jsonResponse([]);
      }

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([
          {
            agent: "binbin",
            created_at: "2026-04-11T14:00:00.000Z",
            id: "sess_remote",
            messages: [
              {
                content: "from desktop",
                created_at: "2026-04-11T14:00:00.000Z",
                role: "user",
              },
            ],
            title: "from desktop",
            updated_at: "2026-04-11T14:00:00.000Z",
          },
        ]);
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<HookProbe />);

    await waitFor(() => {
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("sess_remote");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_remote");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("from desktop");
    });
  });

  it("keeps the active remote session when the session list temporarily misses it", async () => {
    window.localStorage.setItem(
      CHAT_STORAGE_KEY,
      JSON.stringify({
        selectedSessionKey: "session-keep",
        sessions: [
          {
            agentName: "binbin",
            createdAt: "2026-04-11T15:00:00.000Z",
            key: "session-keep",
            messages: [
              {
                content: "still here",
                role: "user",
                timestamp: "2026-04-11T15:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_keep",
            title: "still here",
            updatedAt: "2026-04-11T15:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestURL(input);

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([]);
      }

      if (url === "/sessions/sess_keep") {
        return jsonResponse({
          agent: "binbin",
          id: "sess_keep",
          messages: [
            {
              content: "still here",
              created_at: "2026-04-11T15:00:00.000Z",
              role: "user",
            },
          ],
          presence: "waiting_approval",
          typing: false,
        });
      }

      if (url === "/approvals?status=pending") {
        return jsonResponse([]);
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<HookProbe />);

    await waitFor(() => {
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("session-keep");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_keep");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("still here");
    });
  });

  it("retains previously synced remote sessions when a poll returns a partial list", async () => {
    window.localStorage.setItem(
      CHAT_STORAGE_KEY,
      JSON.stringify({
        selectedSessionKey: "session-newer",
        sessions: [
          {
            agentName: "binbin",
            createdAt: "2026-04-11T15:00:00.000Z",
            key: "session-newer",
            messages: [
              {
                content: "newer session",
                role: "user",
                timestamp: "2026-04-11T15:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_newer",
            title: "newer session",
            updatedAt: "2026-04-11T15:00:00.000Z",
          },
          {
            agentName: "binbin",
            createdAt: "2026-04-11T14:00:00.000Z",
            key: "session-older",
            messages: [
              {
                content: "older remote session",
                role: "user",
                timestamp: "2026-04-11T14:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_older",
            title: "older remote session",
            updatedAt: "2026-04-11T14:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = requestURL(input);

      if (url === "/approvals?status=pending") {
        return jsonResponse([]);
      }

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([
          {
            agent: "binbin",
            created_at: "2026-04-11T15:00:00.000Z",
            id: "sess_newer",
            messages: [
              {
                content: "newer session",
                created_at: "2026-04-11T15:00:00.000Z",
                role: "user",
              },
            ],
            title: "newer session",
            updated_at: "2026-04-11T15:00:00.000Z",
          },
        ]);
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<HookProbe />);

    await waitFor(() => {
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("session-newer");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_newer");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("newer session");
    });

    await waitFor(() => {
      const persisted = JSON.parse(window.localStorage.getItem(CHAT_STORAGE_KEY) || "null");
      expect(persisted.sessions).toHaveLength(2);
      expect(persisted.sessions.map((session: { remoteSessionId: string | null }) => session.remoteSessionId)).toEqual(
        expect.arrayContaining(["sess_newer", "sess_older"]),
      );
    });
  });

  it("deletes synced sessions through the main sessions api", async () => {
    let deleted = false;

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
                content: "delete me",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_delete",
            title: "delete me",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);

      if (url === "/approvals?status=pending") {
        return jsonResponse([]);
      }

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse(
          deleted
            ? []
            : [
                {
                  agent: "binbin",
                  created_at: "2026-04-11T12:00:00.000Z",
                  id: "sess_delete",
                  messages: [
                    {
                      content: "delete me",
                      created_at: "2026-04-11T12:00:00.000Z",
                      role: "user",
                    },
                  ],
                  title: "delete me",
                  updated_at: "2026-04-11T12:00:00.000Z",
                },
              ],
        );
      }

      if (url === "/sessions/sess_delete") {
        expect(init?.method).toBe("DELETE");
        deleted = true;
        return jsonResponse({ status: "deleted" });
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<HookProbe />);

    fireEvent.click(screen.getByRole("button", { name: "delete-selected" }));

    await waitFor(() => {
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("");
      expect(screen.getByTestId("session-id")).toHaveTextContent("");

      const persisted = JSON.parse(window.localStorage.getItem(CHAT_STORAGE_KEY) || "null");
      expect(persisted.sessions).toHaveLength(0);
    });
  });

  it("shows plain-text delete errors instead of a JSON parse crash", async () => {
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
                content: "delete me",
                role: "user",
                timestamp: "2026-04-11T12:00:00.000Z",
              },
            ],
            remoteSessionId: "sess_delete_text",
            title: "delete me",
            updatedAt: "2026-04-11T12:00:00.000Z",
          },
        ],
        version: 2,
      }),
    );

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([
          {
            agent: "binbin",
            created_at: "2026-04-11T12:00:00.000Z",
            id: "sess_delete_text",
            messages: [
              {
                content: "delete me",
                created_at: "2026-04-11T12:00:00.000Z",
                role: "user",
              },
            ],
            title: "delete me",
            updated_at: "2026-04-11T12:00:00.000Z",
          },
        ]);
      }

      if (url === "/sessions/sess_delete_text") {
        expect(init?.method).toBe("DELETE");
        return Promise.resolve({
          ok: false,
          status: 405,
          statusText: "Method Not Allowed",
          text: async () => "method not allowed",
        } as Response);
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<HookProbe />);

    fireEvent.click(screen.getByRole("button", { name: "delete-selected" }));

    await waitFor(() => {
      expect(screen.getByTestId("error-message")).toHaveTextContent("method not allowed");
      expect(screen.getByTestId("selected-session-key")).toHaveTextContent("session-1");
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
        content: "please create a folder",
        created_at: "2026-04-11T12:00:00.000Z",
        role: "user",
      },
    ];

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([
          {
            agent: "binbin",
            created_at: "2026-04-11T12:00:00.000Z",
            id: "sess_approval",
            messages: sessionMessages,
            title: "please create a folder",
            updated_at: approvalPending ? "2026-04-11T12:00:00.000Z" : "2026-04-11T12:00:20.000Z",
          },
        ]);
      }

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
            content: "folder created",
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
      target: { value: "please create a folder" },
    });

    await waitFor(() => {
      expect(screen.getByLabelText("draft")).toHaveValue("please create a folder");
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
      expect(screen.getByTestId("message-preview")).toHaveTextContent("folder created");
    }, { timeout: 4000 });
  }, 10000);

  it("keeps the resumed assistant message when the remote session list is briefly stale", async () => {
    const approval = {
      action: "tool_call",
      id: "approval_stale_list",
      payload: {
        args: {
          command: "mkdir C:\\Users\\TestUser\\Desktop\\Hello",
        },
      },
      requested_at: "2026-04-11T12:30:10.000Z",
      session_id: "sess_approval_stale_list",
      status: "pending",
      tool_name: "run_command",
    };

    const createdAt = "2026-04-11T12:30:00.000Z";
    const staleUpdatedAt = "2026-04-11T12:30:00.000Z";
    const resumedUpdatedAt = "2026-04-11T12:30:20.000Z";
    let approvalPending = true;
    let sessionMessages = [
      {
        content: "please create a folder",
        created_at: createdAt,
        role: "user",
      },
    ];

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse(
          approvalPending
            ? []
            : [
                {
                  agent: "binbin",
                  created_at: createdAt,
                  id: "sess_approval_stale_list",
                  messages: [
                    {
                      content: "please create a folder",
                      created_at: createdAt,
                      role: "user",
                    },
                  ],
                  title: "please create a folder",
                  updated_at: staleUpdatedAt,
                },
              ],
        );
      }

      if (url.startsWith("/chat?workspace=")) {
        return jsonResponse({
          approvals: [approval],
          session: {
            agent: "binbin",
            created_at: createdAt,
            id: "sess_approval_stale_list",
            messages: sessionMessages,
            presence: "waiting_approval",
            title: "please create a folder",
            typing: false,
            updated_at: staleUpdatedAt,
          },
          status: "waiting_approval",
        });
      }

      if (url === "/approvals?status=pending") {
        return jsonResponse(approvalPending ? [approval] : []);
      }

      if (url === "/approvals/approval_stale_list/resolve") {
        expect(init?.method).toBe("POST");
        approvalPending = false;
        sessionMessages = [
          ...sessionMessages,
          {
            content: "folder created",
            created_at: resumedUpdatedAt,
            role: "assistant",
          },
        ];

        return jsonResponse({
          ...approval,
          status: "approved",
        });
      }

      if (url === "/sessions/sess_approval_stale_list") {
        return jsonResponse({
          agent: "binbin",
          created_at: createdAt,
          id: "sess_approval_stale_list",
          messages: sessionMessages,
          presence: approvalPending ? "waiting_approval" : "idle",
          title: "please create a folder",
          typing: false,
          updated_at: approvalPending ? staleUpdatedAt : resumedUpdatedAt,
        });
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<ApprovalProbe />);

    fireEvent.change(screen.getByLabelText("draft"), {
      target: { value: "please create a folder" },
    });

    fireEvent.click(screen.getByRole("button", { name: "send" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("1");
    });

    fireEvent.click(screen.getByRole("button", { name: "approve" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("0");
      expect(screen.getByTestId("message-count")).toHaveTextContent("2");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("folder created");
    }, { timeout: 4000 });

    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 4500));
    });

    await waitFor(() => {
      expect(screen.getByTestId("message-count")).toHaveTextContent("2");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("folder created");
    });
  }, 15000);

  it("binds a new remote session before approval resume when the waiting response has no messages yet", async () => {
    const approval = {
      action: "tool_call",
      id: "approval_new_session",
      payload: {
        args: {
          path: "Hello.md",
        },
      },
      requested_at: "2026-04-11T12:10:10.000Z",
      session_id: "sess_new_approval",
      status: "pending",
      tool_name: "write_file",
    };

    let approvalPending = true;
    let sessionMessages: Array<{ content: string; created_at: string; role: "assistant" | "user" }> = [];

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse(
          approvalPending
            ? []
            : [
                {
                  agent: "binbin",
                  created_at: "2026-04-11T12:10:00.000Z",
                  id: "sess_new_approval",
                  messages: sessionMessages,
                  title: "please create a markdown file",
                  updated_at: "2026-04-11T12:10:20.000Z",
                },
              ],
        );
      }

      if (url.startsWith("/chat?workspace=")) {
        return jsonResponse({
          approvals: [approval],
          session: {
            agent: "binbin",
            id: "sess_new_approval",
            messages: [],
            presence: "waiting_approval",
            typing: false,
          },
          status: "waiting_approval",
        });
      }

      if (url === "/approvals?status=pending") {
        return jsonResponse(approvalPending ? [approval] : []);
      }

      if (url === "/approvals/approval_new_session/resolve") {
        expect(init?.method).toBe("POST");
        approvalPending = false;
        sessionMessages = [
          {
            content: "please create a markdown file",
            created_at: "2026-04-11T12:10:00.000Z",
            role: "user",
          },
          {
            content: "created Hello.md",
            created_at: "2026-04-11T12:10:20.000Z",
            role: "assistant",
          },
        ];
        return jsonResponse({
          ...approval,
          status: "approved",
        });
      }

      if (url === "/sessions/sess_new_approval") {
        return jsonResponse({
          agent: "binbin",
          id: "sess_new_approval",
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
      target: { value: "please create a markdown file" },
    });

    fireEvent.click(screen.getByRole("button", { name: "send" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("1");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_new_approval");
      expect(screen.getByTestId("message-count")).toHaveTextContent("1");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("please create a markdown file");
    });

    fireEvent.click(screen.getByRole("button", { name: "approve" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("0");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_new_approval");
      expect(screen.getByTestId("message-count")).toHaveTextContent("2");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("created Hello.md");
    }, { timeout: 4000 });
  }, 10000);

  it("shows a pending approval even when the current session is not selected", async () => {
    const approval = {
      action: "tool_call",
      id: "approval_orphaned_view",
      payload: {
        args: {
          path: "C:\\Users\\TestUser\\Desktop\\Hello.md",
        },
      },
      requested_at: "2026-04-11T12:20:10.000Z",
      session_id: "sess_orphaned_view",
      status: "pending",
      tool_name: "write_file",
    };

    let approvalPending = true;
    let sessionMessages: Array<{ content: string; created_at: string; role: "assistant" | "user" }> = [
      {
        content: "please create Hello.md",
        created_at: "2026-04-11T12:20:00.000Z",
        role: "user",
      },
    ];

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = requestURL(input);

      if (url.startsWith("/sessions?workspace=")) {
        return jsonResponse([]);
      }

      if (url === "/approvals?status=pending") {
        return jsonResponse(approvalPending ? [approval] : []);
      }

      if (url === "/approvals/approval_orphaned_view/resolve") {
        expect(init?.method).toBe("POST");
        approvalPending = false;
        sessionMessages = [
          ...sessionMessages,
          {
            content: "created Hello.md",
            created_at: "2026-04-11T12:20:20.000Z",
            role: "assistant",
          },
        ];
        return jsonResponse({
          ...approval,
          status: "approved",
        });
      }

      if (url === "/sessions/sess_orphaned_view") {
        return jsonResponse({
          agent: "binbin",
          created_at: "2026-04-11T12:20:00.000Z",
          id: "sess_orphaned_view",
          messages: sessionMessages,
          presence: approvalPending ? "waiting_approval" : "idle",
          title: "please create Hello.md",
          typing: false,
          updated_at: approvalPending ? "2026-04-11T12:20:00.000Z" : "2026-04-11T12:20:20.000Z",
        });
      }

      throw new Error(`Unexpected fetch: ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    render(<ApprovalProbe />);

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("1");
      expect(screen.getByTestId("session-approval-count")).toHaveTextContent("0");
      expect(screen.getByTestId("session-id")).toHaveTextContent("");
    });

    fireEvent.click(screen.getByRole("button", { name: "approve" }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-count")).toHaveTextContent("0");
      expect(screen.getByTestId("session-id")).toHaveTextContent("sess_orphaned_view");
      expect(screen.getByTestId("message-count")).toHaveTextContent("2");
      expect(screen.getByTestId("message-preview")).toHaveTextContent("created Hello.md");
    }, { timeout: 4000 });
  }, 10000);
});
