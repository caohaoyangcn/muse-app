import { useCallback, useEffect, useState } from "react";
import Library from "./pages/Library";
import Settings from "./pages/Settings";
import GuidedTour from "./components/GuidedTour";
import { attachBackendEvents } from "./services/events";
import { getTourSeen, setTourSeen } from "./services/api";
import { useTour } from "./hooks/useTour";

export default function App() {
  // Top-level view switch: the library or the Settings page (left submenu /
  // right detail layout). Settings is opened from a button in the header.
  const [view, setView] = useState<"library" | "settings">("library");
  // Which Settings submenu is open. Owned here so the tour can take the user
  // to 目录监控 / 网络代理 and leave the page usable afterwards. The gear keeps
  // its old default (服务器); a first run lands on the orientation page and the
  // `?` button is the way back to 新手引导.
  const [settingsSection, setSettingsSection] = useState("server");

  // Whether this is the first launch. `null` while the flag is being read: the
  // tour is a single first-run moment, so it is app state in the database, not
  // a webview preference (a disposable or corrupted webview profile silently
  // drops localStorage writes, which used to replay the tour on every launch).
  const [firstRun, setFirstRun] = useState<boolean | null>(null);

  // One subscription for the whole window. Views read through useLiveQuery and
  // never see an event name, so switching between the library and settings does
  // not change what is watched.
  useEffect(() => attachBackendEvents(), []);

  useEffect(() => {
    let alive = true;
    getTourSeen()
      .then((seen) => {
        if (alive) setFirstRun(!seen);
      })
      // A failed read must not swallow the one introduction a new user gets.
      .catch(() => {
        if (alive) setFirstRun(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  // A first run opens Settings on the orientation page, so the gear does not
  // drop a new user into server configuration before the tour has explained it.
  useEffect(() => {
    if (firstRun) setSettingsSection("guide");
  }, [firstRun]);

  const { open, stepIndex, start, goTo, close } = useTour({
    autoOpen: firstRun === true,
    onStep: (step) => {
      setView(step.view);
      if (step.section) setSettingsSection(step.section);
    },
    // The tour ends where the work happens: back in the library.
    onSettle: () => setView("library"),
  });

  // Every exit (finished, skipped, Esc, close) records the tour so the next
  // launch starts in the library; a failed write only costs one replay.
  const dismissTour = useCallback(() => {
    close();
    void setTourSeen().catch(() => {});
  }, [close]);

  const openSettings = useCallback((section?: string) => {
    if (section) setSettingsSection(section);
    setView("settings");
  }, []);
  const closeSettings = useCallback(() => setView("library"), []);

  const tourProps = { open, stepIndex, onAdvance: goTo, onClose: dismissTour };

  if (view === "settings") {
    return (
      <>
        <Settings
          onClose={closeSettings}
          onStartTour={start}
          section={settingsSection}
          onSectionChange={setSettingsSection}
        />
        <GuidedTour {...tourProps} />
      </>
    );
  }
  return (
    <>
      <Library onOpenSettings={openSettings} />
      <GuidedTour {...tourProps} />
    </>
  );
}
