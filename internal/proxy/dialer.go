package proxy

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
	"github.com/kevinyuzekai/LinkPool/internal/rules"
	"github.com/kevinyuzekai/LinkPool/internal/scheduler"
	"github.com/kevinyuzekai/LinkPool/internal/stats"
)

// Dialer dials outbound connections with source-IP binding via the scheduler.
type Dialer struct {
	Sched   *scheduler.Scheduler
	Rules   *rules.Matcher
	Stats   *stats.Registry
	Timeout time.Duration
}

func (d *Dialer) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return 30 * time.Second
}

// DialContext picks an adapter (unless host is bypassed) and dials with LocalAddr bind.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	if d.Rules != nil && d.Rules.Match(host) {
		c, err := d.dialDefault(ctx, network, address)
		return c, "", err
	}

	ad, err := d.Sched.Next()
	if err != nil {
		return nil, "", err
	}
	c, err := d.dialBound(ctx, network, address, ad)
	if err != nil {
		if d.Stats != nil {
			d.Stats.AddError(ad.ID)
		}
		return nil, ad.ID, err
	}
	if d.Stats != nil {
		d.Stats.ConnOpen(ad.ID)
	}
	return &trackedConn{Conn: c, id: ad.ID, stats: d.Stats}, ad.ID, nil
}

func (d *Dialer) dialDefault(ctx context.Context, network, address string) (net.Conn, error) {
	nd := &net.Dialer{Timeout: d.timeout()}
	return nd.DialContext(ctx, network, address)
}

func (d *Dialer) dialBound(ctx context.Context, network, address string, ad adapter.Adapter) (net.Conn, error) {
	ip := ad.Addr()
	if ip == nil {
		return nil, fmt.Errorf("invalid adapter IPv4 %q", ad.IPv4)
	}
	local := &net.TCPAddr{IP: ip}
	nd := &net.Dialer{
		Timeout:   d.timeout(),
		LocalAddr: local,
	}
	return nd.DialContext(ctx, network, address)
}

// BindSelect picks LocalAddr for tests / diagnostics.
func BindSelect(ad adapter.Adapter) (*net.TCPAddr, error) {
	ip := ad.Addr()
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("invalid IPv4: %s", ad.IPv4)
	}
	return &net.TCPAddr{IP: ip}, nil
}

type trackedConn struct {
	net.Conn
	id    string
	stats *stats.Registry
	closed bool
}

func (c *trackedConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 && c.stats != nil {
		c.stats.AddRecv(c.id, int64(n))
	}
	return n, err
}

func (c *trackedConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 && c.stats != nil {
		c.stats.AddSent(c.id, int64(n))
	}
	return n, err
}

func (c *trackedConn) Close() error {
	if !c.closed {
		c.closed = true
		if c.stats != nil {
			c.stats.ConnClose(c.id)
		}
	}
	return c.Conn.Close()
}
