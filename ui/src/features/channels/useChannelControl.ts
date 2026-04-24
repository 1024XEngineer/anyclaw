import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export type ChannelAdapterStatus = {
  enabled: boolean;
  healthy: boolean;
  last_activity?: string;
  last_error?: string;
  name: string;
  running: boolean;
};

export type MentionGateState = {
  enabled: boolean;
};

export type ChannelPairingRecord = {
  channel: string;
  device_id: string;
  display_name: string;
  expires_at: string;
  last_seen: string;
  paired_at: string;
  user_id: string;
};

export type ChannelPresenceRecord = {
  activity?: string;
  channel: string;
  key: string;
  last_update: string;
  since: string;
  status: string;
  user_id: string;
};

export type ChannelContactRecord = {
  added_at: string;
  channel: string;
  display_name: string;
  first_name: string;
  is_bot: boolean;
  last_name: string;
  last_seen: string;
  metadata?: Record<string, string>;
  user_id: string;
  username: string;
};

type ChannelPairingResponse = {
  enabled: boolean;
  paired: ChannelPairingRecord[];
};

type PairDeviceInput = {
  device_id: string;
  display_name: string;
  ttl_seconds: number;
  user_id: string;
};

function normalizeName(value: string) {
  return value.trim().toLowerCase();
}

function ensureArray<T>(value: T[] | null | undefined) {
  return Array.isArray(value) ? value : [];
}

async function requestJSON<T>(input: string, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });

  const text = await response.text();
  let payload: T | { error?: string } | string | null = null;
  if (text !== "") {
    try {
      payload = JSON.parse(text) as T | { error?: string };
    } catch {
      payload = text;
    }
  }

  if (!response.ok) {
    const message =
      typeof payload === "string" && payload.trim() !== ""
        ? payload.trim()
        : payload && typeof payload === "object" && "error" in payload && typeof payload.error === "string"
        ? payload.error
        : `${response.status} ${response.statusText}`.trim();

    throw new Error(message);
  }

  return payload as T;
}

function mapPresenceMap(
  payload: Record<string, Omit<ChannelPresenceRecord, "channel" | "key" | "user_id">> | null | undefined,
) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return [];
  }

  return Object.entries(payload).map(([key, value]) => {
    const [channel, ...rest] = key.split(":");

    return {
      ...value,
      channel: channel ?? "",
      key,
      user_id: rest.join(":"),
    };
  });
}

export function useChannelControl(selectedSlug: string | null) {
  const queryClient = useQueryClient();

  const adapterStatusesQuery = useQuery({
    queryKey: ["channel-adapter-statuses"],
    queryFn: async () => ensureArray(await requestJSON<ChannelAdapterStatus[] | null>("/channels")),
    initialData: [],
    refetchInterval: 15000,
    retry: 0,
    staleTime: 8000,
  });

  const mentionGateQuery = useQuery({
    queryKey: ["channel-mention-gate"],
    queryFn: () => requestJSON<MentionGateState>("/channel/mention-gate"),
    retry: 0,
    staleTime: 8000,
  });

  const pairingQuery = useQuery({
    queryKey: ["channel-pairing"],
    queryFn: async () => {
      const payload = await requestJSON<ChannelPairingResponse | null>("/channel/pairing");

      return {
        enabled: payload?.enabled ?? false,
        paired: ensureArray(payload?.paired),
      };
    },
    retry: 0,
    staleTime: 8000,
  });

  const presenceQuery = useQuery({
    queryKey: ["channel-presence"],
    queryFn: async () =>
      mapPresenceMap(
        await requestJSON<Record<string, Omit<ChannelPresenceRecord, "channel" | "key" | "user_id">> | null>(
          "/channel/presence",
        ),
      ),
    initialData: [],
    refetchInterval: 15000,
    retry: 0,
    staleTime: 8000,
  });

  const contactsQuery = useQuery({
    queryKey: ["channel-contacts", selectedSlug],
    queryFn: async () =>
      ensureArray(
        await requestJSON<ChannelContactRecord[] | null>(
          `/channel/contacts?channel=${encodeURIComponent(selectedSlug ?? "")}`,
        ),
      ),
    enabled: Boolean(selectedSlug),
    initialData: [],
    refetchInterval: 20000,
    retry: 0,
    staleTime: 8000,
  });

  const toggleMentionGateMutation = useMutation({
    mutationFn: (enabled: boolean) =>
      requestJSON<MentionGateState>("/channel/mention-gate", {
        body: JSON.stringify({ enabled }),
        method: "POST",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["channel-mention-gate"] });
    },
  });

  const pairDeviceMutation = useMutation({
    mutationFn: (input: PairDeviceInput) => {
      if (!selectedSlug) {
        throw new Error("请选择渠道后再新增配对");
      }

      return requestJSON<{ status: string }>("/channel/pairing", {
        body: JSON.stringify({
          action: "pair",
          channel: selectedSlug,
          ...input,
        }),
        method: "POST",
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["channel-pairing"] });
    },
  });

  const unpairDeviceMutation = useMutation({
    mutationFn: (input: { channel: string; device_id: string; user_id: string }) =>
      requestJSON<{ status: string }>("/channel/pairing", {
        body: JSON.stringify({
          action: "unpair",
          channel: input.channel,
          device_id: input.device_id,
          user_id: input.user_id,
        }),
        method: "POST",
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["channel-pairing"] });
    },
  });

  const selectedAdapterStatus =
    adapterStatusesQuery.data.find((status) => normalizeName(status.name) === normalizeName(selectedSlug ?? "")) ??
    null;

  const selectedPairings = (pairingQuery.data?.paired ?? []).filter(
    (record) => normalizeName(record.channel) === normalizeName(selectedSlug ?? ""),
  );

  const selectedPresence = presenceQuery.data.filter(
    (record) => normalizeName(record.channel) === normalizeName(selectedSlug ?? ""),
  );

  return {
    adapterStatusesError: adapterStatusesQuery.error,
    isLoading:
      adapterStatusesQuery.isLoading ||
      mentionGateQuery.isLoading ||
      pairingQuery.isLoading ||
      presenceQuery.isLoading,
    mentionGate: mentionGateQuery.data ?? null,
    mentionGateError: mentionGateQuery.error,
    pairDevice: pairDeviceMutation.mutateAsync,
    pairDeviceError: pairDeviceMutation.error,
    pairDevicePending: pairDeviceMutation.isPending,
    pairingEnabled: pairingQuery.data?.enabled ?? false,
    pairingError: pairingQuery.error,
    presenceError: presenceQuery.error,
    selectedAdapterStatus,
    selectedContacts: contactsQuery.data ?? [],
    selectedPairings,
    selectedPresence,
    toggleMentionGate: toggleMentionGateMutation.mutateAsync,
    toggleMentionGateError: toggleMentionGateMutation.error,
    toggleMentionGatePending: toggleMentionGateMutation.isPending,
    unpairDevice: unpairDeviceMutation.mutateAsync,
    unpairDeviceError: unpairDeviceMutation.error,
    unpairDevicePending: unpairDeviceMutation.isPending,
  };
}
