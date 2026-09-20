package proxy

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Status of the proxy lifecycle.
type Status string

const (
	StatusStopped Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning Status = "running"
	StatusStopping Status = "stopping"
	StatusError   Status = "error"
)

// Config for local listeners.
type Config struct {
	HTTPAddr  string // e.g. "127.0.0.1:18080"
	SOCKSAddr string // e.g. "127.0.0.1:11080"
}

// Server runs HTTP and SOCKS5 listeners.
type Server struct {
	Dialer *Dialer
	Config Config

	mu         sync.Mutex
	status     Status
	lastError  string
	httpLn     net.Listener
	socksLn    net.Listener
	wg         sync.WaitGroup
	httpConns  atomic.Int64
	socksConns atomic.Int64
}

func NewServer(d *Dialer, cfg Config) *Server {
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = "127.0.0.1:18080"
	}
	if cfg.SOCKSAddr == "" {
		cfg.SOCKSAddr = "127.0.0.1:11080"
	}
	return &Server{Dialer: d, Config: cfg, status: StatusStopped}
}

func (s *Server) Status() (Status, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.lastError
}

func (s *Server) setStatus(st Status, errMsg string) {
	s.mu.Lock()
	s.status = st
	s.lastError = errMsg
	s.mu.Unlock()
}

// Start begins listening. Idempotent if already running.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.status == StatusRunning || s.status == StatusStarting {
		s.mu.Unlock()
		return nil
	}
	s.status = StatusStarting
	s.lastError = ""
	s.mu.Unlock()

	httpLn, err := net.Listen("tcp", s.Config.HTTPAddr)
	if err != nil {
		s.setStatus(StatusError, err.Error())
		return fmt.Errorf("http listen: %w", err)
	}
	socksLn, err := net.Listen("tcp", s.Config.SOCKSAddr)
	if err != nil {
		_ = httpLn.Close()
		s.setStatus(StatusError, err.Error())
		return fmt.Errorf("socks listen: %w", err)
	}

	s.mu.Lock()
	s.httpLn = httpLn
	s.socksLn = socksLn
	s.status = StatusRunning
	s.mu.Unlock()

	s.wg.Add(2)
	go s.acceptLoop(httpLn, true)
	go s.acceptLoop(socksLn, false)
	return nil
}

func (s *Server) acceptLoop(ln net.Listener, httpMode bool) {
	defer s.wg.Done()
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		if httpMode {
			s.httpConns.Add(1)
			go func() {
				defer s.httpConns.Add(-1)
				s.ServeHTTPProxy(c)
			}()
		} else {
			s.socksConns.Add(1)
			go func() {
				defer s.socksConns.Add(-1)
				s.ServeSOCKS5(c)
			}()
		}
	}
}

// Stop closes listeners and waits for accept loops.
func (s *Server) Stop() error {
	s.mu.Lock()
	if s.status != StatusRunning && s.status != StatusStarting && s.status != StatusError {
		s.mu.Unlock()
		return nil
	}
	s.status = StatusStopping
	httpLn := s.httpLn
	socksLn := s.socksLn
	s.httpLn = nil
	s.socksLn = nil
	s.mu.Unlock()

	if httpLn != nil {
		_ = httpLn.Close()
	}
	if socksLn != nil {
		_ = socksLn.Close()
	}
	s.wg.Wait()
	s.setStatus(StatusStopped, "")
	return nil
}

func (s *Server) Addresses() (httpAddr, socksAddr string) {
	return s.Config.HTTPAddr, s.Config.SOCKSAddr
}
