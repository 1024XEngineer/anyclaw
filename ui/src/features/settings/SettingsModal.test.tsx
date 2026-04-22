import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SettingsModal } from "@/features/settings/SettingsModal";
import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview, type WorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

vi.mock("@/features/workspace/useWorkspaceOverview", () => ({
  useWorkspaceOverview: vi.fn(),
}));

function createOverview(): WorkspaceOverview {
  return {
    appearanceSettings: [],
    channelSettings: [],
    cloudRoadmap: [],
    extensionAdapters: [],
    localAgents: [],
    localSkills: [],
    meta: {
      generatedAt: "2026-04-22T08:00:00.000Z",
      liveConnected: true,
      sourceLabel: "live",
    },
    priorityChannels: [],
    providers: [],
    runtimeProfile: {
      address: "http://127.0.0.1:18789",
      description: "workspace",
      events: 0,
      gatewayOnline: true,
      gatewaySource: "live",
      language: "zh-CN",
      model: "gpt-5.4",
      name: "workspace",
      orchestrator: "default",
      permission: "workspace-write",
      provider: "openai",
      providerLabel: "OpenAI",
      providersCount: 1,
      runtimes: 1,
      secured: true,
      sessions: 0,
      skills: 0,
      startedAt: "2026-04-22T08:00:00.000Z",
      title: "workspace",
      tools: 0,
      workDir: "D:/anyclaw",
      workspace: "D:/anyclaw",
    },
    runtimeSettings: [],
  };
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });

  return { promise, reject, resolve };
}

function jsonResponse(payload: unknown) {
  return new Response(JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    status: 200,
  });
}

function renderWithClient(node: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });

  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>);
}

function mockWorkspaceOverview(data: WorkspaceOverview) {
  vi.mocked(useWorkspaceOverview).mockReturnValue({
    data,
  } as ReturnType<typeof useWorkspaceOverview>);
}

describe("SettingsModal", () => {
  const mockedUseWorkspaceOverview = vi.mocked(useWorkspaceOverview);

  beforeEach(() => {
    useShellStore.setState({
      agentDrawerOpen: false,
      modelSettingsOpen: false,
      settingsOpen: true,
      settingsSection: "general",
    });
    mockedUseWorkspaceOverview.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("disables skill toggles while a skill update is in flight", async () => {
    const deferred = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferred.promise);
    vi.stubGlobal("fetch", fetchMock);

    mockWorkspaceOverview({
      ...createOverview(),
      localSkills: [
        {
          description: "first skill",
          enabled: true,
          installCommand: "",
          loaded: true,
          name: "skill-one",
          registry: "local",
          source: "local",
          version: "1.0.0",
        },
        {
          description: "second skill",
          enabled: false,
          installCommand: "",
          loaded: false,
          name: "skill-two",
          registry: "local",
          source: "local",
          version: "1.0.0",
        },
      ],
    });

    useShellStore.setState({ settingsSection: "skills" });

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    const firstSwitch = screen.getByRole("button", { name: "skill-one 状态" });
    const secondSwitch = screen.getByRole("button", { name: "skill-two 状态" });

    fireEvent.click(firstSwitch);

    await waitFor(() => {
      expect(firstSwitch).toBeDisabled();
      expect(secondSwitch).toBeDisabled();
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);

    deferred.resolve(
      jsonResponse({
        enabled: false,
        loaded: false,
        name: "skill-one",
      }),
    );

    await waitFor(() => {
      expect(firstSwitch).not.toBeDisabled();
      expect(secondSwitch).not.toBeDisabled();
    });
  });

  it("shows agent status as read-only instead of an interactive toggle", () => {
    mockWorkspaceOverview({
      ...createOverview(),
      localAgents: [
        {
          active: true,
          model: "gpt-5.4",
          name: "Agent One",
          permissionLevel: "workspace-write",
          providerName: "OpenAI",
          role: "assistant",
          skillsCount: 2,
          status: "运行中",
          summary: "agent summary",
          tags: [],
          workingDir: "D:/anyclaw",
        },
      ],
    });

    useShellStore.setState({ settingsSection: "agents" });

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    expect(screen.getByText("当前仅展示状态")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Agent One 状态" })).not.toBeInTheDocument();
    expect(screen.getByText("后端已启用")).toBeInTheDocument();
  });
});
