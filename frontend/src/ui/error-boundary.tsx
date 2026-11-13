import { Component, type ErrorInfo, type ReactNode } from "react";

type Props = {
  children: ReactNode;
  /** What this boundary protects, e.g. "当前页面". Shown in the fallback. */
  scope: string;
  /**
   * "page" replaces the whole window (root boundary); "region" keeps the app
   * shell usable and only covers the area that failed.
   */
  variant?: "page" | "region";
  /**
   * Changing this resets a failed boundary, so navigating away from a broken
   * screen (another tab, another video) recovers without a manual retry.
   */
  resetKey?: string | number;
};

type State = { error: Error | null };

/**
 * Catches render errors below it so one broken panel never takes the whole
 * window down. React has no hook equivalent: an error boundary must be a class.
 */
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // The fallback hides the stack from users; the console keeps it for whoever
    // is debugging. Tagged so it is easy to find in the devtools log.
    console.error(`[ui] ${this.props.scope} failed to render`, error, info.componentStack);
  }

  componentDidUpdate(prevProps: Props) {
    if (this.state.error && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ error: null });
    }
  }

  private retry = () => {
    this.setState({ error: null });
  };

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;

    if (this.props.variant === "page") {
      return <PageFallback error={error} onRetry={this.retry} />;
    }
    return <RegionFallback scope={this.props.scope} error={error} onRetry={this.retry} />;
  }
}

function PageFallback({ error, onRetry }: { error: Error; onRetry: () => void }) {
  return (
    <div
      className="flex min-h-full flex-col items-center justify-center bg-[var(--bg)] px-6 py-10 text-[var(--text)]"
      role="alert"
    >
      <div className="w-full max-w-[560px] text-center">
        <h1 className="text-lg font-semibold">界面遇到问题</h1>
        <p className="mt-2 text-sm leading-relaxed text-[var(--muted)]">
          本次操作没能完成渲染。收藏记录、缩略图和视频文件都没有受到影响。
        </p>
        <div className="mt-5 flex justify-center gap-2">
          <button
            className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] px-3.5 py-1.5 text-sm hover:border-[var(--accent)]"
            onClick={onRetry}
            type="button"
          >
            重试
          </button>
          <button
            className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] px-3.5 py-1.5 text-sm hover:border-[var(--accent)]"
            onClick={() => window.location.reload()}
            type="button"
          >
            重新加载
          </button>
        </div>
        <ErrorDetails error={error} />
      </div>
    </div>
  );
}

function RegionFallback({ scope, error, onRetry }: { scope: string; error: Error; onRetry: () => void }) {
  return (
    <div
      className="flex min-h-[220px] flex-1 flex-col items-center justify-center gap-3 px-6 py-10 text-center"
      role="alert"
    >
      <div className="text-sm text-[var(--text)]">「{scope}」没能显示出来</div>
      <div className="text-xs text-[var(--muted)]">其余界面仍可正常使用。</div>
      <button
        className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--panel-2)] px-3.5 py-1.5 text-sm hover:border-[var(--accent)]"
        onClick={onRetry}
        type="button"
      >
        重试
      </button>
      <ErrorDetails error={error} />
    </div>
  );
}

// A one-line message is always shown so a bug report can name the failure; the
// stack stays in development, where it is useful.
function ErrorDetails({ error }: { error: Error }) {
  return (
    <div className="mt-4 text-xs text-[var(--muted)]">
      <p className="break-words">{error.message || String(error)}</p>
      {import.meta.env.DEV && (
        <details className="mt-2 max-h-64 overflow-auto text-left">
          <summary className="cursor-pointer">技术细节</summary>
          <pre className="mt-2 whitespace-pre-wrap break-words text-[11px] leading-relaxed">
            {error.stack || String(error)}
          </pre>
        </details>
      )}
    </div>
  );
}
