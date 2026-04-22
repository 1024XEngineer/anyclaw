import { Outlet } from "react-router-dom";

import { LeftRail } from "@/features/left-rail/LeftRail";
import { ShellOverlays } from "@/features/shell/ShellOverlays";

export function AppShell() {
  return (
    <div className="relative min-h-dvh bg-[#f6f8fb] text-ink lg:pl-[104px]">
      <LeftRail />
      <main className="relative min-h-dvh">
        <Outlet />
      </main>
      <ShellOverlays />
    </div>
  );
}
