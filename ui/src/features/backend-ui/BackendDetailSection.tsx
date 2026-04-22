import type { ReactNode } from "react";

type BackendDetailSectionProps = {
  children: ReactNode;
  description?: string;
  title: string;
};

export function BackendDetailSection({
  children,
  description,
  title,
}: BackendDetailSectionProps) {
  return (
    <section className="rounded-[22px] border border-skin bg-white p-5">
      <div>
        <h3 className="text-[18px] font-semibold tracking-[-0.03em] text-ink">{title}</h3>
        {description ? <p className="mt-2 text-sm leading-6 text-mute">{description}</p> : null}
      </div>
      <div className="mt-4">{children}</div>
    </section>
  );
}
