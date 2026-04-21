import type { ReactNode } from "react";

type BackendProperty = {
  label: string;
  value: ReactNode;
};

type BackendPropertyListProps = {
  items: BackendProperty[];
};

export function BackendPropertyList({ items }: BackendPropertyListProps) {
  return (
    <dl className="overflow-hidden rounded-[18px] border border-skin bg-[#fbfcfe]">
      {items.map((item) => (
        <div
          key={item.label}
          className="flex items-start justify-between gap-4 border-b border-skin px-4 py-3 last:border-b-0"
        >
          <dt className="text-sm text-mute">{item.label}</dt>
          <dd className="text-right text-sm font-medium text-ink">{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}
