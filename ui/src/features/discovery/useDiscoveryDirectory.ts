import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export type DiscoveryInstance = {
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
};

type DiscoveryInstancesResponse = {
  instances?: DiscoveryInstance[];
};

type DiscoveryScanResponse = {
  status?: string;
};

async function readJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.headers ?? {}),
    },
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(text || response.statusText || "Request failed");
  }

  return (text ? JSON.parse(text) : {}) as T;
}

async function loadDiscoveryInstances() {
  const payload = await readJSON<DiscoveryInstancesResponse>("/discovery/instances");
  const instances = Array.isArray(payload.instances) ? payload.instances : [];

  return instances
    .map((instance) => ({
      address: instance.address ?? "",
      capabilities: Array.isArray(instance.capabilities) ? instance.capabilities : [],
      host: instance.host ?? "",
      id: instance.id ?? "",
      is_self: Boolean(instance.is_self),
      last_seen: instance.last_seen ?? "",
      metadata: instance.metadata ?? {},
      name: instance.name ?? "",
      port: Number.isFinite(instance.port) ? instance.port : 0,
      url: instance.url ?? "",
      version: instance.version ?? "",
    }))
    .sort((left, right) => {
      if (left.is_self !== right.is_self) {
        return left.is_self ? -1 : 1;
      }

      const rightSeen = new Date(right.last_seen).getTime();
      const leftSeen = new Date(left.last_seen).getTime();
      if (!Number.isNaN(leftSeen) && !Number.isNaN(rightSeen) && leftSeen !== rightSeen) {
        return rightSeen - leftSeen;
      }

      return (left.name || left.id).localeCompare(right.name || right.id);
    });
}

async function scanDiscoveryNetwork() {
  const payload = await readJSON<DiscoveryScanResponse>("/discovery/query", {
    method: "POST",
  });

  return payload.status?.trim() || "query sent";
}

export function useDiscoveryDirectory() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["discovery", "instances"],
    queryFn: loadDiscoveryInstances,
    placeholderData: [] as DiscoveryInstance[],
    refetchInterval: 10000,
    staleTime: 4000,
  });

  const scan = useMutation({
    mutationFn: scanDiscoveryNetwork,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["discovery", "instances"] });
      window.setTimeout(() => {
        void queryClient.invalidateQueries({ queryKey: ["discovery", "instances"] });
      }, 2000);
    },
  });

  const instances = query.data ?? [];
  const selfInstance = instances.find((instance) => instance.is_self) ?? null;
  const peerInstances = instances.filter((instance) => !instance.is_self);

  return {
    errorMessage: query.error instanceof Error ? query.error.message : "",
    instances,
    isFetching: query.isFetching,
    isLoading: query.isLoading,
    isScanning: scan.isPending,
    lastUpdatedAt: query.dataUpdatedAt,
    peerInstances,
    refetch: query.refetch,
    scanNetwork: scan.mutateAsync,
    scanStatus: scan.data ?? "",
    selfInstance,
  };
}
