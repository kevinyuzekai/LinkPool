package rules

import (
	"strings"
	"sync"
)

// Matcher decides whether a host should bypass the proxy (go direct / OS default).
// Supports exact hostnames and suffix rules (leading '.' or '*.').
type Matcher struct {
	mu     sync.RWMutex
	exact  map[string]struct{}
	suffix []string // stored without leading '.'
}

// New creates an empty matcher.
func New() *Matcher {
	return &Matcher{exact: map[string]struct{}{}}
}

// SetRules replaces all rules. Blank lines and comments (#) are ignored.
// Examples: "localhost", ".apple.com", "*.local"
func (m *Matcher) SetRules(lines []string) {
	exact := map[string]struct{}{}
	var suffix []string
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.ToLower(line)
		if strings.HasPrefix(line, "*.") {
			suffix = append(suffix, line[2:])
			continue
		}
		if strings.HasPrefix(line, ".") {
			suffix = append(suffix, line[1:])
			continue
		}
		exact[line] = struct{}{}
	}
	m.mu.Lock()
	m.exact = exact
	m.suffix = suffix
	m.mu.Unlock()
}

// SetFromText splits text by newlines and applies SetRules.
func (m *Matcher) SetFromText(text string) {
	m.SetRules(strings.Split(text, "\n"))
}

// Match reports whether host (without port) should bypass aggregation.
func (m *Matcher) Match(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	// strip brackets for IPv6 literals if present
	host = strings.Trim(host, "[]")

	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.exact[host]; ok {
		return true
	}
	for _, sfx := range m.suffix {
		if host == sfx || strings.HasSuffix(host, "."+sfx) {
			return true
		}
	}
	return false
}

// List returns current rule strings for display (exact + suffix with leading '.').
func (m *Matcher) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.exact)+len(m.suffix))
	for e := range m.exact {
		out = append(out, e)
	}
	for _, s := range m.suffix {
		out = append(out, "."+s)
	}
	return out
}

// DefaultBypass returns sensible defaults for local / Apple services.
func DefaultBypass() []string {
	return []string{
		"localhost",
		"127.0.0.1",
		"::1",
		".local",
		".apple.com",
		".icloud.com",
		".mzstatic.com",
	}
}
