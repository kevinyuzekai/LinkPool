//go:build !darwin

package sysproxy

import "fmt"

// Manager is a no-op stub outside macOS.
type Manager struct {
	enabled bool
}

func New() *Manager { return &Manager{} }

func (m *Manager) Enabled() bool { return m.enabled }

func (m *Manager) Enable(httpHost string, httpPort int, socksHost string, socksPort int) error {
	_ = httpHost
	_ = httpPort
	_ = socksHost
	_ = socksPort
	return fmt.Errorf("system proxy is only supported on macOS (darwin); build with GOOS=darwin and test on a Mac")
}

func (m *Manager) Disable() error {
	m.enabled = false
	return nil
}
