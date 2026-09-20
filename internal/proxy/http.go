package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ServeHTTPProxy handles a single accepted TCP connection as an HTTP/HTTPS proxy.
func (s *Server) ServeHTTPProxy(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}

	if strings.EqualFold(req.Method, http.MethodConnect) {
		s.handleConnect(conn, req)
		return
	}
	s.handleHTTP(conn, br, req)
}

func (s *Server) handleConnect(client net.Conn, req *http.Request) {
	addr := req.Host
	if !hasPort(addr) {
		addr = net.JoinHostPort(addr, "443")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, _, err := s.Dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		_, _ = client.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer remote.Close()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	_ = client.SetDeadline(time.Time{})
	relay(client, remote)
}

func (s *Server) handleHTTP(client net.Conn, br *bufio.Reader, req *http.Request) {
	if req.URL.Scheme == "" {
		req.URL.Scheme = "http"
	}
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}
	host := req.URL.Host
	if !hasPort(host) {
		host = net.JoinHostPort(host, "80")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, _, err := s.Dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		_, _ = fmt.Fprintf(client, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer remote.Close()

	req.RequestURI = ""
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")
	if err := req.Write(remote); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})

	// Drain any remaining buffered client body bytes, then bidirectional relay.
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remote, br)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, remote)
		done <- struct{}{}
	}()
	<-done
}

func hasPort(hostport string) bool {
	_, _, err := net.SplitHostPort(hostport)
	return err == nil
}

type closeWriter interface {
	CloseWrite() error
}

func relay(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}
