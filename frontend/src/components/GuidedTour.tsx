import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ArrowLeft, ArrowRight, Lightbulb, X } from "lucide-react";
import {
  TOUR_STEPS,
  cardPosition,
  clampToViewport,
  pickPlacement,
  rectOf,
  spotlightRect,
  type Rect,
  type TourStep,
} from "../utils/tour";
import { Button } from "../ui/button";

const OVERLAY_Z = 500;
/** How long the card waits for its target to appear (a view may still render). */
const TARGET_TIMEOUT_MS = 1600;
const DIM = "rgba(9, 9, 12, 0.68)";

// Two authored moments: the card arrives from its anchor, the ring breathes
// once per cycle to say "here". Reduced-motion users get the final state.
const TOUR_CSS = `
@keyframes muse-tour-in {
  from { opacity: 0; transform: translateY(6px) scale(0.985); }
  to   { opacity: 1; transform: translateY(0) scale(1); }
}
@keyframes muse-tour-pulse {
  0%   { box-shadow: 0 0 0 2px var(--accent), 0 0 0 2px rgba(124, 92, 255, 0.38); }
  70%  { box-shadow: 0 0 0 2px var(--accent), 0 0 0 14px rgba(124, 92, 255, 0); }
  100% { box-shadow: 0 0 0 2px var(--accent), 0 0 0 14px rgba(124, 92, 255, 0); }
}
.muse-tour-ring { animation: muse-tour-pulse 2.4s cubic-bezier(0.16, 1, 0.3, 1) infinite; }
@media (prefers-reduced-motion: reduce) {
  .muse-tour-ring { animation: none; }
  .muse-tour-card { animation: none !important; }
}
`;

/**
 * The first-run guided tour, rendered over the whole window.
 *
 * The card is informational: the page under it stays visible through the
 * spotlight cut-out, but the dimmed area does not take clicks, so the user
 * cannot wander off mid-tour or fire a scrape by accident. Navigation is
 * explicit — arrows, the buttons, or Esc to leave.
 */
export default function GuidedTour({
  open,
  stepIndex,
  onAdvance,
  onClose,
}: {
  open: boolean;
  /** Owned by useTour so the shell can follow step changes. */
  stepIndex: number;
  onAdvance: (next: number) => void;
  onClose: () => void;
}) {
  // The spotlit box in viewport coordinates; null means "no usable target".
  const [spot, setSpot] = useState<Rect | null>(null);
  const cardRef = useRef<HTMLDivElement | null>(null);
  const closeRef = useRef<HTMLButtonElement | null>(null);
  const targetRef = useRef<HTMLElement | null>(null);

  const step = TOUR_STEPS[Math.min(stepIndex, TOUR_STEPS.length - 1)];
  const placement = useMemo(() => (spot ? pickPlacement(spot) : "center"), [spot]);
  const measured = spot != null && placement !== "center";

  // Leaving the tour is the shell's call: it records the first-run flag and
  // decides where the user lands afterwards. This component only renders.
  const next = useCallback(() => {
    if (stepIndex >= TOUR_STEPS.length - 1) {
      onClose();
      return;
    }
    onAdvance(stepIndex + 1);
  }, [stepIndex, onAdvance, onClose]);

  const back = useCallback(() => {
    if (stepIndex > 0) onAdvance(stepIndex - 1);
  }, [stepIndex, onAdvance]);

  // --- measurement ---------------------------------------------------------

  // Recomputes the spotlit box for the current step. Runs once the step's view
  // has rendered; polls briefly because switching to Settings mounts the page
  // after this effect.
  const measure = useCallback((current: TourStep) => {
    if (!current.target) {
      setSpot(null);
      targetRef.current = null;
      return;
    }
    const selector = `[data-tour="${current.target}"]`;
    let cancelled = false;
    let timer: number | null = null;
    const started = performance.now();

    const attempt = () => {
      if (cancelled) return;
      const el = document.querySelector<HTMLElement>(selector);
      if (el) {
        const box = clampToViewport(spotlightRect(el.getBoundingClientRect()), 4);
        const { width, height } = rectOf(box);
        if (width > 0 && height > 0) {
          targetRef.current = el;
          setSpot(box);
          return;
        }
      }
      if (performance.now() - started > TARGET_TIMEOUT_MS) {
        targetRef.current = null;
        setSpot(null);
        return;
      }
      timer = window.setTimeout(attempt, 60);
    };

    setSpot(null);
    targetRef.current = null;
    attempt();

    return () => {
      cancelled = true;
      if (timer != null) window.clearTimeout(timer);
    };
  }, []);

  // The target is measured after the step commits. Steps that live on another
  // screen may name a section there; the screen switch itself is owned by the
  // shell's useTour `onStep`, which runs before this effect on the same change.
  useEffect(() => {
    if (!open) return;
    return measure(step);
  }, [open, step, measure]);

  // Keep the spotlight glued to its element while the window changes size or
  // the user scrolls the library list under it.
  useEffect(() => {
    if (!open) return;
    const sync = () => {
      const el = targetRef.current;
      if (!el) return;
      const box = clampToViewport(spotlightRect(el.getBoundingClientRect()), 4);
      const { width, height } = rectOf(box);
      setSpot(width > 0 && height > 0 ? box : null);
    };
    window.addEventListener("resize", sync);
    window.addEventListener("scroll", sync, true);
    return () => {
      window.removeEventListener("resize", sync);
      window.removeEventListener("scroll", sync, true);
    };
  }, [open]);

  // Focus moves into the card on every step, so keyboard users are not left on
  // the page behind the tour.
  useEffect(() => {
    if (!open) return;
    const card = cardRef.current;
    if (!card) return;
    const preferred = card.querySelector<HTMLElement>("[data-tour-primary]");
    (preferred ?? closeRef.current ?? card).focus({ preventScroll: true });
  }, [open, stepIndex]);

  // --- keyboard ------------------------------------------------------------

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      // A real dialog on top of the tour keeps its own keyboard contract.
      const stackedDialog = Array.from(
        document.querySelectorAll('[role="dialog"][aria-modal="true"]'),
      ).some((el) => !el.hasAttribute("data-tour-root"));
      if (stackedDialog) return;

      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      // Arrows are the tour's own navigation, on every step: → advances (and
      // finishes the last card), ← goes back. They never reach the page.
      if (e.key === "ArrowRight") {
        e.preventDefault();
        next();
        return;
      }
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        back();
        return;
      }
      if (e.key === "Enter") {
        const onControl = (e.target as HTMLElement | null)?.closest(
          "button, a, input, textarea, select",
        );
        if (onControl) return;
        e.preventDefault();
        next();
        return;
      }
      if (e.key === "Tab") {
        const card = cardRef.current;
        if (!card) return;
        const focusables = Array.from(
          card.querySelectorAll<HTMLElement>(
            'button:not([disabled]), a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
          ),
        );
        if (focusables.length === 0) return;
        const first = focusables[0];
        const last = focusables[focusables.length - 1];
        if (e.shiftKey && (document.activeElement === first || document.activeElement === card)) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, step.intro, step.outro, onClose, next, back]);

  // The page behind the tour is inert: it cannot be clicked, tabbed into, or
  // announced while the tour is on screen.
  useEffect(() => {
    if (!open) return;
    const root = document.getElementById("root");
    if (!root) return;
    root.setAttribute("inert", "");
    root.setAttribute("aria-hidden", "true");
    return () => {
      root.removeAttribute("inert");
      root.removeAttribute("aria-hidden");
    };
  }, [open]);

  if (!open) return null;

  const { style: cardStyle, arrow } = cardPosition(placement, spot);
  const paragraphs = step.body.split("\n").filter((p) => p.trim() !== "");

  return createPortal(
    <div
      data-tour-root
      className="fixed inset-0"
      style={{ zIndex: OVERLAY_Z, pointerEvents: "none" }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="muse-tour-title"
      aria-describedby="muse-tour-body"
    >
      <style>{TOUR_CSS}</style>

      {measured ? (
        <>
          {/* Four panes around the cut-out: the dim is real, the hole is the
              page itself, so the target stays crisp and interactive. */}
          <Pane style={{ left: 0, right: 0, top: 0, height: spot!.top }} />
          <Pane style={{ left: 0, right: 0, top: spot!.bottom, bottom: 0 }} />
          <Pane style={{ left: 0, top: spot!.top, width: spot!.left, height: spot!.bottom - spot!.top }} />
          <Pane
            style={{
              left: spot!.right,
              top: spot!.top,
              right: 0,
              height: spot!.bottom - spot!.top,
            }}
          />
          <div
            className="muse-tour-ring pointer-events-none fixed rounded-[12px]"
            style={{
              left: spot!.left,
              top: spot!.top,
              width: spot!.right - spot!.left,
              height: spot!.bottom - spot!.top,
              transition: "left 260ms ease, top 260ms ease, width 260ms ease, height 260ms ease",
            }}
            aria-hidden
          />
        </>
      ) : (
        <div
          className="fixed inset-0"
          style={{ background: DIM, backdropFilter: "blur(2px)", pointerEvents: "auto" }}
          aria-hidden
        />
      )}

      <div
        ref={cardRef}
        className="muse-tour-card fixed rounded-[14px] border"
        style={{
          ...cardStyle,
          background: "var(--panel-2)",
          borderColor: "rgba(124, 92, 255, 0.5)",
          boxShadow: "0 24px 64px rgba(0, 0, 0, 0.55), 0 0 0 1px rgba(0, 0, 0, 0.35)",
          color: "var(--text)",
          pointerEvents: "auto",
          animation: "muse-tour-in 240ms cubic-bezier(0.16, 1, 0.3, 1) both",
          outline: "none",
        }}
        tabIndex={-1}
      >
        {arrow && (
          <span
            className="pointer-events-none absolute h-3.5 w-3.5 rounded-[2px] border"
            style={{
              ...arrow,
              background: "var(--panel-2)",
              borderColor: "rgba(124, 92, 255, 0.5)",
            }}
            aria-hidden
          />
        )}

        <div className="max-h-[calc(100dvh-32px)] overflow-y-auto rounded-[14px] p-5 pb-4">
          <div className="mb-2.5 flex items-start gap-2.5">
            <div className="min-w-0 flex-1">
              <div
                className="mb-1 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.9px]"
                style={{ color: "#a3a3b0" }}
              >
                <span
                  className="h-1.5 w-1.5 rounded-full"
                  style={{ background: "var(--accent)" }}
                  aria-hidden
                />
                {step.intro
                  ? "开始之前"
                  : step.outro
                    ? "完成"
                    : `第 ${stepIndex + 1} 步，共 ${TOUR_STEPS.length} 步`}
              </div>
              <h2 id="muse-tour-title" className="m-0 text-[17px] font-semibold leading-snug">
                {step.title}
              </h2>
            </div>
            <button
              ref={closeRef}
              type="button"
              onClick={onClose}
              aria-label="退出新手引导"
              title="退出引导 (Esc)"
              className="rounded-md p-1 opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
              style={{ color: "#a3a3b0" }}
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <div id="muse-tour-body" className="space-y-2 text-[13.5px] leading-[1.65]">
            {paragraphs.map((p, i) => (
              <p key={i} className="m-0">
                {p}
              </p>
            ))}
          </div>

          {step.hint && (
            <div
              className="mt-3 flex items-start gap-2 rounded-[9px] px-2.5 py-2 text-[12.5px] leading-[1.55]"
              style={{
                background: "rgba(124, 92, 255, 0.1)",
                border: "1px solid rgba(124, 92, 255, 0.2)",
              }}
            >
              <Lightbulb
                className="mt-px h-3.5 w-3.5 shrink-0"
                style={{ color: "var(--accent)" }}
                aria-hidden
              />
              <span>{step.hint}</span>
            </div>
          )}

          <div className="mt-4 flex items-center gap-2">
            <QuietButton onClick={onClose}>
              {step.intro ? "Esc 跳过" : step.outro ? "稍后再看" : "跳过"}
            </QuietButton>
            <div className="flex-1" />
            {step.intro || step.outro ? (
              <Button data-tour-primary variant="primary" onClick={step.intro ? next : onClose}>
                {step.intro ? "开始了解" : "开始使用"}
                <ArrowRight className="h-4 w-4" />
              </Button>
            ) : (
              <>
                <Progress current={stepIndex} total={TOUR_STEPS.length} />
                <Button
                  data-tour-primary={stepIndex === 0 ? true : undefined}
                  variant="primary"
                  onClick={next}
                >
                  {stepIndex === TOUR_STEPS.length - 1 ? "完成" : "下一步"}
                  <ArrowRight className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  className="border border-[var(--border)]"
                  onClick={back}
                  disabled={stepIndex === 0}
                  aria-label="上一步"
                  title="上一步 (←)"
                >
                  <ArrowLeft className="h-4 w-4" />
                </Button>
              </>
            )}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}

/** The quiet "skip" affordance: present, never competing with the primary. */
function QuietButton({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="rounded text-[12.5px] opacity-80 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
      style={{ color: "#a3a3b0" }}
    >
      {children}
    </button>
  );
}

/**
 * One dimming pane around the cut-out. It takes pointer events on purpose: the
 * dimmed page must not be clicked through mid-tour. The cursor stays an arrow —
 * clicking here does nothing rather than skipping by accident.
 */
function Pane({ style }: { style: React.CSSProperties }) {
  return (
    <div
      className="pointer-events-auto fixed cursor-default"
      style={{ ...style, background: DIM }}
      aria-hidden
    />
  );
}

function Progress({ current, total }: { current: number; total: number }) {
  return (
    <div
      className="flex items-center gap-1"
      role="img"
      aria-label={`第 ${current + 1} 步，共 ${total} 步`}
      title={`第 ${current + 1} 步 / 共 ${total} 步`}
    >
      {Array.from({ length: total }, (_, i) => (
        <span
          key={i}
          className="h-1 rounded-full transition-all duration-200"
          style={{
            width: i === current ? 14 : 5,
            background: i === current ? "var(--accent)" : "var(--border)",
          }}
        />
      ))}
    </div>
  );
}
