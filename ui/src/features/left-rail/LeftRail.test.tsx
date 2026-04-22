import { fireEvent, render, screen } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

import { LeftRail } from "./LeftRail";

vi.mock("@/features/workspace/useWorkspaceOverview", () => ({
  useWorkspaceOverview: vi.fn(),
}));

const useWorkspaceOverviewMock = vi.mocked(useWorkspaceOverview);

function Wrapper({ children }: PropsWithChildren) {
  return <MemoryRouter initialEntries={["/market"]}>{children}</MemoryRouter>;
}

describe("LeftRail", () => {
  beforeEach(() => {
    useShellStore.setState({
      agentDrawerOpen: false,
      modelSettingsOpen: false,
      settingsOpen: false,
      settingsSection: "general",
    });

    useWorkspaceOverviewMock.mockReturnValue({
      data: {
        runtimeProfile: {
          name: "Builder",
        },
      },
    } as never);
  });

  it("highlights the active route and shows the workspace avatar letter", () => {
    render(<LeftRail />, { wrapper: Wrapper });

    expect(screen.getByRole("link", { name: "市场" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "总览" })).not.toHaveAttribute("aria-current", "page");
    expect(screen.getByText("B")).toBeInTheDocument();
  });

  it("opens general settings from the footer action", () => {
    render(<LeftRail />, { wrapper: Wrapper });

    fireEvent.click(screen.getByRole("button", { name: "打开设置" }));

    expect(useShellStore.getState().settingsOpen).toBe(true);
    expect(useShellStore.getState().settingsSection).toBe("general");
  });
});
