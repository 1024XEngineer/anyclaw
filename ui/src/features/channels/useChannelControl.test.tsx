import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, renderHook, screen, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useChannelControl } from "./useChannelControl";

function jsonResponse(payload: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : "ERROR",
    text: async () => JSON.stringify(payload),
  } as Response);
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function Probe() {
  const { mentionGate, pairingEnabled, selectedAdapterStatus, selectedContacts, selectedPairings, selectedPresence } =
    useChannelControl("wechat");

  return (
    <div>
      <div data-testid="contacts">{selectedContacts.length}</div>
      <div data-testid="pairings">{selectedPairings.length}</div>
      <div data-testid="presence">{selectedPresence.length}</div>
      <div data-testid="pairing-enabled">{String(pairingEnabled)}</div>
      <div data-testid="mention-gate">{String(mentionGate?.enabled ?? false)}</div>
      <div data-testid="adapter">{selectedAdapterStatus?.name ?? ""}</div>
    </div>
  );
}

describe("useChannelControl", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("normalizes null collection responses into empty arrays", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;

        if (url === "/channels") {
          return jsonResponse(null);
        }
        if (url === "/channel/mention-gate") {
          return jsonResponse({ enabled: true });
        }
        if (url === "/channel/pairing") {
          return jsonResponse({ enabled: true, paired: null });
        }
        if (url === "/channel/presence") {
          return jsonResponse(null);
        }
        if (url.startsWith("/channel/contacts")) {
          return jsonResponse(null);
        }

        throw new Error(`Unhandled fetch request: ${url}`);
      }),
    );

    render(<Probe />, { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByTestId("contacts")).toHaveTextContent("0");
      expect(screen.getByTestId("pairings")).toHaveTextContent("0");
      expect(screen.getByTestId("presence")).toHaveTextContent("0");
      expect(screen.getByTestId("pairing-enabled")).toHaveTextContent("true");
      expect(screen.getByTestId("mention-gate")).toHaveTextContent("true");
      expect(screen.getByTestId("adapter")).toHaveTextContent("");
    });
  });

  it("surfaces plain-text server errors instead of a JSON parse failure", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn((input: string | URL | Request) => {
        const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;

        if (url === "/channels") {
          return Promise.resolve({
            ok: false,
            status: 503,
            statusText: "Service Unavailable",
            text: async () => "channels unavailable",
          } as Response);
        }
        if (url === "/channel/mention-gate") {
          return jsonResponse({ enabled: false });
        }
        if (url === "/channel/pairing") {
          return jsonResponse({ enabled: false, paired: [] });
        }
        if (url === "/channel/presence") {
          return jsonResponse({});
        }
        if (url.startsWith("/channel/contacts")) {
          return jsonResponse([]);
        }

        throw new Error(`Unhandled fetch request: ${url}`);
      }),
    );

    const { result } = renderHook(() => useChannelControl("wechat"), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.selectedAdapterStatus).toBeNull();
    });
  });
});
