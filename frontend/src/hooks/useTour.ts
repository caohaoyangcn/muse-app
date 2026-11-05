import { useCallback, useEffect, useRef, useState } from "react";
import { TOUR_STEPS, type TourStep } from "../utils/tour";

/**
 * State machine for the guided tour.
 *
 * The tour is a sequence of steps that cross screens (the library and the
 * Settings page), so the shell owns two contracts around it:
 *
 * - `onStep` runs while the tour is open and its step becomes current. The
 *   shell switches to that step's screen and opens its sub-section; the tour
 *   measures its target with a short poll while that render lands.
 * - `onSettle` runs once the tour closes. Landing back on the library is
 *   deliberate: most people close the tour on the settings screen, and the
 *   library is where the actual work happens.
 */
export function useTour({
  onStep,
  onSettle,
  autoOpen = false,
}: {
  onStep: (step: TourStep) => void;
  onSettle: () => void;
  /** Start on mount — the first launch of a fresh install. */
  autoOpen?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [stepIndex, setStepIndex] = useState(0);

  // The shell's callbacks are read through refs: switching screens re-creates
  // them, and an effect keyed on their identity would re-run mid-tour.
  const stepHandler = useRef(onStep);
  stepHandler.current = onStep;
  const settleHandler = useRef(onSettle);
  settleHandler.current = onSettle;
  const wasOpen = useRef(false);

  const start = useCallback(() => {
    // A replay starts from the beginning; the automatic first launch is
    // already there but resetting keeps the two paths identical.
    setStepIndex(0);
    setOpen(true);
  }, []);

  const close = useCallback(() => setOpen(false), []);

  // Step navigation. The renderer asks for the next index; the shell sees the
  // change through onStep and switches screens before the tour measures.
  const goTo = useCallback((index: number) => {
    setStepIndex(Math.max(0, Math.min(index, TOUR_STEPS.length - 1)));
  }, []);

  // Announce every step while open. This is the only place the shell is told
  // which screen to show, so closing the tour never moves the user.
  useEffect(() => {
    if (!open) return;
    wasOpen.current = true;
    stepHandler.current(TOUR_STEPS[Math.min(stepIndex, TOUR_STEPS.length - 1)]);
  }, [open, stepIndex]);

  // One settle after any close, not per step.
  useEffect(() => {
    if (open || !wasOpen.current) return;
    wasOpen.current = false;
    settleHandler.current();
  }, [open]);

  useEffect(() => {
    if (!autoOpen) return;
    const timer = window.setTimeout(start, 700);
    return () => window.clearTimeout(timer);
  }, [autoOpen, start]);

  return { open, stepIndex, start, goTo, close };
}
