//go:build darwin

package adapter

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// platformDiscoverer lists adapters using net.Interfaces plus networksetup
// for friendlier service names on macOS.
type platformDiscoverer struct{}

func (platformDiscoverer) List() ([]Adapter, error) {
	serviceNames := mapHardwareToService()

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
		display := iface.Name
		if svc, ok := serviceNames[iface.Name]; ok && svc != "" {
			display = svc
		}
		t := guessDarwinType(iface.Name, display)
		out = append(out, Adapter{
			ID:        iface.Name,
			Name:      display,
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

func guessDarwinType(ifaceName, display string) Type {
	d := strings.ToLower(display)
	switch {
	case strings.Contains(d, "wi-fi") || strings.Contains(d, "wifi") || strings.Contains(d, "airport"):
		return TypeWiFi
	case strings.Contains(d, "ethernet") || strings.Contains(d, "usb") || strings.Contains(d, "thunderbolt"):
		return TypeEthernet
	case strings.Contains(d, "iphone") || strings.Contains(d, "ipad") || strings.Contains(d, "cellular"):
		return TypeCellular
	case strings.HasPrefix(ifaceName, "utun") || strings.HasPrefix(ifaceName, "ipsec") ||
		strings.Contains(d, "vpn"):
		return TypeVPN
	default:
		return GuessType(ifaceName)
	}
}

// mapHardwareToService builds hardware port -> network service name via networksetup.
func mapHardwareToService() map[string]string {
	out := map[string]string{}
	cmd := exec.Command("networksetup", "-listallhardwareports")
	b, err := cmd.Output()
	if err != nil {
		return out
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	var currentPort string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "Hardware Port:") {
			currentPort = strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
			continue
		}
		if strings.HasPrefix(line, "Device:") && currentPort != "" {
			dev := strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
			out[dev] = currentPort
			currentPort = ""
		}
	}
	return out
}
