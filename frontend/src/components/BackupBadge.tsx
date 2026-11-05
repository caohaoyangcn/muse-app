import { cn } from "../lib/utils";

/**
 * 已备份: the same work (番号 + part) has an active, available copy on both a
 * local disk and a cloud source. It is a work-level marker, not a claim that
 * the two files are byte-identical or the same encode.
 */
export default function BackupBadge({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center whitespace-nowrap rounded-[4px] border border-[rgba(63,178,111,0.45)] bg-[rgba(63,178,111,0.14)] px-1.5 py-0.5 text-[10px] font-medium leading-none text-[#8fe0b0]",
        className,
      )}
      title="同番号在本地和网盘均有可用收录；可能是不同清晰度或版本"
    >
      已备份
    </span>
  );
}
