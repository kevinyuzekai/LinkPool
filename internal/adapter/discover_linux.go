//go:build linux

package adapter

import (
	"fmt"
	"net"
)

// platformDiscoverer lists adapters via Go's net package (limited; for local tests).
type platformDiscoverer struct{}

func (platformDiscoverer) List() ([]Adapter, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var ipv4 string
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			if ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			ipv4 = ipNet.IP.String()
			break
		}
		if ipv4 == "" {
			continue
		}
		up := iface.Flags&net.FlagUp != 0
		t := GuessType(iface.Name)
		out = append(out, Adapter{
			ID:        iface.Name,
			Name:      iface.Name,
			IfaceName: iface.Name,
			IPv4:      ipv4,
			Type:      t,
			Up:        up,
			Selected:  false,
			Weight:    1,
		})
	}
	if len(out) == 0 {
		return out, fmt.Errorf("no usable IPv4 adapters found")
	}
	return out, nil
}
