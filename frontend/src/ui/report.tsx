import { Banner } from "./astryx";

export type ReportKind = "success" | "info" | "error";

/** One outcome, before the shell assigns it an id. */
export interface Notice {
  kind: ReportKind;
  message: string;
  details?: string[];
}

/** A persistent outcome with optional detail lines. */
export interface ReportItem extends Notice {
  id: number;
}

/**
 * The shell's report surface: one Banner for the latest outcome, with every
 * other report reachable behind its expand toggle. Reports are for content that
 * is too long for a toast (scan summaries, error causes); short confirmations
 * stay in `ToastStack`.
 */
export function ReportBanner({
  reports,
  onDismiss,
}: {
  reports: ReportItem[];
  onDismiss: () => void;
}) {
  if (reports.length === 0) return null;
  const latest = reports[0];
  // A single report keeps its own lines; several reports prefix each line with
  // the message it came from so the combined list still reads correctly.
  const lines =
    reports.length === 1
      ? (reports[0].details ?? [])
      : reports.flatMap((report) =>
          report.details?.length
            ? report.details.map((detail) => `${report.message}：${detail}`)
            : [report.message],
        );

  return (
    <Banner
      status={latest.kind}
      title={latest.message}
      description={reports.length > 1 ? `另有 ${reports.length - 1} 条较早报告` : undefined}
      isDismissable
      onDismiss={onDismiss}
    >
      {lines.length > 0 ? (
        <ul className="m-0 list-disc space-y-1 pl-4 text-xs leading-relaxed">
          {lines.map((line, index) => (
            <li key={index}>{line}</li>
          ))}
        </ul>
      ) : null}
    </Banner>
  );
}
