import { create } from "zustand";

export type SettingsSection = "about" | "agents" | "channels" | "general" | "skills" | "usage";

type ShellStore = {
  agentDrawerOpen: boolean;
  modelSettingsOpen: boolean;
  settingsOpen: boolean;
  settingsSection: SettingsSection;
  closeAll: () => void;
  closeAgentDrawer: () => void;
  closeModelSettings: () => void;
  closeSettings: () => void;
  openAgentDrawer: () => void;
  openModelSettings: () => void;
  openSettings: (section?: SettingsSection) => void;
  setSettingsSection: (section: SettingsSection) => void;
};

export const useShellStore = create<ShellStore>((set) => ({
  agentDrawerOpen: false,
  modelSettingsOpen: false,
  settingsOpen: false,
  settingsSection: "general",
  closeAll: () => set({ agentDrawerOpen: false, modelSettingsOpen: false, settingsOpen: false }),
  closeAgentDrawer: () => set({ agentDrawerOpen: false }),
  closeModelSettings: () => set({ modelSettingsOpen: false }),
  closeSettings: () => set({ settingsOpen: false }),
  openAgentDrawer: () => set({ agentDrawerOpen: true, modelSettingsOpen: false, settingsOpen: false }),
  openModelSettings: () => set({ agentDrawerOpen: false, modelSettingsOpen: true, settingsOpen: false }),
  openSettings: (section = "general") =>
    set({
      agentDrawerOpen: false,
      modelSettingsOpen: false,
      settingsOpen: true,
      settingsSection: section,
    }),
  setSettingsSection: (section) => set({ settingsSection: section }),
}));
