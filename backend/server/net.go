package server

import (
	"net"
)

// LanIPv4s returns the non-loopback IPv4 addresses to advertise for LAN access,
// private (RFC1918) addresses first, so the printed URL is meaningful to a
// phone on the same network.
//
// Two things make naive enumeration unreliable on a desktop:
//
//   - net.InterfaceAddrs() reports an address for every adapter, even ones that
//     are down, disconnected, or have no DHCP. Those adapters carry link-local
//     APIPA addresses (169.254/16) which are only valid within a single link
//     and can never be reached by another device — serving one produces
//     ERR_ADDRESS_UNREACHABLE. So we enumerate net.Interfaces() and keep only
//     adapters whose flag is Up, which the OS sets only for a connected link.
//   - A VPN/proxy adapter may hold a non-RFC1918 address (e.g. 198.18.x.x on
//     the "Meta" TUN adapter). Such addresses are not routable to a phone, so
//     we NEVER fall back to them; if no usable private address exists, we
//     return nothing and the UI shows a clear "no reachable LAN address"
//     message instead of a doomed URL.
//
// The slice orders private addresses first; within a private group the OS
// interface order is preserved.
func LanIPv4s() []string {
	var privates []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for i := range ifaces {
		ifc := &ifaces[i]
		// Up is the "link is connected" flag; it excludes the disconnected /
		// APIPA-only / bluetooth adapters that dominate a desktop's list.
		if ifc.Flags&net.FlagUp == 0 {
			continue
		}
		// Loopback has no usable peer address.
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			if !isRFC1918(ip4) {
				continue // link-local, CGNAT, VPN/proxy ranges ⇒ not phone-reachable
			}
			privates = append(privates, ip4.String())
		}
	}
	return privates
}

// LanIPv4 returns the first candidate or "".
func LanIPv4() string {
	list := LanIPv4s()
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

// isRFC1918 reports whether ip is globally unique private space that a LAN
// peer can route to: 10/8, 172.16/12, 192.168/16. Link-local (169.254/16) and
// other special ranges are deliberately excluded — they mean "no DHCP" on
// desktop hosts and are not reachable across the network.
func isRFC1918(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return ip4[0] == 10 ||
		(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] < 32) ||
		(ip4[0] == 192 && ip4[1] == 168)
}
