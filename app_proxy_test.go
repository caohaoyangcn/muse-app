package main

import (
	"database/sql"
	"net/http"
	"testing"

	"muse-app/backend/database"
	"muse-app/backend/netproxy"
	"muse-app/backend/settings"
)

func newProxyTestApp(t *testing.T) (*App, *netproxy.Config) {
	t.Helper()
	db, err := sql.Open(database.DriverName, "file:"+t.TempDir()+"/app.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := netproxy.New()
	app := NewApp("", nil)
	app.SetSettings(settings.New(db))
	app.setScrapeProxyConfig(cfg)
	return app, cfg
}

// TestSetScrapeProxyAppliesAndPersists covers the binding path: the address is
// validated, stored canonically, and the live transports see it immediately.
func TestSetScrapeProxyAppliesAndPersists(t *testing.T) {
	app, cfg := newProxyTestApp(t)
	if err := app.SetScrapeProxy(" 127.0.0.1:7890 "); err != nil {
		t.Fatalf("set proxy: %v", err)
	}
	got, err := app.GetScrapeProxy()
	if err != nil {
		t.Fatalf("get proxy: %v", err)
	}
	if got != "http://127.0.0.1:7890" {
		t.Fatalf("stored proxy = %q, want canonical http://127.0.0.1:7890", got)
	}

	req, err := http.NewRequest(http.MethodGet, "https://javbus.com/ABP-576", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if proxy, err := cfg.ProxyFunc(req); err != nil || proxy == nil || proxy.Host != "127.0.0.1:7890" {
		t.Fatalf("live proxy = (%v, %v), want 127.0.0.1:7890", proxy, err)
	}

	// A rejected address must not change either the stored or the live value.
	if err := app.SetScrapeProxy("ftp://127.0.0.1:21"); err == nil {
		t.Fatal("unsupported scheme should be rejected")
	}
	if got, _ := app.GetScrapeProxy(); got != "http://127.0.0.1:7890" {
		t.Fatalf("proxy after rejected write = %q", got)
	}

	if err := app.SetScrapeProxy(""); err != nil {
		t.Fatalf("clear proxy: %v", err)
	}
	if got, _ := app.GetScrapeProxy(); got != "" {
		t.Fatalf("proxy after clear = %q, want empty", got)
	}
	if proxy, err := cfg.ProxyFunc(req); err != nil || proxy != nil {
		t.Fatalf("live proxy after clear = (%v, %v), want nil", proxy, err)
	}
}
