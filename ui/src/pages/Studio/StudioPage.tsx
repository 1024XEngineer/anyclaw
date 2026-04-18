import { Hammer } from "lucide-react";
import { NavLink } from "react-router-dom";

const tabs = [
  { label: "对话", to: "/" },
  { label: "工作台", to: "/studio" },
];

export function StudioPage() {
  return (
    <div className="relative z-10 flex min-h-full flex-1 flex-col">
      <header className="px-5 pb-3 pt-5 sm:px-6 lg:px-8 lg:pt-7">
        <div className="inline-flex max-w-full rounded-full border border-white/70 bg-white/72 p-1 shadow-soft">
          <nav className="flex flex-wrap items-center gap-1">
            {tabs.map((tab) => (
              <NavLink
                key={tab.label}
                className={({ isActive }) =>
                  [
                    "rounded-full px-5 py-3 text-base font-medium transition-colors duration-150 lg:px-7 lg:text-[18px]",
                    isActive ? "bg-[#1e232b] text-white" : "text-mute hover:text-ink",
                  ].join(" ")
                }
                end={tab.to === "/"}
                to={tab.to}
              >
                {tab.label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>

      <div className="flex flex-1 items-center justify-center px-5 py-8 sm:px-6 lg:px-8 lg:py-10">
        <section className="shell-panel mx-auto flex w-full max-w-[920px] items-start gap-5 rounded-[36px] border border-white/80 p-6 shadow-panel lg:p-8">
          <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-[22px] bg-[linear-gradient(135deg,#edf3fb,#dbe7f6)] text-[#556274]">
            <Hammer size={24} strokeWidth={2.1} />
          </div>
          <div className="space-y-3">
            <h2 className="text-[30px] font-semibold tracking-[-0.04em] text-ink lg:text-[34px]">
              工作区暂未接入
            </h2>
            <p className="max-w-[620px] text-lg leading-8 text-mute">后续会在这里接入工作台。</p>
          </div>
        </section>
      </div>
    </div>
  );
}
