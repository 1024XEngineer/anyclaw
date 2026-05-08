import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

import { useMarketDirectory } from "./useMarketDirectory";

vi.mock("@/features/workspace/useWorkspaceOverview", () => ({
  useWorkspaceOverview: vi.fn(),
}));

const useWorkspaceOverviewMock = vi.mocked(useWorkspaceOverview);

function createWrapper(initialEntries: string[]) {
  return function Wrapper({ children }: PropsWithChildren) {
    return <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>;
  };
}

describe("useMarketDirectory", () => {
  beforeEach(() => {
    useWorkspaceOverviewMock.mockReturnValue({
      data: {
        cloudRoadmap: [
          { status: "规划中", summary: "Agent catalog", title: "云端 Agent Catalog" },
          { status: "预留位", summary: "Skill hub", title: "云端 Skill Hub" },
          { status: "规划中", summary: "Unified center", title: "统一连接中心" },
        ],
        localAgents: [
          {
            name: "Main Agent",
            permissionLevel: "limited",
            providerName: "Primary Provider",
            role: "主 Agent",
            skillsCount: 4,
            status: "当前启用",
            summary: "负责默认入口",
            workingDir: "D:/workspace/main",
          },
          {
            name: "Reviewer",
            permissionLevel: "system",
            providerName: "Primary Provider",
            role: "协同评审",
            skillsCount: 2,
            status: "已配置",
            summary: "负责评审与复核",
            workingDir: "D:/workspace/reviewer",
          },
        ],
        localSkills: [
          {
            description: "处理提交规划",
            installCommand: "anyclaw skill install plan",
            loaded: true,
            name: "plan-pr",
            registry: "official",
            source: "local",
            version: "1.0.0",
          },
          {
            description: "整理聊天记录",
            installCommand: "",
            loaded: false,
            name: "chat-log",
            registry: "",
            source: "workspace",
            version: "",
          },
        ],
      },
    } as never);
  });

  it("defaults to the local agent directory and selects the first entry", () => {
    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market"]),
    });

    expect(result.current.kind).toBe("agent");
    expect(result.current.source).toBe("local");
    expect(result.current.localEntries.map((entry) => entry.name)).toEqual(["Main Agent", "Reviewer"]);
    expect(result.current.selectedEntry?.id).toBe("agent:Main Agent");
    expect(result.current.cloudPanels.map((panel) => panel.title)).toEqual([
      "云端 Agent Catalog",
      "统一连接中心",
    ]);
  });

  it("filters local entries by the search query and falls back to the first visible selection", () => {
    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?selected=agent:Reviewer"]),
    });

    expect(result.current.selectedEntry?.id).toBe("agent:Reviewer");

    act(() => {
      result.current.setQuery("main");
    });

    expect(result.current.localEntries.map((entry) => entry.name)).toEqual(["Main Agent"]);
    expect(result.current.selectedEntry?.id).toBe("agent:Main Agent");
  });

  it("supports the cloud skill view without selecting a local entry", () => {
    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?kind=skill&source=cloud"]),
    });

    expect(result.current.kind).toBe("skill");
    expect(result.current.source).toBe("cloud");
    expect(result.current.selectedEntry).toBeNull();
    expect(result.current.cloudPanels.map((panel) => panel.title)).toEqual([
      "云端 Skill Hub",
      "统一连接中心",
    ]);
  });
});
