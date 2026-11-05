package settings

import (
	"database/sql"
	"errors"
	"runtime"
	"strings"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/windowstate"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open(database.DriverName, "file:"+t.TempDir()+"/app.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db)
}

// TestTourSeenDefault verifies a fresh install has not seen the tour.
func TestTourSeenDefault(t *testing.T) {
	s := newStore(t)
	seen, err := s.TourSeen()
	if err != nil {
		t.Fatalf("tour seen: %v", err)
	}
	if seen {
		t.Error("fresh install should not report the tour as seen")
	}
}

// TestMarkTourSeenPersists verifies the flag survives a restart: the store is
// rebuilt over the same database, which is what a new app process does.
func TestMarkTourSeenPersists(t *testing.T) {
	db, err := sql.Open(database.DriverName, "file:"+t.TempDir()+"/app.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := New(db).MarkTourSeen(); err != nil {
		t.Fatalf("mark tour seen: %v", err)
	}
	seen, err := New(db).TourSeen()
	if err != nil {
		t.Fatalf("tour seen: %v", err)
	}
	if !seen {
		t.Error("tour seen flag did not survive a new store")
	}
}

// TestMarkTourSeenIdempotent verifies replays do not corrupt the flag.
func TestMarkTourSeenIdempotent(t *testing.T) {
	s := newStore(t)
	for i := 0; i < 3; i++ {
		if err := s.MarkTourSeen(); err != nil {
			t.Fatalf("mark tour seen (%d): %v", i, err)
		}
	}
	seen, err := s.TourSeen()
	if err != nil {
		t.Fatalf("tour seen: %v", err)
	}
	if !seen {
		t.Error("tour should be seen after repeated marks")
	}
}

// TestServerPortDefault verifies an unset port falls back to the default.
func TestServerPortDefault(t *testing.T) {
	s := newStore(t)
	port, err := s.ServerPort()
	if err != nil {
		t.Fatalf("server port: %v", err)
	}
	if port != DefaultServerPort {
		t.Errorf("default port = %d, want %d", port, DefaultServerPort)
	}
	if DefaultServerPort <= 1023 {
		t.Error("default port should be non-well-known (>1023)")
	}
}

// TestSetAndReadServerPort verifies a configured port round-trips.
func TestSetAndReadServerPort(t *testing.T) {
	s := newStore(t)
	if err := s.SetServerPort(7788); err != nil {
		t.Fatalf("set port: %v", err)
	}
	port, err := s.ServerPort()
	if err != nil {
		t.Fatalf("read port: %v", err)
	}
	if port != 7788 {
		t.Errorf("port = %d, want 7788", port)
	}
}

// TestSetServerPortValidation verifies out-of-range ports are rejected and
// never persisted.
func TestSetServerPortValidation(t *testing.T) {
	s := newStore(t)
	for _, bad := range []int{0, -1, 1024 * 64, 70000} {
		if err := s.SetServerPort(bad); err == nil {
			t.Errorf("SetServerPort(%d) should error", bad)
		}
	}
	// The previous default must be untouched after failed writes.
	port, err := s.ServerPort()
	if err != nil {
		t.Fatalf("read port: %v", err)
	}
	if port != DefaultServerPort {
		t.Errorf("port after rejected writes = %d, want default %d", port, DefaultServerPort)
	}
	// Minimum and maximum bounds are accepted.
	if err := s.SetServerPort(1); err != nil {
		t.Errorf("SetServerPort(1): %v", err)
	}
	if err := s.SetServerPort(65535); err != nil {
		t.Errorf("SetServerPort(65535): %v", err)
	}
}

// TestWindowPlacementRoundTrip verifies the saved geometry survives a restart
// byte for byte, including a negative origin from a monitor left of the primary.
func TestWindowPlacementRoundTrip(t *testing.T) {
	s := newStore(t)
	if _, ok, err := s.WindowPlacement(); err != nil || ok {
		t.Fatalf("unset placement: ok=%v err=%v, want no placement", ok, err)
	}
	want := windowstate.Placement{Left: -1800, Top: 120, Width: 1333, Height: 777, Maximised: true}
	if err := s.SetWindowPlacement(want); err != nil {
		t.Fatalf("set placement: %v", err)
	}
	got, ok, err := s.WindowPlacement()
	if err != nil {
		t.Fatalf("read placement: %v", err)
	}
	if !ok {
		t.Fatal("placement not reported as stored")
	}
	if got != want {
		t.Errorf("placement = %+v, want %+v", got, want)
	}
}

// TestWindowPlacementIgnoresBadValues verifies corrupt or nonsensical stored
// values fall back to the default window instead of moving it somewhere absurd.
func TestWindowPlacementIgnoresBadValues(t *testing.T) {
	s := newStore(t)
	for _, raw := range []string{
		`{"left":`,
		`{"width":0,"height":720}`,
		`{"width":100000,"height":720}`,
	} {
		if err := s.set(keyWindowPlacement, raw); err != nil {
			t.Fatalf("seed placement: %v", err)
		}
		if _, ok, err := s.WindowPlacement(); err != nil || ok {
			t.Errorf("%s: ok=%v err=%v, want no placement", raw, ok, err)
		}
	}
	if err := s.SetWindowPlacement(windowstate.Placement{Width: -5, Height: 600}); err == nil {
		t.Error("SetWindowPlacement accepted an invalid size")
	}
}

func TestScrapeProxyRoundTrip(t *testing.T) {
	s := newStore(t)
	got, err := s.ScrapeProxy()
	if err != nil {
		t.Fatalf("read empty proxy: %v", err)
	}
	if got != "" {
		t.Fatalf("unset proxy = %q, want empty", got)
	}
	if err := s.SetScrapeProxy(" 127.0.0.1:7890 "); err != nil {
		t.Fatalf("set proxy: %v", err)
	}
	got, err = s.ScrapeProxy()
	if err != nil {
		t.Fatalf("read proxy: %v", err)
	}
	if got != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %q, want canonical http://127.0.0.1:7890", got)
	}
	if err := s.SetScrapeProxy(""); err != nil {
		t.Fatalf("clear proxy: %v", err)
	}
	if got, err = s.ScrapeProxy(); err != nil || got != "" {
		t.Fatalf("proxy after clear = %q err=%v, want empty", got, err)
	}
}

func TestScrapeProxyValidation(t *testing.T) {
	s := newStore(t)
	if err := s.SetScrapeProxy("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("set proxy: %v", err)
	}
	for _, tc := range []struct {
		name string
		in   string
	}{
		{name: "unsupported scheme", in: "ftp://127.0.0.1:21"},
		{name: "missing port", in: "http://127.0.0.1"},
		{name: "port out of range", in: "http://127.0.0.1:70000"},
		{name: "path", in: "http://127.0.0.1:7890/proxy"},
		{name: "newline", in: "http://127.0.0.1:7890\r\nX: y"},
		{name: "too long", in: "http://127.0.0.1:7890/" + strings.Repeat("a", 2048)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.SetScrapeProxy(tc.in); err == nil {
				t.Fatalf("SetScrapeProxy(%q) should reject input", tc.in)
			}
		})
	}
	// A rejected write must leave the previous value untouched.
	if got, err := s.ScrapeProxy(); err != nil || got != "http://127.0.0.1:7890" {
		t.Fatalf("proxy after rejected writes = %q err=%v", got, err)
	}
}

func TestSetJavBusCookieValidation(t *testing.T) {
	s := newStore(t)
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "whitespace", input: " \t\r\n "},
		{name: "carriage return", input: "PHPSESSID=abc\rdef"},
		{name: "line feed", input: "PHPSESSID=abc\ndef"},
		{name: "nul", input: "PHPSESSID=abc\x00def"},
		{name: "too long", input: strings.Repeat("a", maxJavBusCookieLength+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.SetJavBusCookie(tc.input); err == nil {
				t.Fatalf("SetJavBusCookie(%q) should reject input", tc.input)
			}
		})
	}
}

func TestFC2CookieValidation(t *testing.T) {
	s := newStore(t)
	for _, input := range []string{"", "bad\nvalue", "bad\rvalue", "bad\x00value", strings.Repeat("a", maxFC2CookieLength+1)} {
		if err := s.SetFC2Cookie(input); err == nil {
			t.Fatalf("SetFC2Cookie(%q) should reject input", input)
		}
	}
}

func TestFC2CookieStorage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI storage test requires Windows")
	}
	s := newStore(t)
	cookie := " ageVerified=true; fc2cmadb-session=session-value "
	want := strings.TrimSpace(cookie)
	if err := s.SetFC2Cookie(cookie); err != nil {
		t.Fatalf("set FC2 cookie: %v", err)
	}
	has, err := s.HasFC2Cookie()
	if err != nil || !has {
		t.Fatalf("has FC2 cookie=%v err=%v", has, err)
	}
	got, err := s.GetFC2Cookie()
	if err != nil || got != want {
		t.Fatalf("FC2 cookie=%q err=%v want=%q", got, err, want)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, keyFC2Cookie).Scan(&stored); err != nil {
		t.Fatalf("read stored FC2 setting: %v", err)
	}
	if strings.Contains(stored, want) {
		t.Fatal("database contains FC2 Cookie plaintext")
	}
	if err := s.ClearFC2Cookie(); err != nil {
		t.Fatalf("clear FC2 cookie: %v", err)
	}
	if has, _ := s.HasFC2Cookie(); has {
		t.Fatal("FC2 cookie remains configured after clear")
	}
}

func TestJavBusCookieStorage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI storage test requires Windows")
	}

	s := newStore(t)
	cookie := "  existmag=mag; PHPSESSID=session-value; forum_auth=abc  "
	want := strings.TrimSpace(cookie)
	if err := s.SetJavBusCookie(cookie); err != nil {
		t.Fatalf("set cookie: %v", err)
	}

	has, err := s.HasJavBusCookie()
	if err != nil {
		t.Fatalf("has cookie: %v", err)
	}
	if !has {
		t.Fatal("HasJavBusCookie = false, want true")
	}

	got, err := s.GetJavBusCookie()
	if err != nil {
		t.Fatalf("get cookie: %v", err)
	}
	if got != want {
		t.Fatalf("cookie = %q, want %q", got, want)
	}

	var stored string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, keyJavBusCookie).Scan(&stored); err != nil {
		t.Fatalf("read stored setting: %v", err)
	}
	if stored == "" {
		t.Fatal("stored protected cookie is empty")
	}
	if strings.Contains(stored, want) {
		t.Fatal("database contains the Cookie plaintext")
	}

	if err := s.ClearJavBusCookie(); err != nil {
		t.Fatalf("clear cookie: %v", err)
	}
	has, err = s.HasJavBusCookie()
	if err != nil {
		t.Fatalf("has cookie after clear: %v", err)
	}
	if has {
		t.Fatal("HasJavBusCookie after clear = true, want false")
	}
	got, err = s.GetJavBusCookie()
	if err != nil {
		t.Fatalf("get cookie after clear: %v", err)
	}
	if got != "" {
		t.Fatalf("cookie after clear = %q, want empty", got)
	}
}

func TestFC2CookieUnsupportedOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unsupported-platform test requires non-Windows")
	}
	s := newStore(t)
	for name, call := range map[string]func() error{
		"get":   func() error { _, err := s.GetFC2Cookie(); return err },
		"set":   func() error { return s.SetFC2Cookie("ageVerified=true") },
		"has":   func() error { _, err := s.HasFC2Cookie(); return err },
		"clear": s.ClearFC2Cookie,
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrSecretStorageUnsupported) {
				t.Fatalf("error = %v, want ErrSecretStorageUnsupported", err)
			}
		})
	}
}

func TestJavBusCookieUnsupportedOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unsupported-platform test requires non-Windows")
	}

	s := newStore(t)
	for name, call := range map[string]func() error{
		"get":   func() error { _, err := s.GetJavBusCookie(); return err },
		"set":   func() error { return s.SetJavBusCookie("PHPSESSID=session") },
		"has":   func() error { _, err := s.HasJavBusCookie(); return err },
		"clear": s.ClearJavBusCookie,
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrSecretStorageUnsupported) {
				t.Fatalf("error = %v, want ErrSecretStorageUnsupported", err)
			}
		})
	}
}
