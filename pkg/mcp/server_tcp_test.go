package mcp

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestIsLoopbackAddr(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8443", true},
		{"127.0.0.1", true},
		{"127.0.1.1:9000", true},
		{"localhost:8443", true},
		{"localhost", true},
		{"[::1]:8443", true},
		{"::1", true},
		{"0.0.0.0:8443", false},
		{":8443", false},
		{"[::]:8443", false},
		{"192.168.1.100:8443", false},
		{"10.0.0.1:8443", false},
		{"example.com:8443", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsLoopbackAddr(tt.addr)
		if got != tt.want {
			t.Errorf("IsLoopbackAddr(%q) = %v; want %v", tt.addr, got, tt.want)
		}
	}
}

const (
	tcpDialRetryAttempts = 50
	tcpDialRetryBackoff  = 10 * time.Millisecond
)

func dialLoopbackTCP(t *testing.T, addr string) net.Conn {
	t.Helper()
	var lastErr error
	for i := 0; i < tcpDialRetryAttempts; i++ {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			return c
		}
		lastErr = err
		time.Sleep(tcpDialRetryBackoff)
	}
	t.Fatalf("failed to dial loopback MCP TCP server on %s: %v", addr, lastErr)
	return nil
}

// TRACK: BLI-CEF-R15-MCP-TCP-AUTH-001 / REQ-CEF-R2-SEC-MCP-TCP-AUTH
func TestServeTCP_RefusesNonLoopback(t *testing.T) {
	s := NewServer()
	nonLoopbackAddrs := []string{
		"0.0.0.0:8443",
		":8443",
		"192.168.1.50:8443",
		"10.255.0.1:9090",
		"[::]:8443",
	}

	for _, addr := range nonLoopbackAddrs {
		err := s.ServeTCP(addr)
		if err == nil {
			t.Errorf("ServeTCP(%q) expected error refusing unauthenticated plain TCP on non-loopback; got nil", addr)
		}
	}
}

// TRACK: CRIT-CEF-R2-SEC-MCP-TCP-AUTH-A
func TestServeTLS_RefusesNonLoopback(t *testing.T) {
	s := NewServer()
	nonLoopbackAddrs := []string{
		"0.0.0.0:8443",
		":8443",
		"192.168.1.50:8443",
		"10.255.0.1:9090",
		"[::]:8443",
	}

	for _, addr := range nonLoopbackAddrs {
		err := s.ServeTLS(addr, "cert.pem", "key.pem")
		if err == nil {
			t.Errorf("ServeTLS(%q) expected error refusing unilateral TLS on non-loopback; got nil", addr)
		}
	}
}

// TRACK: BLI-CEF-R15-MCP-TCP-AUTH-001 / CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 / REQ-CEF-R2-SEC-MCP-TCP-AUTH
func TestServeTCP_LoopbackRefusesUnauthenticatedToolsList(t *testing.T) {
	s := NewServer()

	// Register a tool so tool list is not empty
	s.RegisterTool("test_secret_tool", "Sensitive kernel tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return "secret result", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind loopback test listener: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	goroutinelabels.NewGoroutine("test_mcp_tcp_loopback", "run test MCP TCP server").
		StartSimple(func() {
			_ = s.ServeTCP(addr)
		})

	conn := dialLoopbackTCP(t, addr)
	defer conn.Close()

	// Probe: Send tools/list WITHOUT initialize / credentials
	probeReq := fmt.Sprintf(`{"jsonrpc":"2.0","%s":1,"method":"tools/list"}`+"\n", objects.FieldKeyID)
	if _, err := conn.Write([]byte(probeReq)); err != nil {
		t.Fatalf("failed to write raw probe request: %v", err)
	}

	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read probe response line: %v", err)
	}

	var jsonResp map[string]any
	if err := json.Unmarshal(respLine, &jsonResp); err != nil {
		t.Fatalf("failed to parse JSON response: %v, raw=%s", err, string(respLine))
	}

	// Must contain error with Unauthenticated (-32000)
	errObj, hasErr := jsonResp["error"].(map[string]any)
	if !hasErr {
		t.Fatalf("CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 violation: unauthenticated tools/list succeeded! Response: %s", string(respLine))
	}

	const fieldKeyErrCode = "code"
	code, _ := errObj[fieldKeyErrCode].(float64)
	if int(code) != Unauthenticated {
		t.Fatalf("expected Unauthenticated error code (-32000), got: %v", code)
	}

	s.RequestShutdown("test complete")
}

// TRACK: BLI-CEF-R15-MCP-TCP-AUTH-001 / CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 / REQ-CEF-R2-SEC-MCP-TCP-AUTH
func TestServeTCP_LoopbackRefusesInitializeWithoutCredentialsThenToolsList(t *testing.T) {
	s := NewServer()

	s.RegisterTool("test_secret_tool", "Sensitive kernel tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return "secret result", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind loopback test listener: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	goroutinelabels.NewGoroutine("test_mcp_tcp_loopback_init_bypass", "run test MCP TCP server").
		StartSimple(func() {
			_ = s.ServeTCP(addr)
		})

	conn := dialLoopbackTCP(t, addr)
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Step 1: Send initialize WITHOUT credentials
	initPayload, _ := json.Marshal(map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyID:     1,
		objects.FieldKeyMethod: "initialize",
		"params": InitializeParams{
			ProtocolVersion: "2024-11-05",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "unauthenticated-agent",
				Version: "1.0",
			},
		},
	})
	if _, err := conn.Write(append(initPayload, '\n')); err != nil {
		t.Fatalf("failed to write initialize request: %v", err)
	}

	initRespLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read initialize response line: %v", err)
	}

	var initResp map[string]any
	if err := json.Unmarshal(initRespLine, &initResp); err != nil {
		t.Fatalf("failed to parse initialize response: %v, raw=%s", err, string(initRespLine))
	}

	// Initialize must fail with error
	const fieldKeyErrCode = "code"
	initErrObj, initHasErr := initResp["error"].(map[string]any)
	if !initHasErr {
		t.Fatalf("CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 violation: initialize without credentials succeeded! Response: %s", string(initRespLine))
	}
	initCode, _ := initErrObj[fieldKeyErrCode].(float64)
	if int(initCode) != Unauthenticated {
		t.Fatalf("expected Unauthenticated error code (-32000) on uncredentialed initialize, got: %v", initCode)
	}

	// Step 2: Probe tools/list after failed initialize
	probeReq := fmt.Sprintf(`{"jsonrpc":"2.0","%s":2,"method":"tools/list"}`+"\n", objects.FieldKeyID)
	if _, err := conn.Write([]byte(probeReq)); err != nil {
		t.Fatalf("failed to write tools/list probe request: %v", err)
	}

	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read tools/list probe response line: %v", err)
	}

	var jsonResp map[string]any
	if err := json.Unmarshal(respLine, &jsonResp); err != nil {
		t.Fatalf("failed to parse JSON response: %v, raw=%s", err, string(respLine))
	}

	// Must contain error with Unauthenticated (-32000)
	errObj, hasErr := jsonResp["error"].(map[string]any)
	if !hasErr {
		t.Fatalf("CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 violation: tools/list succeeded after uncredentialed initialize! Response: %s", string(respLine))
	}

	code, _ := errObj[fieldKeyErrCode].(float64)
	if int(code) != Unauthenticated {
		t.Fatalf("expected Unauthenticated error code (-32000), got: %v", code)
	}

	s.RequestShutdown("test complete")
}

// TRACK: BLI-CEF-R2-REL-MCP-RECONNECT / CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001
func TestServeTCP_LoopbackAllowsNamedIDEAdapterWithoutCredentials(t *testing.T) {
	s := NewServer()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind loopback test listener: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	goroutinelabels.NewGoroutine("test_mcp_tcp_loopback_ide_adapter", "run test MCP TCP server").
		StartSimple(func() {
			_ = s.ServeTCP(addr)
		})

	conn := dialLoopbackTCP(t, addr)
	defer conn.Close()

	reader := bufio.NewReader(conn)
	initPayload, _ := json.Marshal(map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyID:     1,
		objects.FieldKeyMethod: "initialize",
		"params": InitializeParams{
			ProtocolVersion: "2024-11-05",
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    IDEProxySubscriberClientID,
				Version: "1.0",
			},
		},
	})
	if _, err := conn.Write(append(initPayload, '\n')); err != nil {
		t.Fatalf("failed to write initialize request: %v", err)
	}

	initRespLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read initialize response line: %v", err)
	}

	var initResp map[string]any
	if err := json.Unmarshal(initRespLine, &initResp); err != nil {
		t.Fatalf("failed to parse initialize response: %v, raw=%s", err, string(initRespLine))
	}
	if _, hasErr := initResp["error"]; hasErr {
		t.Fatalf("named ide-adapter initialize should succeed on loopback TCP: %s", string(initRespLine))
	}
	if _, hasResult := initResp["result"]; !hasResult {
		t.Fatalf("expected initialize result for %s: %s", IDEProxySubscriberClientID, string(initRespLine))
	}

	s.RequestShutdown("test complete")
}

// TRACK: BLI-CEF-R15-MCP-TCP-AUTH-001 / CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 / REQ-CEF-R2-SEC-MCP-TCP-AUTH
func TestServeTCP_LoopbackRefusesSpoofedClientIDWithoutCredentials(t *testing.T) {
	s := NewServer()

	s.RegisterTool("test_secret_tool", "Sensitive kernel tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return "secret result", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind loopback test listener: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	goroutinelabels.NewGoroutine("test_mcp_tcp_loopback_spoof_bypass", "run test MCP TCP server").
		StartSimple(func() {
			_ = s.ServeTCP(addr)
		})

	conn := dialLoopbackTCP(t, addr)
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Step 1: Send initialize with SPOOFED ZQK- client_id and NO credentials
	initPayload, _ := json.Marshal(map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyID:     1,
		objects.FieldKeyMethod: "initialize",
		"params": InitializeParams{
			ProtocolVersion: "2024-11-05",
			Capabilities: map[string]any{
				objects.FieldKeyClientID: "ZQK-spoofed-session-id",
			},
			ClientInfo: struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			}{
				Name:    "spoofed-agent",
				Version: "1.0",
			},
		},
	})
	if _, err := conn.Write(append(initPayload, '\n')); err != nil {
		t.Fatalf("failed to write spoofed initialize request: %v", err)
	}

	initRespLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read initialize response line: %v", err)
	}

	var initResp map[string]any
	if err := json.Unmarshal(initRespLine, &initResp); err != nil {
		t.Fatalf("failed to parse initialize response: %v, raw=%s", err, string(initRespLine))
	}

	// Initialize must fail with Unauthenticated (-32000)
	const fieldKeyErrCode = "code"
	initErrObj, initHasErr := initResp["error"].(map[string]any)
	if !initHasErr {
		t.Fatalf("CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 violation: initialize with spoofed client_id succeeded! Response: %s", string(initRespLine))
	}
	initCode, _ := initErrObj[fieldKeyErrCode].(float64)
	if int(initCode) != Unauthenticated {
		t.Fatalf("expected Unauthenticated error code (-32000) on spoofed client_id initialize, got: %v", initCode)
	}

	// Step 2: Probe tools/list after spoofed initialize
	probeReq := fmt.Sprintf(`{"jsonrpc":"2.0","%s":2,"method":"tools/list"}`+"\n", objects.FieldKeyID)
	if _, err := conn.Write([]byte(probeReq)); err != nil {
		t.Fatalf("failed to write tools/list probe request: %v", err)
	}

	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read tools/list probe response line: %v", err)
	}

	var jsonResp map[string]any
	if err := json.Unmarshal(respLine, &jsonResp); err != nil {
		t.Fatalf("failed to parse JSON response: %v, raw=%s", err, string(respLine))
	}

	errObj, hasErr := jsonResp["error"].(map[string]any)
	if !hasErr {
		t.Fatalf("CRIT-CEF-R15-MCP-LOOPBACK-AUTH-001 violation: tools/list succeeded after spoofed client_id initialize! Response: %s", string(respLine))
	}

	code, _ := errObj[fieldKeyErrCode].(float64)
	if int(code) != Unauthenticated {
		t.Fatalf("expected Unauthenticated error code (-32000), got: %v", code)
	}

	s.RequestShutdown("test complete")
}

// TRACK: BLI-CEF-R14-SEC-MCP-SURFACE-001 / CRIT-CEF-R14-SEC-MCP-SURFACE-001 / REQ-CEF-R14-RCV-SEC-001
func TestServeTCP_SecretsNotLoggedAndToolAuthorizationEnforced(t *testing.T) {
	s := NewServer()

	// Register a tool with restricted role requirement
	s.RegisterTool("admin_danger_tool", "Privileged tool", nil, func(ctx context.Context, args map[string]any) (any, error) {
		return "admin execution result", nil
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind loopback test listener: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	goroutinelabels.NewGoroutine("test_mcp_tcp_secrets_audit", "run MCP TCP server for secrets audit").
		StartSimple(func() {
			_ = s.ServeTCP(addr)
		})

	conn := dialLoopbackTCP(t, addr)
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Step 1: Send unauthenticated tools/call probe to privileged tool
	toolReqPayload, _ := json.Marshal(map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyID:     1,
		objects.FieldKeyMethod: "tools/call",
		"params": map[string]any{
			objects.FieldKeyName: "admin_danger_tool",
			"arguments": map[string]any{
				"secret_key": "sensitive_test_secret_val",
			},
		},
	})
	if _, err := conn.Write(append(toolReqPayload, '\n')); err != nil {
		t.Fatalf("failed to write tools/call request: %v", err)
	}

	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("failed to read tools/call response line: %v", err)
	}

	var jsonResp map[string]any
	if err := json.Unmarshal(respLine, &jsonResp); err != nil {
		t.Fatalf("failed to parse JSON response: %v, raw=%s", err, string(respLine))
	}

	// Tool call MUST be refused fail-closed with Unauthenticated (-32000)
	const fieldKeyErrCode = "code"
	errObj, hasErr := jsonResp["error"].(map[string]any)
	if !hasErr {
		t.Fatalf("CRIT-CEF-R14-SEC-MCP-SURFACE-001 violation: unauthenticated tools/call succeeded! Response: %s", string(respLine))
	}
	code, _ := errObj[fieldKeyErrCode].(float64)
	if int(code) != Unauthenticated {
		t.Fatalf("expected Unauthenticated error code (-32000), got: %v", code)
	}

	s.RequestShutdown("test complete")
}

func generateTestPKI(t *testing.T, dir string) (caCertFile, serverCertFile, serverKeyFile, clientCertFile, clientKeyFile string) {
	t.Helper()

	// 1. Generate CA
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "ZQK Test CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA cert: %v", err)
	}

	caCertFile = filepath.Join(dir, "ca.crt")
	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if err := fileutil.WriteFile(caCertFile, caCertPEM, 0644); err != nil {
		t.Fatalf("failed to write CA cert: %v", err)
	}

	// 2. Generate Server Cert
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate server key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:    []string{"localhost"},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create server cert: %v", err)
	}

	serverCertFile = filepath.Join(dir, "server.crt")
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	if err := fileutil.WriteFile(serverCertFile, serverCertPEM, 0644); err != nil {
		t.Fatalf("failed to write server cert: %v", err)
	}

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("failed to marshal server key: %v", err)
	}
	serverKeyFile = filepath.Join(dir, "server.key")
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyDER})
	if err := fileutil.WriteFile(serverKeyFile, serverKeyPEM, 0600); err != nil {
		t.Fatalf("failed to write server key: %v", err)
	}

	// 3. Generate Client Cert
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate client key: %v", err)
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			CommonName: "zqk-client",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create client cert: %v", err)
	}

	clientCertFile = filepath.Join(dir, "client.crt")
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	if err := fileutil.WriteFile(clientCertFile, clientCertPEM, 0644); err != nil {
		t.Fatalf("failed to write client cert: %v", err)
	}

	clientKeyDER, err := x509.MarshalECPrivateKey(clientKey)
	if err != nil {
		t.Fatalf("failed to marshal client key: %v", err)
	}
	clientKeyFile = filepath.Join(dir, "client.key")
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyDER})
	if err := fileutil.WriteFile(clientKeyFile, clientKeyPEM, 0600); err != nil {
		t.Fatalf("failed to write client key: %v", err)
	}

	return caCertFile, serverCertFile, serverKeyFile, clientCertFile, clientKeyFile
}

func TestServeTLS_MissingCertificates(t *testing.T) {
	s := NewServer()
	err := s.ServeTLS("127.0.0.1:0", "", "")
	if err == nil {
		t.Fatal("expected error with missing TLS cert and key")
	}
}

func TestServeMTLS_MissingCertificates(t *testing.T) {
	s := NewServer()
	err := s.ServeMTLS("127.0.0.1:0", "", "", "")
	if err == nil {
		t.Fatal("expected error with missing mTLS cert, key, and CA")
	}
}

func TestServeMTLS_RequiresClientCert(t *testing.T) {
	tmpDir := t.TempDir()
	caCertFile, serverCertFile, serverKeyFile, clientCertFile, clientKeyFile := generateTestPKI(t, tmpDir)

	s := NewServer()

	// Pick a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	// Run ServeMTLS in background
	goroutinelabels.NewGoroutine("test_mcp_mtls", "run test MCP mTLS server").
		StartSimple(func() {
			_ = s.ServeMTLS(addr, serverCertFile, serverKeyFile, caCertFile)
		})

	plain := dialLoopbackTCP(t, addr)
	_ = plain.Close()

	// 1. Connection WITHOUT client cert should fail handshake
	caCertPEM, err := fileutil.ReadFile(caCertFile)
	if err != nil {
		t.Fatalf("failed to read CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caCertPEM)

	tlsConfigNoClientCert := &tls.Config{
		RootCAs:    caPool,
		ServerName: "localhost",
	}

	conn, err := tls.Dial("tcp", addr, tlsConfigNoClientCert)
	if err == nil {
		defer conn.Close()
		_, writeErr := conn.Write([]byte(`{"jsonrpc":"2.0","method":"ping","id":1}` + "\n"))
		buf := make([]byte, 100)
		_, readErr := conn.Read(buf)
		if writeErr == nil && readErr == nil {
			t.Fatal("expected mTLS connection without client certificate to fail on data read/write")
		}
	}

	// 2. Connection WITH valid client cert should succeed handshake
	clientCert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		t.Fatalf("failed to load client cert key pair: %v", err)
	}

	tlsConfigWithClientCert := &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{clientCert},
		ServerName:   "localhost",
	}

	clientConn, err := tls.Dial("tcp", addr, tlsConfigWithClientCert)
	if err != nil {
		t.Fatalf("expected successful TLS dial with valid client cert; got error: %v", err)
	}
	defer clientConn.Close()
	if err := clientConn.Handshake(); err != nil {
		t.Fatalf("expected successful TLS handshake with valid client cert; got error: %v", err)
	}

	s.RequestShutdown("test complete")
}

// tdd refresh
