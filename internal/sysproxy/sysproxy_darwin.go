//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Manager best-effort sets / restores macOS system HTTP & SOCKS proxy via networksetup.
type Manager struct {
	mu       sync.Mutex
	enabled  bool
	services []string
	backup   map[string]serviceBackup
}

type serviceBackup struct {
	webOn, secureOn, socksOn bool
	webHost, webPort         string
	secureHost, securePort   string
	socksHost, socksPort     string
}

func New() *Manager {
	return &Manager{backup: map[string]serviceBackup{}}
}

func (m *Manager) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

// Enable sets system proxy on all hardware network services that look active.
func (m *Manager) Enable(httpHost string, httpPort int, socksHost string, socksPort int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.enabled {
		return nil
	}
	services, err := listNetworkServices()
	if err != nil {
		return err
	}
	m.backup = map[string]serviceBackup{}
	m.services = nil
	var lastErr error
	for _, svc := range services {
		if svc == "Thunderbolt Bridge" || strings.Contains(svc, "*") {
			continue
		}
		b, err := snapshotService(svc)
		if err != nil {
			lastErr = err
			continue
		}
		m.backup[svc] = b
		if err := applyService(svc, httpHost, httpPort, socksHost, socksPort); err != nil {
			lastErr = err
			continue
		}
		m.services = append(m.services, svc)
	}
	if len(m.services) == 0 {
		if lastErr != nil {
			return lastErr
		}
		return fmt.Errorf("no network services updated")
	}
	m.enabled = true
	return nil
}

// Disable restores previous proxy settings.
func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.enabled {
		return nil
	}
	var lastErr error
	for _, svc := range m.services {
		b := m.backup[svc]
		if err := restoreService(svc, b); err != nil {
			lastErr = err
		}
	}
	m.enabled = false
	m.services = nil
	m.backup = map[string]serviceBackup{}
	return lastErr
}

func listNetworkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	var svcs []string
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if i == 0 && strings.HasPrefix(line, "An asterisk") {
			continue
		}
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		svcs = append(svcs, line)
	}
	return svcs, nil
}

func snapshotService(svc string) (serviceBackup, error) {
	var b serviceBackup
	web, err := exec.Command("networksetup", "-getwebproxy", svc).Output()
	if err != nil {
		return b, err
	}
	b.webOn, b.webHost, b.webPort = parseProxyGet(string(web))
	sec, err := exec.Command("networksetup", "-getsecurewebproxy", svc).Output()
	if err != nil {
		return b, err
	}
	b.secureOn, b.secureHost, b.securePort = parseProxyGet(string(sec))
	socks, err := exec.Command("networksetup", "-getsocksfirewallproxy", svc).Output()
	if err != nil {
		return b, err
	}
	b.socksOn, b.socksHost, b.socksPort = parseProxyGet(string(socks))
	return b, nil
}

func parseProxyGet(out string) (enabled bool, server, port string) {
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		switch k {
		case "Enabled":
			enabled = strings.EqualFold(v, "Yes")
		case "Server":
			server = v
		case "Port":
			port = v
		}
	}
	return
}

func applyService(svc, httpHost string, httpPort int, socksHost string, socksPort int) error {
	hp := fmt.Sprintf("%d", httpPort)
	sp := fmt.Sprintf("%d", socksPort)
	cmds := [][]string{
		{"networksetup", "-setwebproxy", svc, httpHost, hp},
		{"networksetup", "-setsecurewebproxy", svc, httpHost, hp},
		{"networksetup", "-setsocksfirewallproxy", svc, socksHost, sp},
		{"networksetup", "-setwebproxystate", svc, "on"},
		{"networksetup", "-setsecurewebproxystate", svc, "on"},
		{"networksetup", "-setsocksfirewallproxystate", svc, "on"},
	}
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w (%s)", c, err, string(out))
		}
	}
	return nil
}

func restoreService(svc string, b serviceBackup) error {
	// Turn off first, then restore values if previously on.
	_ = exec.Command("networksetup", "-setwebproxystate", svc, "off").Run()
	_ = exec.Command("networksetup", "-setsecurewebproxystate", svc, "off").Run()
	_ = exec.Command("networksetup", "-setsocksfirewallproxystate", svc, "off").Run()

	if b.webHost != "" {
		_ = exec.Command("networksetup", "-setwebproxy", svc, b.webHost, orPort(b.webPort)).Run()
	}
	if b.secureHost != "" {
		_ = exec.Command("networksetup", "-setsecurewebproxy", svc, b.secureHost, orPort(b.securePort)).Run()
	}
	if b.socksHost != "" {
		_ = exec.Command("networksetup", "-setsocksfirewallproxy", svc, b.socksHost, orPort(b.socksPort)).Run()
	}
	if b.webOn {
		_ = exec.Command("networksetup", "-setwebproxystate", svc, "on").Run()
	}
	if b.secureOn {
		_ = exec.Command("networksetup", "-setsecurewebproxystate", svc, "on").Run()
	}
	if b.socksOn {
		_ = exec.Command("networksetup", "-setsocksfirewallproxystate", svc, "on").Run()
	}
	return nil
}

func orPort(p string) string {
	if p == "" {
		return "0"
	}
	return p
}
