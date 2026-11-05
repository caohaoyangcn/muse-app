package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsRFC1918(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.10", true},
		{"10.0.0.5", true},
		{"172.16.0.1", true},
		{"172.31.255.1", true},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"172.15.0.1", false},
		{"198.18.0.1", false},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
	}
	for _, c := range cases {
		if got := isRFC1918(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("isRFC1918(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestStartDoesNotWaitForFirewallConfiguration(t *testing.T) {
	s := New(nil, t.TempDir(), slog.New(slog.DiscardHandler))
	// Bind loopback, not every interface. A wildcard bind is what makes Windows
	// Firewall ask whether to allow this test binary — and because each build
	// lands in a new temp path, the dialog comes back on every run. The
	// production address is asserted in the configured-port test below.
	s.listen = func(string, string) (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	s.firewall = func(context.Context, int) error {
		close(started)
		<-release
		return nil
	}
	defer func() {
		close(release)
		_ = s.Stop()
	}()

	start := time.Now()
	info, err := s.Start(0)
	if err != nil {
		t.Fatal(err)
	}
	// A fast-fail for the bug this test guards: Start must not call the firewall
	// helper synchronously (that call gets a 5s context). The budget is not a
	// performance assertion — enumerating network interfaces can take a few
	// hundred milliseconds on a machine with VPN or virtual adapters.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Start waited %s for firewall configuration", elapsed)
	}
	if !info.Running || info.Port == 0 {
		t.Fatalf("start info = %+v", info)
	}

	conn, err := net.DialTimeout("tcp", s.addr, time.Second)
	if err != nil {
		t.Fatalf("HTTP listener is not reachable: %v", err)
	}
	_ = conn.Close()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("firewall configuration was not scheduled")
	}
}

func TestStartDoesNotFallBackWhenConfiguredPortCannotBeBound(t *testing.T) {
	s := New(nil, t.TempDir(), slog.New(slog.DiscardHandler))
	var calls int
	s.listen = func(network, address string) (net.Listener, error) {
		calls++
		if address != "0.0.0.0:8900" {
			t.Fatalf("listen address = %q, want configured port", address)
		}
		return nil, errors.New("bind: configured port is unavailable")
	}
	t.Cleanup(func() { _ = s.Stop() })

	info, err := s.Start(8900)
	if err == nil {
		t.Fatal("Start should report the configured port bind error")
	}
	if info.Running || info.Port != 0 {
		t.Fatalf("start info = %+v, want a stopped server", info)
	}
	if !strings.Contains(err.Error(), "无法监听端口 8900") {
		t.Fatalf("error = %v, want configured port in message", err)
	}
	if calls != 1 {
		t.Fatalf("listen calls = %d, want one configured-port attempt", calls)
	}
}
