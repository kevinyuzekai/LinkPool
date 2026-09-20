package proxy

import (
	"net"
	"testing"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
)

func TestBindSelectValid(t *testing.T) {
	addr, err := BindSelect(adapter.Adapter{IPv4: "192.168.1.10"})
	if err != nil {
		t.Fatal(err)
	}
	if addr.IP.String() != "192.168.1.10" {
		t.Fatalf("got %v", addr.IP)
	}
	if addr.IP.To4() == nil {
		t.Fatal("expected IPv4")
	}
}

func TestBindSelectInvalid(t *testing.T) {
	_, err := BindSelect(adapter.Adapter{IPv4: "not-an-ip"})
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = BindSelect(adapter.Adapter{IPv4: "2001:db8::1"})
	if err == nil {
		t.Fatal("expected error for IPv6-only")
	}
}

func TestBindSelectLocalAddrType(t *testing.T) {
	addr, err := BindSelect(adapter.Adapter{IPv4: "10.0.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	var _ net.Addr = addr
}
