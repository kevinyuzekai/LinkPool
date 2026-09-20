//go:build !linux && !darwin

package adapter

import "fmt"

type platformDiscoverer struct{}

func (platformDiscoverer) List() ([]Adapter, error) {
	return nil, fmt.Errorf("adapter discovery not implemented on this platform")
}
