export type StatusBadgeTone = "default" | "info" | "success" | "warning";

type StatusBadgeProps = {
  label: string;
  tone?: StatusBadgeTone;
};

const toneClassName: Record<StatusBadgeTone, string> = {
  default: "bg-[#f3f6fb] text-[#5b6f8b]",
  info: "bg-[#eef4ff] text-[#49658d]",
  success: "bg-[#eefaf4] text-[#3f7a59]",
  warning: "bg-[#fff4e8] text-[#8a6135]",
};

export function StatusBadge({ label, tone = "default" }: StatusBadgeProps) {
  return (
    <span
      className={[
        "inline-flex rounded-[10px] px-2.5 py-1.5 text-xs font-medium",
        toneClassName[tone],
      ].join(" ")}
    >
      {label}
    </span>
  );
}
