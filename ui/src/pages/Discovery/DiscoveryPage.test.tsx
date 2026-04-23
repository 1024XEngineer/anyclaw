import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useDiscoveryDirectory } from "@/features/discovery/useDiscoveryDirectory";

import { DiscoveryPage } from "./DiscoveryPage";

vi.mock("@/features/discovery/useDiscoveryDirectory", () => ({
  useDiscoveryDirectory: vi.fn(),
}));

const useDiscoveryDirectoryMock = vi.mocked(useDiscoveryDirectory);

function createInstance(overrides: Partial<ReturnType<typeof buildInstance>> = {}) {
  return buildInstance(overrides);
}

function buildInstance(overrides: Partial<{
  address: string;
  capabilities: string[];
  host: string;
  id: string;
  is_self: boolean;
  last_seen: string;
  metadata: Record<string, string>;
  name: string;
  port: number;
  url: string;
  version: string;
}> = {}) {
  return {
    address: "10.0.0.20:18789",
    capabilities: ["chat"],
    host: "10.0.0.20",
    id: "peer-node",
    is_self: false,
    last_seen: "2026-04-22T07:30:00Z",
    metadata: {},
    name: "Peer Node",
    port: 18789,
    url: "https://node.example:18789",
    version: "1.0.0",
    ...overrides,
  };
}

function mockDiscoveryDirectory(peerInstances: Array<ReturnType<typeof buildInstance>>) {
  useDiscoveryDirectoryMock.mockReturnValue({
    errorMessage: "",
    instances: peerInstances,
    isFetching: false,
    isLoading: false,
    isScanning: false,
    lastUpdatedAt: 0,
    peerInstances,
    refetch: vi.fn(),
    scanNetwork: vi.fn(),
    scanStatus: "",
    selfInstance: null,
  });
}

describe("DiscoveryPage", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("does not render external actions for unsafe discovery urls", () => {
    mockDiscoveryDirectory([
      createInstance({
        address: "10.0.0.21:18789",
        id: "unsafe-node",
        name: "Unsafe Node",
        url: "javascript:alert(1)",
      }),
    ]);

    render(<DiscoveryPage />);

    expect(screen.queryByRole("button", { name: "复制地址" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "打开节点" })).not.toBeInTheDocument();
  });

  it("keeps external actions for http and https discovery urls", () => {
    mockDiscoveryDirectory([
      createInstance({
        id: "safe-node",
        name: "Safe Node",
        url: "https://safe-node.example:18789",
      }),
    ]);

    render(<DiscoveryPage />);

    expect(screen.getByRole("button", { name: "复制地址" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开节点" })).toHaveAttribute(
      "href",
      "https://safe-node.example:18789/",
    );
  });
});
