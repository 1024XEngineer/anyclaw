import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useDiscoveryDirectory } from "./useDiscoveryDirectory";

function jsonResponse(payload: unknown) {
  return new Response(JSON.stringify(payload), {
    headers: { "Content-Type": "application/json" },
    status: 200,
  });
}

function createWrapper() {
  const client = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });

  return function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

describe("useDiscoveryDirectory", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("separates the current node from peers and sorts peers by last seen time", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) !== "/discovery/instances") {
        throw new Error(`Unexpected request: ${String(input)}`);
      }

      return jsonResponse({
        instances: [
          {
            address: "10.0.0.20:18789",
            capabilities: ["chat"],
            host: "10.0.0.20",
            id: "peer-older",
            is_self: false,
            last_seen: "2026-04-22T06:00:00Z",
            metadata: {},
            name: "Peer Older",
            port: 18789,
            url: "http://10.0.0.20:18789",
            version: "1.0.0",
          },
          {
            address: "127.0.0.1:18789",
            capabilities: ["chat", "skills"],
            host: "127.0.0.1",
            id: "self-node",
            is_self: true,
            last_seen: "2026-04-22T08:00:00Z",
            metadata: { scope: "local" },
            name: "Local Node",
            port: 18789,
            url: "http://127.0.0.1:18789",
            version: "1.0.0",
          },
          {
            address: "10.0.0.30:18789",
            capabilities: ["market"],
            host: "10.0.0.30",
            id: "peer-newer",
            is_self: false,
            last_seen: "2026-04-22T07:30:00Z",
            metadata: {},
            name: "Peer Newer",
            port: 18789,
            url: "http://10.0.0.30:18789",
            version: "1.1.0",
          },
        ],
      });
    });

    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useDiscoveryDirectory(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.selfInstance?.id).toBe("self-node");
    });

    expect(result.current.peerInstances.map((instance) => instance.id)).toEqual([
      "peer-newer",
      "peer-older",
    ]);
  });

  it("posts a scan request and exposes the returned status", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";

      if (url === "/discovery/instances" && method === "GET") {
        return jsonResponse({ instances: [] });
      }

      if (url === "/discovery/query" && method === "POST") {
        return jsonResponse({ status: "discovery not enabled" });
      }

      throw new Error(`Unexpected request: ${method} ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useDiscoveryDirectory(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    await act(async () => {
      await result.current.scanNetwork();
    });

    await waitFor(() => {
      expect(result.current.scanStatus).toBe("discovery not enabled");
    });

    expect(
      fetchMock.mock.calls.some(
        ([input, init]) => String(input) === "/discovery/query" && (init?.method ?? "GET") === "POST",
      ),
    ).toBe(true);
  });
});
