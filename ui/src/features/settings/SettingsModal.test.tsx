import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AgentDrawer } from "@/features/agent-drawer/AgentDrawer";
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
      configSource: "example",
      generatedAt: "2026-04-22T08:00:00.000Z",
      liveConnected: true,
      sourceLabel: "live",
    },
    priorityChannels: [],
    providers: [],
    runtimeProfile: {
      address: "http://127.0.0.1:18789",
      configStatus: "ok",
      description: "workspace",
      events: 0,
      gatewayOnline: true,
      gatewaySource: "live",
      language: "zh-CN",
      mainPermission: "limited",
      mainPermissionSource: "config",
      model: "gpt-5.4",
      name: "workspace",
      orchestrator: "default",
      permission: "limited",
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
      workspaceId: "ws-test",
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

  return render(
    <MemoryRouter>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </MemoryRouter>,
  );
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

  it("reflects the live main permission instead of falling back to the snapshot default", () => {
    mockWorkspaceOverview({
      ...createOverview(),
      runtimeProfile: {
        ...createOverview().runtimeProfile,
        mainPermission: "full",
      },
    });

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    const select = screen.getByRole("combobox", { name: "主 Agent 权限级别" });
    expect(select).toHaveValue("full");
  });

  it("uses the same main permission in settings and the agent drawer", () => {
    const overview = {
      ...createOverview(),
      runtimeProfile: {
        ...createOverview().runtimeProfile,
        mainPermission: "read-only",
        permission: "limited",
      },
    };
    mockWorkspaceOverview(overview);

    const settingsView = renderWithClient(<SettingsModal onClose={vi.fn()} />);
    const select = screen.getByRole("combobox", { name: "主 Agent 权限级别" });
    expect(select).toHaveValue("read-only");
    settingsView.unmount();

    renderWithClient(<AgentDrawer onClose={vi.fn()} />);
    expect(screen.getByText("read-only")).toBeInTheDocument();
    expect(screen.queryByText("limited")).not.toBeInTheDocument();
  });

  it("updates the main agent permission from general settings", async () => {
    const fetchMock = vi.fn(() => jsonResponse({ agent: { permission_level: "full" } }));
    vi.stubGlobal("fetch", fetchMock);

    mockWorkspaceOverview(createOverview());

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    const select = screen.getByRole("combobox", { name: "主 Agent 权限级别" });
    expect(select).toHaveValue("limited");
    expect(screen.getByText("主 Agent 权限会作为运行时上限，SubAgent 的最终权限不会超过这里的设置。")).toBeInTheDocument();

    fireEvent.change(select, { target: { value: "full" } });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/config",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ agent: { permission_level: "full" } }),
        }),
      );
    });
  });

  it("disables the main agent permission selector while saving", async () => {
    const deferred = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferred.promise);
    vi.stubGlobal("fetch", fetchMock);

    mockWorkspaceOverview(createOverview());

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    const select = screen.getByRole("combobox", { name: "主 Agent 权限级别" });
    fireEvent.change(select, { target: { value: "read-only" } });

    await waitFor(() => {
      expect(select).toBeDisabled();
    });

    deferred.resolve(jsonResponse({ agent: { permission_level: "read-only" } }));

    await waitFor(() => {
      expect(select).not.toBeDisabled();
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

    act(() => {
      useShellStore.setState({ settingsSection: "agents" });
    });

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    expect(screen.getByText("当前仅展示状态")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Agent One 状态" })).not.toBeInTheDocument();
    expect(screen.getByText("后端已启用")).toBeInTheDocument();
  });

  it("links skill and agent management to the market page", () => {
    mockWorkspaceOverview(createOverview());

    act(() => {
      useShellStore.setState({ settingsSection: "skills" });
    });

    const onClose = vi.fn();
    const view = renderWithClient(<SettingsModal onClose={onClose} />);

    const skillLink = screen.getByRole("link", { name: /添加 Skill/i });
    expect(skillLink).toHaveAttribute("href", "/market?kind=skill");

    fireEvent.click(skillLink);
    expect(onClose).toHaveBeenCalledTimes(1);

    act(() => {
      useShellStore.setState({ settingsSection: "agents" });
    });
    view.unmount();
    renderWithClient(<SettingsModal onClose={onClose} />);

    const agentLink = screen.getByRole("link", { name: /添加 Agent/i });
    expect(agentLink).toHaveAttribute("href", "/market");
  });

  it("disables all skill toggles while one skill toggle is saving", async () => {
    const deferred = createDeferred<Response>();
    const fetchMock = vi.fn(() => deferred.promise);
    vi.stubGlobal("fetch", fetchMock);

    mockWorkspaceOverview({
      ...createOverview(),
      localSkills: [
        {
          description: "First skill",
          enabled: true,
          installCommand: "",
          loaded: true,
          name: "alpha",
          registry: "local",
          source: "local",
          version: "1.0.0",
        },
        {
          description: "Second skill",
          enabled: false,
          installCommand: "",
          loaded: false,
          name: "beta",
          registry: "local",
          source: "local",
          version: "1.0.0",
        },
      ],
    });

    act(() => {
      useShellStore.setState({ settingsSection: "skills" });
    });

    renderWithClient(<SettingsModal onClose={vi.fn()} />);

    const alphaToggle = screen.getByRole("button", { name: /alpha/i });
    const betaToggle = screen.getByRole("button", { name: /beta/i });
    fireEvent.click(alphaToggle);

    await waitFor(() => {
      expect(alphaToggle).toBeDisabled();
      expect(betaToggle).toBeDisabled();
    });

    deferred.resolve(jsonResponse({ enabled: false, loaded: false, name: "alpha" }));

    await waitFor(() => {
      expect(alphaToggle).not.toBeDisabled();
      expect(betaToggle).not.toBeDisabled();
    });
  });
});
