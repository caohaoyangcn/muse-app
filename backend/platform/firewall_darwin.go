//go:build darwin

package platform

import "context"

// EnsureInboundRule is intentionally a no-op on macOS. The system firewall
// prompts the user for incoming connections and this app does not modify it.
func EnsureInboundRule(port int) error { return nil }

func EnsureInboundRuleContext(ctx context.Context, port int) error { return nil }
