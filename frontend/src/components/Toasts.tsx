export interface ToastItem {
  id: number;
  kind: "success" | "error" | "info";
  message: string;
  details?: string[];
}

export default function Toasts({
  toasts,
  onDismiss,
}: {
  toasts: ToastItem[];
  onDismiss: (id: number) => void;
}) {
  if (toasts.length === 0) return null;
  return (
    <div className="fixed right-4 top-4 z-[400] flex w-[340px] flex-col gap-2.5">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={cn(
            "cursor-pointer rounded-lg border border-[var(--border)] border-l-[3px] bg-[var(--panel-2)] px-3.5 py-2.5 shadow-[0_8px_24px_rgba(0,0,0,0.45)]",
            t.kind === "success" && "border-l-[#3fb26f]",
            t.kind === "error" && "border-l-[var(--danger)]",
            t.kind === "info" && "border-l-[var(--accent)]",
          )}
          onClick={() => onDismiss(t.id)}
        >
          <div className="font-medium">{t.message}</div>
          {t.details && t.details.length > 0 && (
            <ul className="mt-2 list-disc pl-[18px] text-xs leading-relaxed text-[var(--muted)] break-words">
              {t.details.map((d, i) => (
                <li key={i}>{d}</li>
              ))}
            </ul>
          )}
        </div>
      ))}
    </div>
  );
}
import { cn } from "../lib/utils";
