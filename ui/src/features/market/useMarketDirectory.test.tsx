import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { Mock } from "vitest";
import type { PropsWithChildren } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { requestJSON } from "@/features/api/client";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

import { useMarketDirectory } from "./useMarketDirectory";

vi.mock("@/features/api/client", () => ({
  requestJSON: vi.fn(),
}));

vi.mock("@/features/workspace/useWorkspaceOverview", () => ({
  useWorkspaceOverview: vi.fn(),
}));

const requestJSONMock = vi.mocked(requestJSON);
const useWorkspaceOverviewMock = vi.mocked(useWorkspaceOverview);

type MockArtifact = {
  description?: string;
  id: string;
  kind: "agent" | "skill" | "cli";
  name: string;
  owner?: string;
  permissions?: string[];
  source?: "local" | "cloud";
  source_id?: string;
  status?: string;
  version?: string;
};

function createWrapper(initialEntries: string[]) {
  return function Wrapper({ children }: PropsWithChildren) {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
        },
      },
    });

    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>{children}</MemoryRouter>
      </QueryClientProvider>
    );
  };
}

function marketResponse(items: MockArtifact[]) {
  return {
    data: {
      items,
      total: items.length,
    },
  };
}

function detailResponse(item: MockArtifact | undefined) {
  return {
    data: item ?? null,
  };
}

function setupMarketAPI(initialItems: MockArtifact[], queriedItems = initialItems) {
  requestJSONMock.mockImplementation((async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);

    if (url.startsWith("/market/artifacts?")) {
      return url.includes("q=main") ? marketResponse(queriedItems) : marketResponse(initialItems);
    }

    if (url.includes("/versions")) {
      return {
        data: {
          items: [{ version: "1.0.0", size_bytes: 1200 }],
          total: 1,
        },
      };
    }

    if (url.startsWith("/market/artifacts/")) {
      const id = decodeURIComponent(url.replace("/market/artifacts/", "").split("?")[0] ?? "");
      return detailResponse([...initialItems, ...queriedItems].find((item) => item.id === id));
    }

    if (url === "/market/bindings" && init?.method === "POST") {
      return {
        data: {
          artifact_id: initialItems[0]?.id ?? "agent:Main Agent",
          created_at: "2026-05-07T00:00:00Z",
          id: "binding-1",
          kind: initialItems[0]?.kind ?? "agent",
          state: "enabled",
          target_id: "main",
          target_type: "main_agent",
          version: "1.0.0",
        },
      };
    }

    if (url === "/market/bindings") {
      return { data: { items: [], total: 0 } };
    }

    if (url === "/market/install") {
      return {
        job_id: "job-1",
        job: {
          artifact_id: initialItems[0]?.id ?? "cloud.skill.writer",
          id: "job-1",
          progress_index: 1,
          progress_step: "Resolve",
          progress_total: 5,
          state: "running",
        },
      };
    }

    if (url === "/market/upgrade") {
      return {
        job_id: "job-upgrade-1",
        job: {
          artifact_id: initialItems[0]?.id ?? "cloud.skill.writer",
          id: "job-upgrade-1",
          progress_index: 1,
          progress_step: "Resolve",
          progress_total: 5,
          state: "running",
        },
      };
    }

    if (url === "/market/uninstall") {
      return {
        data: {
          artifact_id: initialItems[0]?.id ?? "cloud.skill.writer",
          previous_version: "1.0.0",
          receipt_id: "cloud.skill.writer@1.0.0",
          removed_bindings: ["binding-1"],
          uninstalled_at: "2026-05-07T00:00:00Z",
        },
      };
    }

    if (url === "/market/jobs/job-1") {
      return {
        data: {
          artifact_id: initialItems[0]?.id ?? "cloud.skill.writer",
          id: "job-1",
          progress_index: 5,
          progress_step: "Receipt",
          progress_total: 5,
          state: "succeeded",
        },
      };
    }

    if (url === "/market/jobs/job-upgrade-1") {
      return {
        data: {
          artifact_id: initialItems[0]?.id ?? "cloud.skill.writer",
          id: "job-upgrade-1",
          progress_index: 5,
          progress_step: "Receipt",
          progress_total: 5,
          state: "succeeded",
        },
      };
    }

    return { data: { items: [] } };
  }) as Mock);
}

describe("useMarketDirectory", () => {
  beforeEach(() => {
    requestJSONMock.mockReset();
    useWorkspaceOverviewMock.mockReturnValue({
      data: {
        cloudRoadmap: [
          { status: "已上线", summary: "Marketplace Operator", title: "Marketplace Operator" },
          { status: "已上线", summary: "Skill Author", title: "Skill Author" },
          { status: "已上线", summary: "Agent Native Runner", title: "Agent Native Runner" },
        ],
      },
    } as never);
  });

  it("loads the local agent directory from the marketplace API and selects the first entry", async () => {
    setupMarketAPI([
      {
        description: "Main entry",
        id: "agent:Main Agent",
        kind: "agent",
        name: "Main Agent",
        owner: "Primary Provider",
        permissions: ["limited"],
        source: "local",
        source_id: "agent.profiles",
        status: "active",
      },
      {
        description: "Review work",
        id: "agent:Reviewer",
        kind: "agent",
        name: "Reviewer",
        source: "local",
        source_id: "agent.profiles",
        status: "bound",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?source=local"]),
    });

    await waitFor(() => expect(result.current.localEntries).toHaveLength(2));

    expect(result.current.kind).toBe("agent");
    expect(result.current.source).toBe("local");
    expect(result.current.localEntries.map((entry) => entry.name)).toEqual(["Main Agent", "Reviewer"]);
    expect(result.current.selectedEntry?.id).toBe("agent:Main Agent");
    expect(requestJSONMock).toHaveBeenCalledWith("/market/artifacts?kind=agent&source=local&limit=100");
  });

  it("defaults to the live cloud catalog instead of roadmap placeholders", async () => {
    setupMarketAPI([
      {
        description: "Runs marketplace releases.",
        id: "anyclaw.agent.marketplace-operator",
        kind: "agent",
        name: "Marketplace Operator",
        source: "cloud",
        status: "available",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market"]),
    });

    await waitFor(() => expect(result.current.selectedEntry?.id).toBe("anyclaw.agent.marketplace-operator"));

    expect(result.current.source).toBe("cloud");
    expect(result.current.cloudPanels).toEqual([]);
    expect(requestJSONMock).toHaveBeenCalledWith("/market/artifacts?kind=agent&source=cloud&limit=100");
    expect(requestJSONMock).toHaveBeenCalledWith("/market/artifacts/anyclaw.agent.marketplace-operator?source=cloud");
    expect(requestJSONMock).toHaveBeenCalledWith("/market/artifacts/anyclaw.agent.marketplace-operator/versions?source=cloud");
  });

  it("updates the query string and reloads matching entries", async () => {
    setupMarketAPI(
      [
        { description: "Main entry", id: "agent:Main Agent", kind: "agent", name: "Main Agent", source: "local" },
        { description: "Review work", id: "agent:Reviewer", kind: "agent", name: "Reviewer", source: "local" },
      ],
      [{ description: "Main entry", id: "agent:Main Agent", kind: "agent", name: "Main Agent", source: "local" }],
    );

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?source=local&selected=agent:Reviewer"]),
    });

    await waitFor(() => expect(result.current.selectedEntry?.id).toBe("agent:Reviewer"));

    act(() => {
      result.current.setQuery("main");
    });

    await waitFor(() => expect(result.current.localEntries.map((entry) => entry.name)).toEqual(["Main Agent"]));
    expect(result.current.selectedEntry?.id).toBe("agent:Main Agent");
    expect(requestJSONMock).toHaveBeenCalledWith("/market/artifacts?kind=agent&source=local&limit=100&q=main");
  });

  it("passes combined discovery filters to the marketplace API", async () => {
    setupMarketAPI([
      {
        description: "Creates marketplace-ready skills.",
        id: "anyclaw.skill.skill-author",
        kind: "skill",
        name: "Skill Author",
        owner: "AnyClaw Labs",
        permissions: ["fs.read", "fs.write"],
        source: "cloud",
        status: "available",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper([
        "/market?kind=skill&source=cloud&q=skill&risk=low&trust=verified&tag=marketplace&permission=fs.read&publisher=AnyClaw%20Labs&os=windows&arch=amd64&sort=name",
      ]),
    });

    await waitFor(() => expect(result.current.selectedEntry?.id).toBe("anyclaw.skill.skill-author"));

    expect(requestJSONMock).toHaveBeenCalledWith(
      "/market/artifacts?kind=skill&source=cloud&limit=100&q=skill&risk=low&trust=verified&tag=marketplace&permission=fs.read&publisher=AnyClaw+Labs&os=windows&arch=amd64&sort=name",
    );
  });

  it("supports the cloud skill view without local placeholders when cloud data is returned", async () => {
    setupMarketAPI([
      {
        description: "Cloud skill",
        id: "cloud.skill.writer",
        kind: "skill",
        name: "writer",
        source: "cloud",
        status: "available",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?kind=skill&source=cloud"]),
    });

    await waitFor(() => expect(result.current.localEntries).toHaveLength(1));

    expect(result.current.kind).toBe("skill");
    expect(result.current.source).toBe("cloud");
    expect(result.current.selectedEntry?.id).toBe("cloud.skill.writer");
    expect(result.current.cloudPanels).toEqual([]);
  });

  it("starts a cloud install job with an idempotency key", async () => {
    setupMarketAPI([
      {
        description: "Cloud skill",
        id: "cloud.skill.writer",
        kind: "skill",
        name: "writer",
        source: "cloud",
        status: "available",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?kind=skill&source=cloud"]),
    });

    await waitFor(() => expect(result.current.canInstall).toBe(true));

    act(() => {
      result.current.installArtifact();
    });

    await waitFor(() => expect(result.current.installJob?.id).toBe("job-1"));
    expect(requestJSONMock).toHaveBeenCalledWith(
      "/market/install",
      expect.objectContaining({
        body: JSON.stringify({ artifact_id: "cloud.skill.writer", risk_acknowledged: true, user_confirmed: true }),
        headers: expect.objectContaining({ "Idempotency-Key": expect.stringContaining("ui-cloud.skill.writer-") }),
        method: "POST",
      }),
    );
  });

  it("binds an installed artifact to a selected target", async () => {
    setupMarketAPI([
      {
        description: "Cloud skill",
        id: "cloud.skill.writer",
        kind: "skill",
        name: "writer",
        source: "cloud",
        status: "installed",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?kind=skill&source=cloud"]),
    });

    await waitFor(() => expect(result.current.canBind).toBe(true));

    act(() => {
      result.current.bindArtifact("main_agent");
    });

    await waitFor(() =>
      expect(requestJSONMock).toHaveBeenCalledWith(
        "/market/bindings",
        expect.objectContaining({
          body: JSON.stringify({ artifact_id: "cloud.skill.writer", target_type: "main_agent" }),
          method: "POST",
        }),
      ),
    );
  });

  it("starts an upgrade job for an installed artifact", async () => {
    setupMarketAPI([
      {
        description: "Cloud skill",
        id: "cloud.skill.writer",
        kind: "skill",
        name: "writer",
        source: "cloud",
        status: "installed",
        version: "1.0.0",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?kind=skill&source=cloud"]),
    });

    await waitFor(() => expect(result.current.canBind).toBe(true));

    act(() => {
      result.current.upgradeArtifact("2.0.0");
    });

    await waitFor(() => expect(result.current.installJob?.id).toBe("job-upgrade-1"));
    expect(requestJSONMock).toHaveBeenCalledWith(
      "/market/upgrade",
      expect.objectContaining({
        body: JSON.stringify({ artifact_id: "cloud.skill.writer", risk_acknowledged: true, user_confirmed: true, version_constraint: "2.0.0" }),
        headers: expect.objectContaining({ "Idempotency-Key": expect.stringContaining("ui-upgrade-cloud.skill.writer-2.0.0-") }),
        method: "POST",
      }),
    );
  });

  it("uninstalls an installed artifact", async () => {
    setupMarketAPI([
      {
        description: "Cloud skill",
        id: "cloud.skill.writer",
        kind: "skill",
        name: "writer",
        source: "cloud",
        status: "installed",
      },
    ]);

    const { result } = renderHook(() => useMarketDirectory(), {
      wrapper: createWrapper(["/market?kind=skill&source=cloud"]),
    });

    await waitFor(() => expect(result.current.canBind).toBe(true));

    act(() => {
      result.current.uninstallArtifact();
    });

    await waitFor(() =>
      expect(requestJSONMock).toHaveBeenCalledWith(
        "/market/uninstall",
        expect.objectContaining({
          body: JSON.stringify({ artifact_id: "cloud.skill.writer" }),
          method: "POST",
        }),
      ),
    );
  });
});
