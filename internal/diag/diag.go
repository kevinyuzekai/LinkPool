package diag

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
)

// Result of a source-IP diagnostic request.
type Result struct {
	AdapterID string `json:"adapterId"`
	Name      string `json:"name"`
	LocalIP   string `json:"localIp"`
	PublicIP  string `json:"publicIp,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latencyMs"`
}

// CheckFromAdapter dials an HTTP echo endpoint bound to the adapter's IPv4.
func CheckFromAdapter(ctx context.Context, ad adapter.Adapter, echoURL string) Result {
	if echoURL == "" {
		echoURL = "https://api.ipify.org"
	}
	res := Result{AdapterID: ad.ID, Name: ad.Name, LocalIP: ad.IPv4}
	ip := ad.Addr()
	if ip == nil {
		res.Error = "invalid local IPv4"
		return res
	}
	start := time.Now()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 10 * time.Second, LocalAddr: &net.TCPAddr{IP: ip}}
			return d.DialContext(ctx, network, address)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, echoURL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	resp, err := client.Do(req)
	res.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
	if resp.StatusCode != 200 {
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return res
	}
	res.PublicIP = string(body)
	res.OK = true
	return res
}
