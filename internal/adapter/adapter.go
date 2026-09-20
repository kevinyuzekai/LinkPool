package adapter

import "net"

// Type guesses the kind of network interface.
type Type string

const (
	TypeEthernet Type = "ethernet"
	TypeWiFi     Type = "wifi"
	TypeCellular Type = "cellular"
	TypeVPN      Type = "vpn"
	TypeLoopback Type = "loopback"
	TypeOther    Type = "other"
)

// Adapter describes a usable network interface with an IPv4 address.
type Adapter struct {
	ID        string `json:"id"`         // stable key (iface name)
	Name      string `json:"name"`       // display name
	IfaceName string `json:"ifaceName"`  // OS interface name
	IPv4      string `json:"ipv4"`       // primary IPv4
	Type      Type   `json:"type"`
	Up        bool   `json:"up"`
	Selected  bool   `json:"selected"`
	Weight    int    `json:"weight"` // relative weight for scheduling (>=1)
}

// Addr returns the IPv4 as net.IP, or nil if invalid.
func (a Adapter) Addr() net.IP {
	return net.ParseIP(a.IPv4)
}

// GuessType infers adapter type from interface name heuristics.
func GuessType(name string) Type {
	n := name
	switch {
	case n == "lo" || n == "lo0":
		return TypeLoopback
	case hasPrefix(n, "en") || hasPrefix(n, "eth") || hasPrefix(n, "lan"):
		// On macOS en0 is often Wi‑Fi; refined by Discover on darwin when possible.
		return TypeEthernet
	case hasPrefix(n, "wl") || hasPrefix(n, "wifi") || hasPrefix(n, "airport"):
		return TypeWiFi
	case hasPrefix(n, "utun") || hasPrefix(n, "tun") || hasPrefix(n, "tap") ||
		hasPrefix(n, "ipsec") || hasPrefix(n, "ppp") || hasPrefix(n, "wg"):
		return TypeVPN
	case hasPrefix(n, "pdp") || hasPrefix(n, "wwan") || hasPrefix(n, "rmnet") ||
		hasPrefix(n, "cellular"):
		return TypeCellular
	default:
		return TypeOther
	}
}

func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
