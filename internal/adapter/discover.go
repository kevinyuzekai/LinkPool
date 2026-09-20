package adapter

// Discoverer lists network adapters suitable for source binding.
type Discoverer interface {
	List() ([]Adapter, error)
}

// DefaultDiscoverer returns the platform discoverer.
func DefaultDiscoverer() Discoverer {
	return platformDiscoverer{}
}
