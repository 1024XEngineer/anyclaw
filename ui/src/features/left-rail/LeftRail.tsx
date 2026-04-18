import { CircleHelp, MessageCircle, Settings, Store, Waypoints } from "lucide-react";
import { NavLink } from "react-router-dom";

import { useShellStore } from "@/features/shell/useShellStore";
import { useWorkspaceOverview } from "@/features/workspace/useWorkspaceOverview";

const primaryItems = [
  { icon: MessageCircle, label: "对话", to: "/" },
  { icon: Store, label: "市场", to: "/market" },
  { icon: Waypoints, label: "渠道", to: "/channels" },
];

export function LeftRail() {
  const { data } = useWorkspaceOverview();
  const openSettings = useShellStore((state) => state.openSettings);
  const avatarLetter = (data.runtimeProfile.name || "A").slice(0, 1).toUpperCase();

  return (
    <aside className="rail-surface relative z-20 flex w-full shrink-0 items-center justify-between border-b border-[rgba(15,23,42,0.06)] px-4 py-4 lg:fixed lg:inset-y-0 lg:left-0 lg:w-[104px] lg:min-w-[104px] lg:flex-col lg:justify-between lg:border-b-0 lg:border-r lg:px-4 lg:py-7">
      <div className="flex min-w-0 items-center gap-3 lg:w-full lg:flex-col lg:items-center lg:gap-7">
        <button
          aria-label="AnyClaw profile"
          className="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-full bg-[radial-gradient(circle_at_30%_30%,#d8ecff,transparent_35%),linear-gradient(145deg,#2a3240,#5b6f8b)] text-lg font-semibold text-white shadow-[0_14px_24px_rgba(15,23,42,0.16)]"
          type="button"
        >
          {avatarLetter}
        </button>

        <nav
          aria-label="Primary navigation"
          className="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto pb-1 lg:w-full lg:flex-none lg:flex-col lg:gap-3 lg:overflow-visible lg:pb-0"
        >
          {[...primaryItems, { icon: Waypoints, label: "发现", to: "/discovery" }].map(({ icon: Icon, label, to }) => (
            <NavLink
              key={label}
              className={({ isActive }) =>
                [
                  "group flex shrink-0 items-center gap-2 rounded-full px-3 py-2 text-sm font-medium transition-all duration-150 lg:w-full lg:flex-col lg:gap-2 lg:rounded-[22px] lg:px-2 lg:py-3 lg:text-xs",
                  isActive ? "text-[#1d1f25]" : "text-[#667085] hover:text-[#1d1f25]",
                ].join(" ")
              }
              end={to === "/"}
              to={to}
            >
              {({ isActive }) => (
                <>
                  <span
                    className={[
                      "flex h-10 w-10 items-center justify-center rounded-[18px] transition-colors duration-150",
                      isActive ? "bg-[#1f2430] text-white" : "text-[#667085] group-hover:bg-[#f3f4f6]",
                    ].join(" ")}
                  >
                    <Icon size={18} strokeWidth={2.1} />
                  </span>
                  <span>{label}</span>
                </>
              )}
            </NavLink>
          ))}
        </nav>
      </div>

      <div className="flex shrink-0 items-center gap-2 lg:w-full lg:flex-col lg:gap-3">
        <button
          aria-label="帮助"
          className="group flex items-center gap-2 rounded-full px-3 py-2 text-sm font-medium text-[#667085] transition-all duration-150 hover:text-[#1d1f25] lg:w-full lg:flex-col lg:gap-2 lg:rounded-[22px] lg:px-2 lg:py-3 lg:text-xs"
          type="button"
        >
          <span className="flex h-10 w-10 items-center justify-center rounded-[18px] text-[#667085] transition-colors duration-150 group-hover:bg-[#f3f4f6] group-hover:text-[#1d1f25]">
            <CircleHelp size={18} strokeWidth={2.1} />
          </span>
        </button>

        <button
          aria-label="打开设置"
          className="group flex items-center gap-2 rounded-full px-3 py-2 text-sm font-medium text-[#667085] transition-all duration-150 hover:text-[#1d1f25] lg:w-full lg:flex-col lg:gap-2 lg:rounded-[22px] lg:px-2 lg:py-3 lg:text-xs"
          onClick={() => openSettings("general")}
          type="button"
        >
          <span className="flex h-10 w-10 items-center justify-center rounded-[18px] text-[#667085] transition-colors duration-150 group-hover:bg-[#f3f4f6] group-hover:text-[#1d1f25]">
            <Settings size={18} strokeWidth={2.1} />
          </span>
          <span>设置</span>
        </button>
      </div>
    </aside>
  );
}
