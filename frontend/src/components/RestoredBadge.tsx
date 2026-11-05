import { cn } from "../lib/utils";

/**
 * The tag shown beside a favorite's name when it belongs to the 马赛克破解
 * group — the automatic collection rule for restored/mosaic-removed works.
 *
 * The membership itself comes from the backend read model (`Video.mosaicRestored`,
 * transitive through child groups), so the desktop and LAN clients agree on who
 * carries the badge. This component only owns how it looks.
 */
export default function RestoredBadge({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center whitespace-nowrap rounded-[4px] border border-[rgba(124,92,255,0.45)] bg-[rgba(124,92,255,0.16)] px-1.5 py-0.5 text-[10px] font-medium leading-none text-[#cdc2ff]",
        className,
      )}
      title="已加入「马赛克破解」分组"
    >
      马赛克破解
    </span>
  );
}
