import { fireEvent, render } from "@testing-library/react";
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
        },
      },
    } as unknown as ReturnType<typeof useWorkspaceOverview>);

    webChatState = {
      approvalActionId: null,
      approvalNoticeApprovals: [],
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
});
