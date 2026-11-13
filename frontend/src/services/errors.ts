/**
 * A failed call, turned into something a user can read.
 *
 * The two transports fail differently: Wails rejects with the Go error (a
 * string, or an Error wrapping it), the LAN client rejects with `ApiError`.
 * Views must not branch on which one they are running behind, so they all go
 * through here instead of `String(e)` / `e instanceof ApiError`.
 */
export function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof Error) {
    const text = error.message.trim();
    return text || fallback;
  }
  if (typeof error === "string") {
    return error.trim() || fallback;
  }
  const text = String(error ?? "").trim();
  return !text || text === "[object Object]" ? fallback : text;
}
