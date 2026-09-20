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
// On dialBound failure it fails over to other adapters.
type Dialer struct {
	Sched   *scheduler.Scheduler
	Rules   *rules.Matcher
	Stats   *stats.Registry
	Timeout time.Duration
	// MaxFailover is max adapters to try (including the first). 0 = all once.
	MaxFailover int
}

func (d *Dialer) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return 15 * time.Second
}

// DialContext picks an adapter (unless host is bypassed) and dials with LocalAddr bind.
// If bind dial fails, tries further scheduler picks before returning error.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	if d.Rules != nil && d.Rules.Match(host) {
		c, err := d.dialDefault(ctx, network, address)
		return c, "", err
	}

	if d.Sched == nil {
		return nil, "", scheduler.ErrNoAdapters
	}

	maxTries := d.MaxFailover
	if maxTries <= 0 {
		maxTries = d.Sched.Len()
	}
	if maxTries < 1 {
		maxTries = 1
	}

	tried := map[string]bool{}
	var lastErr error
	var lastID string

	for attempt := 0; attempt < maxTries; attempt++ {
		ad, err := d.Sched.NextExcluding(tried)
		if err != nil {
			if lastErr != nil {
				return nil, lastID, lastErr
			}
			return nil, "", err
		}
		tried[ad.ID] = true
		lastID = ad.ID

		c, err := d.dialBound(ctx, network, address, ad)
		if err != nil {
			if d.Stats != nil {
				d.Stats.AddError(ad.ID)
			}
			d.Sched.RecordDialFailure(ad.ID)
			lastErr = err
			continue
		}
		d.Sched.RecordDialSuccess(ad.ID)
		if d.Stats != nil {
			d.Stats.ConnOpen(ad.ID)
		}
		return &trackedConn{Conn: c, id: ad.ID, stats: d.Stats}, ad.ID, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("dial failed on all adapters")
	}
	return nil, lastID, lastErr
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

// DialBoundOn dials with a specific adapter (no scheduler / failover). Used by multi-range.
func (d *Dialer) DialBoundOn(ctx context.Context, network, address string, ad adapter.Adapter) (net.Conn, error) {
	c, err := d.dialBound(ctx, network, address, ad)
	if err != nil {
		if d.Stats != nil {
			d.Stats.AddError(ad.ID)
		}
		if d.Sched != nil {
			d.Sched.RecordDialFailure(ad.ID)
		}
		return nil, err
	}
	if d.Sched != nil {
		d.Sched.RecordDialSuccess(ad.ID)
	}
	if d.Stats != nil {
		d.Stats.ConnOpen(ad.ID)
	}
	return &trackedConn{Conn: c, id: ad.ID, stats: d.Stats}, nil
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
	id     string
	stats  *stats.Registry
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
