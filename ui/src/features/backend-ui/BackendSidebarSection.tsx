import type { ReactNode } from "react";

type BackendSidebarSectionProps = {
  bodyClassName?: string;
  className?: string;
  children: ReactNode;
  count?: string;
  title: string;
};

export function BackendSidebarSection({
  bodyClassName,
  className,
  children,
  count,
  title,
}: BackendSidebarSectionProps) {
  return (
    <section
      className={[
        "border-b border-skin pb-5 last:border-b-0 last:pb-0",
        className ?? "",
      ].join(" ")}
    >
      <div className="flex items-center justify-between gap-3">
        <div className="text-xs font-medium uppercase tracking-[0.18em] text-[#98a2b3]">{title}</div>
        {count ? <div className="text-sm text-[#64748b]">{count}</div> : null}
      </div>
      <div className={["mt-3", bodyClassName ?? ""].join(" ")}>{children}</div>
    </section>
  );
}
