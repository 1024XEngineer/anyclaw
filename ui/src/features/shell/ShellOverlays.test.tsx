import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ShellOverlays } from "@/features/shell/ShellOverlays";
import { useShellStore } from "@/features/shell/useShellStore";

vi.mock("@/features/agent-drawer/AgentDrawer", () => ({
  AgentDrawer: () => <div>agent drawer mock</div>,
}));

vi.mock("@/features/model-settings/ModelSettingsModal", () => ({
  ModelSettingsModal: () => <div>model settings mock</div>,
}));

vi.mock("@/features/settings/SettingsModal", () => ({
  SettingsModal: () => <div>settings modal mock</div>,
}));

function resetStore() {
  useShellStore.setState({
    agentDrawerOpen: false,
    modelSettingsOpen: false,
    settingsOpen: false,
    settingsSection: "general",
  });
}

describe("ShellOverlays", () => {
  beforeEach(() => {
    resetStore();
    document.body.style.overflow = "";
  });

  it("renders the requested overlay", () => {
    useShellStore.getState().openSettings();

    render(<ShellOverlays />);

    expect(screen.getByText("settings modal mock")).toBeInTheDocument();
  });

  it("closes overlays when escape is pressed", async () => {
    useShellStore.getState().openModelSettings();

    render(<ShellOverlays />);

    expect(screen.getByText("model settings mock")).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "Escape" });

    await waitFor(() => {
      expect(useShellStore.getState().modelSettingsOpen).toBe(false);
    });
  });

  it("locks body scroll while an overlay is open", () => {
    useShellStore.getState().openAgentDrawer();

    const { unmount } = render(<ShellOverlays />);

    expect(document.body.style.overflow).toBe("hidden");

    unmount();
    expect(document.body.style.overflow).toBe("");
  });
});
