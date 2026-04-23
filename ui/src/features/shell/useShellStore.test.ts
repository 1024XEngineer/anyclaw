import { describe, expect, it } from "vitest";

import { useShellStore } from "@/features/shell/useShellStore";

function resetStore() {
  useShellStore.setState({
    agentDrawerOpen: false,
    modelSettingsOpen: false,
    settingsOpen: false,
    settingsSection: "general",
  });
}

describe("useShellStore", () => {
  it("opens one overlay at a time", () => {
    resetStore();

    useShellStore.getState().openAgentDrawer();
    expect(useShellStore.getState()).toMatchObject({
      agentDrawerOpen: true,
      modelSettingsOpen: false,
      settingsOpen: false,
    });

    useShellStore.getState().openModelSettings();
    expect(useShellStore.getState()).toMatchObject({
      agentDrawerOpen: false,
      modelSettingsOpen: true,
      settingsOpen: false,
    });

    useShellStore.getState().openSettings("channels");
    expect(useShellStore.getState()).toMatchObject({
      agentDrawerOpen: false,
      modelSettingsOpen: false,
      settingsOpen: true,
      settingsSection: "channels",
    });
  });

  it("closes all overlays together", () => {
    resetStore();
    useShellStore.getState().openSettings("skills");

    useShellStore.getState().closeAll();

    expect(useShellStore.getState()).toMatchObject({
      agentDrawerOpen: false,
      modelSettingsOpen: false,
      settingsOpen: false,
      settingsSection: "skills",
    });
  });
});
