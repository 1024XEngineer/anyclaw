import type { LucideIcon } from "lucide-react";

type BackendEmptyStateProps = {
  description?: string;
  icon: LucideIcon;
  title: string;
};

export function BackendEmptyState({
  description,
  icon: Icon,
  title,
}: BackendEmptyStateProps) {
  return (
    <div className="rounded-[18px] border border-dashed border-[#dbe4f0] bg-white px-5 py-8">
      <div className="flex items-start gap-4">
        <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-[14px] bg-[#f3f6fb] text-[#607699]">
          <Icon size={20} strokeWidth={2.1} />
        </span>
        <div>
          <h3 className="text-[22px] font-semibold tracking-[-0.03em] text-ink">{title}</h3>
          {description ? <p className="mt-2 text-sm leading-7 text-mute">{description}</p> : null}
        </div>
      </div>
    </div>
  );
}
