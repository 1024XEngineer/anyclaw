import type { ReactNode } from "react";

type BackendSummaryItem = {
  active?: boolean;
  label: string;
  onClick?: () => void;
  value: ReactNode;
};

type BackendSummaryStripProps = {
  items: BackendSummaryItem[];
};

export function BackendSummaryStrip({ items }: BackendSummaryStripProps) {
  return (
    <div className="flex flex-wrap gap-3">
      {items.map((item) => {
        const className = [
          "min-w-[132px] rounded-[16px] border px-4 py-3 text-left transition-colors duration-150",
          item.active
            ? "border-[#d8e1ef] bg-[#f4f8ff] text-ink"
            : "border-skin bg-white text-ink",
          item.onClick ? "hover:bg-[#f8fafc]" : "",
        ].join(" ");

        const content = (
          <>
            <div className="text-[11px] font-medium uppercase tracking-[0.16em] text-[#98a2b3]">
              {item.label}
            </div>
            <div className="mt-1 text-sm font-medium text-ink">{item.value}</div>
          </>
        );

        if (item.onClick) {
          return (
            <button key={item.label} className={className} onClick={item.onClick} type="button">
              {content}
            </button>
          );
        }

        return (
          <div key={item.label} className={className}>
            {content}
          </div>
        );
      })}
    </div>
  );
}
