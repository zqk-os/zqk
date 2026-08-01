package mcp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// ServeTCP starts the MCP server over a TCP socket
// It accepts multiple concurrent connections, each getting its own ServeLoop
func (s *Server) ServeTCP(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errfmt.Newf("failed to listen on %s", addr).Wrap(err)
	}

	defer listener.Close()

	if s.getTraceWriter() != nil {
		s.traceLogf("[MCP_INFO] Listening for TCP connections on %s", addr)
	}

	// Wait for shutdown to close the listener
	goroutinelabels.NewGoroutine("mcp_tcp_shutdown", "wait for server shutdown to close TCP listener").
		StartSimple(func() {
			<-s.shutdownCtx.Done()
			listener.Close()
		})

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.shutdownFlag.Load() == 1 {
				return nil // Graceful shutdown
			}
			// Just log accept errors and continue
			if s.getTraceWriter() != nil {
				s.traceLogf("[MCP_ERROR] TCP accept error: %v", err)
			}
			continue
		}

		goroutinelabels.NewGoroutine("mcp_tcp_handler", "handle incoming TCP MCP connection").
			StartSimple(func() {
				s.handleTCPConnection(conn)
			})
	}
}

func (s *Server) handleTCPConnection(conn net.Conn) {
	defer conn.Close()

	// Build server lifecycle specifically for this connection
	lifecycle := NewServerLifecycleBuilder(s).
		WithReaderAndWriter(conn, conn).
		LoadConfig().
		ApplyAsyncConfig().
		ApplyEventEmitterConfig().
		ApplyRateLimitConfig().
		InitializeClientMetrics().
		InitializeTraceLogging().
		MarkServing().
		LoadMCPSpecs().
		SetupTransport().
		SetupHandlers().
		Build()

	defer lifecycle.Cleanup()

	// Create message processor
	processor := NewMessageProcessor(s, lifecycle.GetHandler(), lifecycle.GetTransport())

	// Create serve coordinator
	coordinator := NewServeCoordinator(s, processor, lifecycle)

	// Run main serve loop for this specific connection
	err := coordinator.ServeLoop()
	if err != nil && !isBrokenPipeError(err) {
		if s.getTraceWriter() != nil {
			s.traceLogf("[MCP_ERROR] TCP connection serve error: %v", err)
		}
	}
}

// ServeTLS starts the MCP server over a TLS-encrypted TCP socket
func (s *Server) ServeTLS(addr, certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return errfmt.Newf("failed to load TLS key pair").Wrap(err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	}

	listener, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return errfmt.Newf("failed to listen on %s with TLS", addr).Wrap(err)
	}

	defer listener.Close()

	if s.getTraceWriter() != nil {
		s.traceLogf("[MCP_INFO] Listening for TLS connections on %s", addr)
	}

	// Wait for shutdown to close the listener
	goroutinelabels.NewGoroutine("mcp_tls_shutdown", "wait for server shutdown to close TLS listener").
		StartSimple(func() {
			<-s.shutdownCtx.Done()
			listener.Close()
		})

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.shutdownFlag.Load() == 1 {
				return nil // Graceful shutdown
			}
			// Just log accept errors and continue
			if s.getTraceWriter() != nil {
				s.traceLogf("[MCP_ERROR] TLS accept error: %v", err)
			}
			continue
		}

		goroutinelabels.NewGoroutine("mcp_tls_handler", "handle incoming TLS MCP connection").
			StartSimple(func() {
				s.handleTCPConnection(conn)
			})
	}
}

// ServeMTLS starts the MCP server over a TLS-encrypted TCP socket requiring mutual authentication (mTLS)
func (s *Server) ServeMTLS(addr, certFile, keyFile, caCertFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return errfmt.Newf("failed to load TLS key pair").Wrap(err)
	}

	caCert, err := os.ReadFile(caCertFile)
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

	defer listener.Close()

	if s.getTraceWriter() != nil {
		s.traceLogf("[MCP_INFO] Listening for mTLS connections on %s", addr)
	}

	// Wait for shutdown to close the listener
	goroutinelabels.NewGoroutine("mcp_mtls_shutdown", "wait for server shutdown to close mTLS listener").
		StartSimple(func() {
			<-s.shutdownCtx.Done()
			listener.Close()
		})

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.shutdownFlag.Load() == 1 {
				return nil // Graceful shutdown
			}
			// Just log accept errors and continue
			if s.getTraceWriter() != nil {
				s.traceLogf("[MCP_ERROR] mTLS accept error: %v", err)
			}
			continue
		}

		goroutinelabels.NewGoroutine("mcp_mtls_handler", "handle incoming mTLS MCP connection").
			StartSimple(func() {
				s.handleTCPConnection(conn)
			})
	}
}
