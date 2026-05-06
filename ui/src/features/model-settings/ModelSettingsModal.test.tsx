import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelSettingsModal } from "@/features/model-settings/ModelSettingsModal";

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

describe("ModelSettingsModal", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("uses user-facing API protocol fields when creating a custom model", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";

      if (url === "/providers" && method === "GET") {
        return jsonResponse([]);
      }

      throw new Error(`Unexpected request: ${method} ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(<ModelSettingsModal onClose={vi.fn()} />);

    await screen.findByText("自定义大模型");

    expect(screen.getByText("接口地址")).toBeInTheDocument();
    expect(screen.getByText("API 协议类型")).toBeInTheDocument();
    expect(screen.getByText("API Key")).toBeInTheDocument();
    expect(screen.getByText("模型名称")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "API 协议类型" })).toHaveTextContent("OpenAI 兼容协议");
    expect(screen.getByRole("combobox", { name: "API 协议类型" })).toHaveTextContent("Anthropic 兼容协议");
    expect(screen.queryByText("通义千问兼容协议")).not.toBeInTheDocument();
    expect(screen.queryByText("运行时")).not.toBeInTheDocument();
    expect(screen.queryByText("Base URL")).not.toBeInTheDocument();
  });

  it("shows a friendly success message for provider connection tests", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";

      if (url === "/providers" && method === "GET") {
        return jsonResponse([]);
      }

      if (url === "/providers/test" && method === "POST") {
        return jsonResponse({
          http_status: 200,
          message: "Endpoint responded with HTTP 200.",
          ok: true,
          status: "reachable",
        });
      }

      throw new Error(`Unexpected request: ${method} ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(<ModelSettingsModal onClose={vi.fn()} />);

    await screen.findByText("自定义大模型");

    fireEvent.change(screen.getByPlaceholderText("请输入模型的显示名称"), {
      target: { value: "我的模型" },
    });
    fireEvent.change(screen.getByPlaceholderText("例如 gpt-4o-mini"), {
      target: { value: "gpt-4o-mini" },
    });
    fireEvent.change(screen.getByPlaceholderText("请输入 API Key"), {
      target: { value: "test-key" },
    });
    fireEvent.click(screen.getByRole("button", { name: /测试连接/i }));

    expect(await screen.findByText("连接成功，接口已响应（HTTP 200）。")).toBeInTheDocument();
    expect(screen.queryByText("Endpoint responded with HTTP 200.")).not.toBeInTheDocument();
  });

  it("keeps provider selection separate from setting the default provider", async () => {
    const providers = [
      {
        default_model: "gpt-5.4",
        enabled: true,
        has_api_key: true,
        id: "provider-a",
        is_default: true,
        name: "Primary Provider",
        provider: "compatible",
      },
      {
        default_model: "claude-sonnet",
        enabled: true,
        has_api_key: true,
        id: "provider-b",
        is_default: false,
        name: "Secondary Provider",
        provider: "compatible",
      },
    ];

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";

      if (url === "/providers" && method === "GET") {
        return jsonResponse(providers);
      }

      if (url === "/providers" && method === "POST") {
        return jsonResponse(providers[1]);
      }

      if (url === "/providers/default" && method === "POST") {
        return jsonResponse({
          ...providers[1],
          is_default: true,
        });
      }

      throw new Error(`Unexpected request: ${method} ${url}`);
    });

    vi.stubGlobal("fetch", fetchMock);

    renderWithClient(<ModelSettingsModal onClose={vi.fn()} />);

    await screen.findByDisplayValue("Primary Provider");

    fireEvent.click(screen.getByRole("button", { name: /Secondary Provider/i }));

    expect(screen.getByDisplayValue("Secondary Provider")).toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(
        ([input]) => String(input) === "/providers/default",
      ),
    ).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: /设为默认模型/i }));

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            String(input) === "/providers/default" &&
            init?.method === "POST" &&
            init.body === JSON.stringify({ provider_ref: "provider-b" }),
        ),
      ).toBe(true);
    });
  });
});
