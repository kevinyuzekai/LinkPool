package app

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/kevinyuzekai/LinkPool/internal/adapter"
	"github.com/kevinyuzekai/LinkPool/internal/diag"
	"github.com/kevinyuzekai/LinkPool/internal/proxy"
	"github.com/kevinyuzekai/LinkPool/internal/rules"
	"github.com/kevinyuzekai/LinkPool/internal/scheduler"
	"github.com/kevinyuzekai/LinkPool/internal/stats"
	"github.com/kevinyuzekai/LinkPool/internal/sysproxy"
)

// App is the central controller for LinkPool.
type App struct {
	mu sync.Mutex

	Discoverer adapter.Discoverer
	Adapters   []adapter.Adapter
	Sched      *scheduler.Scheduler
	Rules      *rules.Matcher
	Stats      *stats.Registry
	Dialer     *proxy.Dialer
	Proxy      *proxy.Server
	SysProxy   *sysproxy.Manager

	HTTPAddr  string
	SOCKSAddr string
	WantSys   bool
}

func New() *App {
	sched := scheduler.New()
	r := rules.New()
	r.SetRules(rules.DefaultBypass())
	st := stats.New()
	d := &proxy.Dialer{Sched: sched, Rules: r, Stats: st}
	cfg := proxy.Config{HTTPAddr: "127.0.0.1:18080", SOCKSAddr: "127.0.0.1:11080"}
	return &App{
		Discoverer: adapter.DefaultDiscoverer(),
		Sched:      sched,
		Rules:      r,
		Stats:      st,
		Dialer:     d,
		Proxy:      proxy.NewServer(d, cfg),
		SysProxy:   sysproxy.New(),
		HTTPAddr:   cfg.HTTPAddr,
		SOCKSAddr:  cfg.SOCKSAddr,
	}
}

func (a *App) RefreshAdapters() ([]adapter.Adapter, error) {
	list, err := a.Discoverer.List()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Preserve selection / weights by ID
	prev := map[string]adapter.Adapter{}
	for _, old := range a.Adapters {
		prev[old.ID] = old
	}
	for i := range list {
		if p, ok := prev[list[i].ID]; ok {
			list[i].Selected = p.Selected
			list[i].Weight = p.Weight
			if list[i].Weight < 1 {
				list[i].Weight = 1
			}
		} else {
			list[i].Weight = 1
		}
	}
	a.Adapters = list
	a.rebuildSchedulerLocked()
	out := make([]adapter.Adapter, len(list))
	copy(out, list)
	return out, nil
}

type AdapterUpdate struct {
	ID       string `json:"id"`
	Selected bool   `json:"selected"`
	Weight   int    `json:"weight"`
}

func (a *App) UpdateAdapters(updates []AdapterUpdate) []adapter.Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	byID := map[string]AdapterUpdate{}
	for _, u := range updates {
		byID[u.ID] = u
	}
	for i := range a.Adapters {
		if u, ok := byID[a.Adapters[i].ID]; ok {
			a.Adapters[i].Selected = u.Selected
			w := u.Weight
			if w < 1 {
				w = 1
			}
			a.Adapters[i].Weight = w
		}
	}
	a.rebuildSchedulerLocked()
	out := make([]adapter.Adapter, len(a.Adapters))
	copy(out, a.Adapters)
	return out
}

func (a *App) rebuildSchedulerLocked() {
	var entries []scheduler.Entry
	for _, ad := range a.Adapters {
		if !ad.Selected || !ad.Up {
			continue
		}
		entries = append(entries, scheduler.Entry{Adapter: ad, Weight: ad.Weight})
	}
	a.Sched.SetEntries(entries)
}

func (a *App) StartProxy(setSysProxy bool) error {
	a.mu.Lock()
	if a.Sched.Len() == 0 {
		a.mu.Unlock()
		return fmt.Errorf("请至少选择一个可用网卡并设置权重")
	}
	httpAddr, socksAddr := a.HTTPAddr, a.SOCKSAddr
	a.Proxy.Config.HTTPAddr = httpAddr
	a.Proxy.Config.SOCKSAddr = socksAddr
	a.WantSys = setSysProxy
	a.mu.Unlock()

	if err := a.Proxy.Start(); err != nil {
		return err
	}
	if setSysProxy {
		hh, hp := splitHostPort(httpAddr, "127.0.0.1", 18080)
		sh, sp := splitHostPort(socksAddr, "127.0.0.1", 11080)
		if err := a.SysProxy.Enable(hh, hp, sh, sp); err != nil {
			// Proxy still runs; report sysproxy failure
			return fmt.Errorf("代理已启动，但系统代理设置失败: %w", err)
		}
	}
	return nil
}

func (a *App) StopProxy() error {
	_ = a.SysProxy.Disable()
	return a.Proxy.Stop()
}

func (a *App) Status() map[string]any {
	st, errMsg := a.Proxy.Status()
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{
		"status":       st,
		"error":        errMsg,
		"httpAddr":     a.HTTPAddr,
		"socksAddr":    a.SOCKSAddr,
		"sysProxy":     a.SysProxy.Enabled(),
		"selected":     a.Sched.Len(),
		"schedulerPicks": a.Sched.PickCount(),
		"stats":        a.Stats.All(),
	}
}

func (a *App) SetBypassText(text string) {
	a.Rules.SetFromText(text)
}

func (a *App) BypassText() string {
	list := a.Rules.List()
	out := ""
	for i, l := range list {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

func (a *App) SetListenAddrs(httpAddr, socksAddr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, _ := a.Proxy.Status()
	if st == proxy.StatusRunning || st == proxy.StatusStarting {
		return fmt.Errorf("请先停止代理再修改监听地址")
	}
	if httpAddr != "" {
		a.HTTPAddr = httpAddr
	}
	if socksAddr != "" {
		a.SOCKSAddr = socksAddr
	}
	return nil
}

func (a *App) Diagnose(ctx context.Context) []diag.Result {
	a.mu.Lock()
	ads := make([]adapter.Adapter, 0)
	for _, ad := range a.Adapters {
		if ad.Selected {
			ads = append(ads, ad)
		}
	}
	a.mu.Unlock()
	var results []diag.Result
	for _, ad := range ads {
		results = append(results, diag.CheckFromAdapter(ctx, ad, ""))
	}
	return results
}

func splitHostPort(addr, defHost string, defPort int) (string, int) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		return defHost, defPort
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return h, defPort
	}
	return h, port
}
