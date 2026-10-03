package mcp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// IsLoopbackAddr checks if the given TCP address is bound only to a loopback interface (e.g. 127.0.0.1, ::1, localhost).
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || host == "0.0.0.0" || host == "::" {
		return false // wildcard/all interfaces
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ServeTCP starts the MCP server over a TCP socket
// It accepts multiple concurrent connections, each getting its own ServeLoop.
// Per REQ-CEF-R2-SEC-MCP-TCP-AUTH / CRIT-CEF-R2-SEC-MCP-TCP-AUTH-A, plain unauthenticated TCP
// binding is restricted to loopback interfaces. Network exposure requires mTLS (ServeMTLS) or TLS.
func (s *Server) ServeTCP(addr string) error {
	if !IsLoopbackAddr(addr) {
		return errfmt.Errorf("refusing to bind unauthenticated plain TCP server to non-loopback address %q; mTLS (ServeMTLS) or TLS with authentication is required for network exposure (CRIT-CEF-R2-SEC-MCP-TCP-AUTH-A)", addr)
	}

	s.multiClient.Store(true)
	s.ensureServerInitialized()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errfmt.Newf("failed to listen on %s", addr).Wrap(err)
	}
	return s.serveListener(listener, "TCP", addr)
}

// serveListener handles accepting connections and coordinating shutdown for a network listener.
func (s *Server) serveListener(listener net.Listener, scheme, addr string) error {
	defer listener.Close()

	if s.getTraceWriter() != nil {
		s.traceLogf("[MCP_INFO] Listening for %s connections on %s", scheme, addr)
	}

	goroutinelabels.NewGoroutine("mcp_"+scheme+"_shutdown", "wait for server shutdown to close listener").
		StartSimple(func() {
			<-s.shutdownCtx.Done()
			if closeErr := listener.Close(); closeErr != nil && s.getTraceWriter() != nil {
				s.traceLogf("[MCP_DEBUG] %s listener close on shutdown: %v", scheme, closeErr)
			}
		})

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.shutdownFlag.Load() == 1 {
				return nil // Graceful shutdown
			}
			if s.getTraceWriter() != nil {
				s.traceLogf("[MCP_ERROR] %s accept error: %v", scheme, err)
			}
			continue
		}

		goroutinelabels.NewGoroutine("mcp_"+scheme+"_handler", "handle incoming MCP connection").
			StartSimple(func() {
				defer func() {
					if r := recover(); r != nil {
						if s.getTraceWriter() != nil {
							s.traceLogf("[MCP_ERROR] %s connection handler panic: %v", scheme, r)
						}
					}
				}()
				s.handleTCPConnection(conn)
			})
	}
}

func (s *Server) handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	s.ensureServerInitialized()

	// Build connection-scoped lifecycle specifically for this connection
	lifecycle := NewServerLifecycleBuilder(s).
		WithReaderAndWriter(conn, conn).
		SetupTransport().
		SetupHandlers().
		Build()

	// Run main serve loop for this specific connection
	err := s.RunServeLoop(lifecycle)
	if err != nil && !isBrokenPipeError(err) {
		if s.getTraceWriter() != nil {
			s.traceLogf("[MCP_ERROR] TCP connection serve error: %v", err)
		}
	}
}

func (s *Server) prepareTLSCert(certFile, keyFile string) (tls.Certificate, error) {
	s.multiClient.Store(true)
	s.ensureServerInitialized()
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, errfmt.Newf("failed to load TLS key pair").Wrap(err)
	}
	return cert, nil
}

// ServeTLS starts the MCP server over a TLS-encrypted TCP socket
func (s *Server) ServeTLS(addr, certFile, keyFile string) error {
	if !IsLoopbackAddr(addr) {
		return errfmt.Errorf("refusing to bind TLS server without client authentication to non-loopback address %q; mutual TLS (ServeMTLS) is required for network exposure (CRIT-CEF-R2-SEC-MCP-TCP-AUTH-A)", addr)
	}
	if certFile == "" || keyFile == "" {
		return errors.New("both certFile and keyFile are required for TLS")
	}
	cert, err := s.prepareTLSCert(certFile, keyFile)
	if err != nil {
		return err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return errfmt.Newf("failed to listen on %s with TLS", addr).Wrap(err)
	}
	return s.serveListener(listener, "TLS", addr)
}

// ServeMTLS starts the MCP server over a TLS-encrypted TCP socket requiring mutual authentication (mTLS)
func (s *Server) ServeMTLS(addr, certFile, keyFile, caCertFile string) error {
	if certFile == "" || keyFile == "" || caCertFile == "" {
		return errors.New("certFile, keyFile, and caCertFile are all required for mTLS")
	}
	cert, err := s.prepareTLSCert(certFile, keyFile)
	if err != nil {
		return err
	}

	caCert, err := fileutil.ReadFile(caCertFile)
	if err != nil {
		return errfmt.Newf("failed to read CA certificate").Wrap(err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return errors.New("failed to parse CA certificate")
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	listener, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return errfmt.Newf("failed to listen on %s with mTLS", addr).Wrap(err)
	}
	return s.serveListener(listener, "mTLS", addr)
}
