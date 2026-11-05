//go:build windows

package platform

import (
	"context"
	"fmt"
	"os"
)

// EnsureInboundRule makes an inbound TCP allow rule in Windows Firewall for the
// current executable on the given port, so phones on the LAN can reach the
// server. It is best-effort: adding a rule needs Administrator rights (the call
// elevates via `netsh advfirewall`, which prompts UAC or fails with a non-zero
// exit when not elevated). The caller treats any failure as non-fatal and tells
// the user to check the firewall manually if LAN access still times out.
//
// The rule is scoped to the exact binary and port to avoid opening the machine
// broadly. If a rule already exists for this program:port, it is replaced so
// rebinding a different port keeps Firewall in sync.
func EnsureInboundRule(port int) error {
	return EnsureInboundRuleContext(context.Background(), port)
}

// EnsureInboundRuleContext is the cancellable form used by the UI-facing
// server startup path. Firewall configuration is best-effort and must not
// keep the HTTP listener or its caller blocked indefinitely.
func EnsureInboundRuleContext(ctx context.Context, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	// Rule name must be stable across builds of the same program.
	name := fmt.Sprintf("Muse Video LAN Web (%s:%d)", exe, port)

	// Remove a stale rule for an older port of this program, then add the new
	// one. `netsh` is hidden but still needs elevation.
	_, _ = runNetshContext(ctx, "advfirewall", "firewall", "delete", "rule",
		`name=`+name, `program=`+exe)

	if out, err := runNetshContext(ctx, "advfirewall", "firewall", "add", "rule",
		`name=`+name,
		`dir=in`,
		`action=allow`,
		`protocol=TCP`,
		`localport=`+fmt.Sprint(port),
		`program=`+exe,
		`profile=any`,
		`enable=yes`); err != nil {
		return fmt.Errorf("set firewall rule (run the app as Administrator if this fails): %s", out)
	}
	return nil
}

func runNetshContext(ctx context.Context, args ...string) (string, error) {
	cmd := CommandContext(ctx, "netsh", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
