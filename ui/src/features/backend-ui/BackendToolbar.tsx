import { Search } from "lucide-react";

type ToolbarItem = {
  active: boolean;
  label: string;
  onClick: () => void;
};

type ToolbarGroup = {
  items: ToolbarItem[];
};

type BackendToolbarProps = {
  groups: ToolbarGroup[];
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  searchValue: string;
};

export function BackendToolbar({
  groups,
  onSearchChange,
  searchPlaceholder,
  searchValue,
}: BackendToolbarProps) {
  return (
    <section className="flex flex-col gap-4 border-b border-skin py-5 xl:flex-row xl:items-center xl:justify-between">
      <label className="flex h-12 w-full max-w-[720px] items-center gap-3 rounded-[14px] border border-skin bg-white px-4">
        <Search className="text-mute" size={18} strokeWidth={2} />
        <input
          className="w-full bg-transparent text-[15px] text-ink outline-none placeholder:text-[#98a2b3]"
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder={searchPlaceholder}
          value={searchValue}
        />
      </label>

      <div className="flex flex-wrap items-center gap-4">
        {groups.map((group, groupIndex) => (
          <div key={groupIndex} className="flex overflow-hidden rounded-[14px] border border-skin bg-white">
            {group.items.map((item, itemIndex) => (
              <button
                key={item.label}
                className={[
                  itemIndex > 0 ? "border-l border-skin" : "",
                  "px-4 py-2.5 text-sm font-medium transition-colors duration-150",
                  item.active
                    ? "bg-[#1f2430] text-white"
                    : "text-[#64748b] hover:bg-[#f8fafc] hover:text-ink",
                ].join(" ")}
                onClick={item.onClick}
                type="button"
              >
                {item.label}
              </button>
            ))}
          </div>
        ))}
      </div>
    </section>
  );
}
