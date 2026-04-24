import { Outlet } from "react-router-dom";

import { LeftRail } from "@/features/left-rail/LeftRail";
import { SessionSidebar } from "@/features/session-sidebar/SessionSidebar";
import { ShellOverlays } from "@/features/shell/ShellOverlays";

export function AppShell() {
  return (
    <div className="relative min-h-dvh bg-canvas text-ink lg:pl-[456px]">
      <LeftRail />
      <SessionSidebar />
      <main className="relative flex min-h-dvh min-w-0 flex-col bg-white">
        <Outlet />
      </main>
      <ShellOverlays />
    </div>
  );
}
