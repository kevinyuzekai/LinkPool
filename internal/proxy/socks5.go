package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// ServeSOCKS5 handles one SOCKS5 connection (no auth).
func (s *Server) ServeSOCKS5(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	// greeting
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return
	}
	if hdr[0] != 0x05 {
		return
	}
	nMethods := int(hdr[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	// no-auth only
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// request
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[0] != 0x05 || req[1] != 0x01 { // CONNECT
		_ = socksFail(conn, 0x07) // command not supported
		return
	}
	var host string
	switch req[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 0x03: // domain
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return
		}
		dom := make([]byte, l[0])
		if _, err := io.ReadFull(conn, dom); err != nil {
			return
		}
		host = string(dom)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(conn, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		_ = socksFail(conn, 0x08)
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	remote, _, err := s.Dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		_ = socksFail(conn, 0x05) // connection refused
		return
	}
	defer remote.Close()

	// success reply: bind 0.0.0.0:0
	reply := []byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}
	if _, err := conn.Write(reply); err != nil {
		return
	}
	_ = conn.SetDeadline(time.Time{})
	relay(conn, remote)
}

func socksFail(conn net.Conn, code byte) error {
	_, err := conn.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}
