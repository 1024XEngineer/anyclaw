import { useEffect } from "react";

import { AgentDrawer } from "@/features/agent-drawer/AgentDrawer";
import { ModelSettingsModal } from "@/features/model-settings/ModelSettingsModal";
import { SettingsModal } from "@/features/settings/SettingsModal";
import { useShellStore } from "@/features/shell/useShellStore";

export function ShellOverlays() {
  const {
    agentDrawerOpen,
    closeAgentDrawer,
    closeAll,
    closeModelSettings,
    closeSettings,
    modelSettingsOpen,
    settingsOpen,
  } = useShellStore();

  const hasOverlay = agentDrawerOpen || modelSettingsOpen || settingsOpen;

  useEffect(() => {
    if (!hasOverlay) return;

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeAll();
    };

    window.addEventListener("keydown", onKeyDown);

    return () => {
      document.body.style.overflow = previousOverflow;
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [closeAll, hasOverlay]);

  return (
    <>
      {settingsOpen && (
        <div className="fixed inset-0 z-[70] flex items-center justify-center p-3 sm:p-5">
          <button
            aria-label="关闭设置遮罩"
            className="absolute inset-0 z-0 bg-[rgba(23,20,18,0.42)] backdrop-blur-[8px]"
            onClick={closeSettings}
            type="button"
          />
          <div className="relative z-10 flex w-full items-center justify-center">
            <SettingsModal onClose={closeSettings} />
          </div>
        </div>
      )}

      {modelSettingsOpen && (
        <div className="fixed inset-0 z-[76] flex items-center justify-center p-3 sm:p-5">
          <button
            aria-label="关闭模型设置遮罩"
            className="absolute inset-0 z-0 bg-[rgba(23,20,18,0.42)] backdrop-blur-[8px]"
            onClick={closeModelSettings}
            type="button"
          />
          <div className="relative z-10 flex w-full items-center justify-center">
            <ModelSettingsModal onClose={closeModelSettings} />
          </div>
        </div>
      )}

      {agentDrawerOpen && (
        <div className="fixed inset-0 z-[80]">
          <button
            aria-label="关闭 Agent 抽屉遮罩"
            className="absolute inset-0 z-0 bg-[rgba(23,20,18,0.34)] backdrop-blur-[6px]"
            onClick={closeAgentDrawer}
            type="button"
          />
          <div className="pointer-events-none absolute inset-y-0 right-0 z-10 flex w-full justify-end">
            <AgentDrawer onClose={closeAgentDrawer} />
          </div>
        </div>
      )}
    </>
  );
}
