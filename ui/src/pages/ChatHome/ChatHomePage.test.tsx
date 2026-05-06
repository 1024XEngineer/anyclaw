import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";

import { useWebChat } from "@/features/chat/useWebChat";
import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

import { ChatHomePage } from "./ChatHomePage";

vi.mock("@/features/chat/useWebChat", () => ({
  useWebChat: vi.fn(),
}));

vi.mock("@/features/workspace/useWorkspaceOverview", () => ({
  useWorkspaceOverview: vi.fn(),
}));

const useWebChatMock = vi.mocked(useWebChat);
const useWorkspaceOverviewMock = vi.mocked(useWorkspaceOverview);
let webChatState: ReturnType<typeof useWebChat>;

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <ChatHomePage />
    </MemoryRouter>,
  );
}

describe("ChatHomePage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useShellStore.setState({
      agentDrawerOpen: false,
      modelSettingsOpen: false,
      settingsOpen: false,
      settingsSection: "general",
    });

    useWorkspaceOverviewMock.mockReturnValue({
      data: {
        localAgents: [
          {
            active: true,
            model: "gpt-5",
            name: "binbin",
          },
        ],
        localSkills: [],
        providers: [
          {
            enabled: true,
            health: "healthy",
            isDefault: true,
            model: "gpt-5",
            name: "OpenAI",
          },
        ],
        runtimeProfile: {
          model: "gpt-5",
          name: "AnyClaw",
          sessions: 2,
          workspace: "/workspace",
          workspaceId: "ws-test-snapshot",
        },
      },
    } as unknown as ReturnType<typeof useWorkspaceOverview>);

    webChatState = {
      approvalActionId: null,
      approvalNoticeApprovals: [],
      chatTaskState: {
        canCancel: false,
        canContinue: true,
        canRetry: false,
        detail: "上一轮任务已经结束，可以继续追问或开始新任务。",
        label: "已完成",
        phase: "completed",
      },
      deleteSession: vi.fn(),
      draft: "",
      error: null,
      isSending: false,
      messages: [
        {
          content: "first message",
          role: "assistant",
          timestamp: "2026-04-11T12:00:00.000Z",
        },
      ],
      pendingApprovals: [],
      resetConversation: vi.fn(),
      resolveApproval: vi.fn(),
      selectSession: vi.fn(),
      selectedSessionKey: "session-1",
      sendMessage: vi.fn(),
      sessionId: "sess_1",
      sessions: [],
      setDraft: vi.fn(),
    } as unknown as ReturnType<typeof useWebChat>;

    useWebChatMock.mockImplementation(() => webChatState);

    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: vi.fn(),
      writable: true,
    });
  });

  it("does not force auto-scroll when the user has scrolled away from the bottom", () => {
    const { container, rerender } = renderPage();
    const viewport = container.querySelector(".chat-scroll-area") as HTMLDivElement;
    const scrollTo = vi.fn();

    Object.defineProperties(viewport, {
      clientHeight: {
        configurable: true,
        value: 400,
      },
      scrollHeight: {
        configurable: true,
        value: 1000,
      },
      scrollTop: {
        configurable: true,
        value: 600,
        writable: true,
      },
    });
    viewport.scrollTo = scrollTo;

    fireEvent.scroll(viewport);
    scrollTo.mockClear();

    viewport.scrollTop = 120;
    fireEvent.scroll(viewport);

    webChatState = {
      ...webChatState,
      messages: [
        {
          content: "first message",
          role: "assistant",
          timestamp: "2026-04-11T12:00:00.000Z",
        },
        {
          content: "background update",
          role: "assistant",
          timestamp: "2026-04-11T12:00:05.000Z",
        },
      ],
    };

    rerender(
      <MemoryRouter initialEntries={["/"]}>
        <ChatHomePage />
      </MemoryRouter>,
    );

    expect(scrollTo).not.toHaveBeenCalled();
  });

  it("keeps auto-scroll when a new send starts", () => {
    const { container, rerender } = renderPage();
    const viewport = container.querySelector(".chat-scroll-area") as HTMLDivElement;
    const scrollTo = vi.fn();

    Object.defineProperties(viewport, {
      clientHeight: {
        configurable: true,
        value: 400,
      },
      scrollHeight: {
        configurable: true,
        value: 1000,
      },
      scrollTop: {
        configurable: true,
        value: 100,
        writable: true,
      },
    });
    viewport.scrollTo = scrollTo;

    fireEvent.scroll(viewport);
    scrollTo.mockClear();

    webChatState = {
      ...webChatState,
      isSending: true,
    };

    rerender(
      <MemoryRouter initialEntries={["/"]}>
        <ChatHomePage />
      </MemoryRouter>,
    );

    expect(scrollTo).toHaveBeenCalledWith({
      behavior: "smooth",
      top: 1000,
    });
  });

  it("passes the snapshot workspace id to chat persistence", () => {
    renderPage();

    expect(useWebChatMock).toHaveBeenCalledWith("binbin", "/workspace", "ws-test-snapshot");
  });

  it("shows task entry suggestions when the conversation is empty", () => {
    webChatState = {
      ...webChatState,
      chatTaskState: {
        canCancel: false,
        canContinue: true,
        canRetry: false,
        detail: "可以直接描述你想完成的任务。",
        label: "待命",
        phase: "idle",
      },
      messages: [],
      sessionId: null,
    };

    renderPage();

    expect(screen.getByText("想让 AnyClaw 帮你完成什么？")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /排查一个问题/i }));

    expect(webChatState.setDraft).toHaveBeenCalledWith("帮我检查最近的报错，定位原因，并给出可以落地的修复方案。");
  });

  it("renders approvals as an execution preview", () => {
    webChatState = {
      ...webChatState,
      approvalNoticeApprovals: [
        {
          action: "tool_call",
          id: "approval-1",
          payload: {
            args: {
              command: "pnpm test",
            },
          },
          requested_at: "2026-04-11T12:00:10.000Z",
          status: "pending",
          tool_name: "run_command",
        },
      ],
      chatTaskState: {
        canCancel: false,
        canContinue: true,
        canRetry: false,
        detail: "1 个操作需要你确认后才会继续。",
        label: "等待确认",
        phase: "awaiting_approval",
      },
      pendingApprovals: [
        {
          id: "approval-1",
          payload: { args: { command: "pnpm test" } },
          tool_name: "run_command",
        },
      ],
    };

    renderPage();

    expect(screen.getByText("执行前预览")).toBeInTheDocument();
    expect(screen.getByText("运行本地命令")).toBeInTheDocument();
    expect(screen.getByText("高影响")).toBeInTheDocument();
    expect(screen.getByText("命令: pnpm test")).toBeInTheDocument();
  });

  it("shows user-facing error guidance in the composer", () => {
    webChatState = {
      ...webChatState,
      chatTaskState: {
        canCancel: false,
        canContinue: false,
        canRetry: true,
        detail: "这次请求没有完成，你可以调整后重试。",
        label: "可重试",
        phase: "retryable",
        technicalDetail: "session not found",
      },
      error: "session not found",
    };

    renderPage();

    expect(screen.getByText("发生了什么：这个会话已经不可用。")).toBeInTheDocument();
    expect(screen.getByText("你可以怎么做：可以新建对话，或从左侧选择另一个历史会话后继续。")).toBeInTheDocument();
  });
});
