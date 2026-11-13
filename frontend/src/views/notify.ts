/**
 * What a shared view model reports back to its shell.
 *
 * Deliberately not a toast type: the desktop has three kinds plus a details
 * list, the phone has two tones and no detail rows, so each shell maps a notice
 * into its own notifier. A view model that imported either toast type would
 * bend the other platform's shape around it.
 */
export type ViewNotice = {
  kind: "success" | "info" | "error";
  text: string;
  details?: string[];
};

export type ViewNotify = (notice: ViewNotice) => void;
