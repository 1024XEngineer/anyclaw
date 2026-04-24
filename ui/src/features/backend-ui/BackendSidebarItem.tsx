import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

type BackendSidebarItemProps = {
  active?: boolean;
  description?: ReactNode;
  icon?: LucideIcon;
  label: string;
  meta?: ReactNode;
  onClick?: () => void;
  trailing?: ReactNode;
};

export function BackendSidebarItem({
  active = false,
  description,
  icon: Icon,
  label,
  meta,
  onClick,
  trailing,
}: BackendSidebarItemProps) {
  const className = [
    "group relative flex w-full items-start gap-3 border-b border-skin px-3 py-3 text-left transition-colors duration-150 last:border-b-0",
    active ? "bg-[#f8fafc] text-ink" : "text-mute hover:bg-white/64 hover:text-ink",
  ].join(" ");

  const content = (
    <>
      <span
        className={[
          "absolute left-0 top-3 bottom-3 w-[3px] rounded-r-full",
          active ? "bg-[#7b8fad]" : "bg-transparent",
        ].join(" ")}
      />

      {Icon ? (
        <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-2xl bg-[#f3f6fb] text-[#607699]">
          <Icon size={16} strokeWidth={2.1} />
        </span>
      ) : null}

      <span className="min-w-0 flex-1">
        <span className="flex items-center justify-between gap-3">
          <span className="truncate text-[15px] font-semibold text-ink">{label}</span>
          {trailing ? <span className="shrink-0 text-xs text-[#64748b]">{trailing}</span> : null}
        </span>
        {meta ? <span className="mt-1 block truncate text-sm text-[#607699]">{meta}</span> : null}
        {description ? <span className="mt-1 block text-sm leading-6 text-mute">{description}</span> : null}
      </span>
    </>
  );

  if (onClick) {
    return (
      <button className={className} onClick={onClick} type="button">
        {content}
      </button>
    );
  }

  return <div className={className}>{content}</div>;
}
