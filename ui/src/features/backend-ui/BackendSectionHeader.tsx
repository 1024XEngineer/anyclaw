type BackendSectionHeaderProps = {
  countLabel: string;
  description?: string;
  title: string;
};

export function BackendSectionHeader({
  countLabel,
  description,
  title,
}: BackendSectionHeaderProps) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <h2 className="text-[26px] font-semibold tracking-[-0.04em] text-ink">{title}</h2>
        {description ? <p className="mt-1 text-sm leading-6 text-mute">{description}</p> : null}
      </div>
      <div className="text-sm text-[#64748b]">{countLabel}</div>
    </div>
  );
}
