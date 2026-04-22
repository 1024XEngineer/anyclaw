import type { LucideIcon } from "lucide-react";

type HeaderStat = {
  label: string;
  value: string;
};

type BackendPageHeaderProps = {
  description?: string;
  icon: LucideIcon;
  sectionLabel: string;
  sourceLabel: string;
  stats: HeaderStat[];
  title: string;
};

export function BackendPageHeader({
  description,
  icon: Icon,
  sectionLabel,
  sourceLabel,
  stats,
  title,
}: BackendPageHeaderProps) {
  return (
    <header className="border-b border-skin pb-6">
      <div className="flex flex-wrap items-center gap-3 text-sm">
        <div className="inline-flex items-center gap-2 text-[#607699]">
          <Icon size={16} strokeWidth={2.2} />
          <span className="font-medium">{sectionLabel}</span>
        </div>
        <span className="text-[#98a2b3]">/</span>
        <div className="text-[#64748b]">{sourceLabel}</div>
      </div>

      <div className="mt-5 flex flex-col gap-6 xl:flex-row xl:items-end xl:justify-between">
        <div className={["max-w-[920px]", description ? "space-y-3" : ""].join(" ").trim()}>
          <h1 className="text-[clamp(34px,6vw,56px)] font-semibold tracking-[-0.05em] text-ink">
            {title}
          </h1>
          {description ? (
            <p className="max-w-[860px] text-[15px] leading-8 text-mute lg:text-[17px]">{description}</p>
          ) : null}
        </div>

        <div className="grid gap-x-8 gap-y-3 text-sm text-mute sm:grid-cols-2 xl:grid-cols-4">
          {stats.map((stat) => (
            <div key={stat.label}>
              <div>{stat.label}</div>
              <div className="mt-1 text-[20px] font-semibold tracking-[-0.03em] text-ink">
                {stat.value}
              </div>
            </div>
          ))}
        </div>
      </div>
    </header>
  );
}
