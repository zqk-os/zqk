package scheduler

import (
	"bufio"
	"crypto/sha1" //nolint:gosec
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testHandshakeNonce  = "dGhlIHNhbXBsZSBub25jZQ=="
	testExpectedPayload = `{"health": 99.5, "traffic": 1.5}`
	testReadTimeout     = 2 * time.Second
	testMaskBit         = 0x80
	testLenMask         = 0x7f
	testLenMarkerMedium = 126
)

func TestWebSocketTelemetry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(routeTelemetryWS, func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeToWebSocket(w, r)
		if err != nil {
			t.Errorf("failed to upgrade connection: %v", err)
			return
		}
		defer conn.Close()

		// Send a test message using our constant
		err = writeWebSocketTextFrame(conn, []byte(testExpectedPayload))
		if err != nil {
			t.Errorf("failed to write websocket text frame: %v", err)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect using TCP dialer
	u := server.URL
	host := strings.TrimPrefix(u, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("failed to dial TCP: %v", err)
	}
	defer conn.Close()

	// Send WebSocket handshake request
	fmt.Fprintf(conn, "GET /api/ws HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n", host, testHandshakeNonce)

	// Read handshake response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	if err != nil {
		t.Fatalf("failed to read handshake response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected status code 101, got %d", resp.StatusCode)
	}

	// Verify Sec-WebSocket-Accept header
	expectedAccept := func() string {
		h := sha1.New() //nolint:gosec
		h.Write([]byte(testHandshakeNonce + rfc6455WebSocketGuid))
		return base64.StdEncoding.EncodeToString(h.Sum(nil))
	}()

	accept := resp.Header.Get("Sec-WebSocket-Accept")
	if accept != expectedAccept {
		t.Errorf("expected Sec-WebSocket-Accept header %s, got %s", expectedAccept, accept)
	}

	// Set read deadline to avoid hanging if test fails
	_ = conn.SetReadDeadline(time.Now().Add(testReadTimeout))

	// Read WebSocket frame header
	b0, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("failed to read first byte of frame: %v", err)
	}
	if b0 != wsOpcodeText {
		t.Errorf("expected frame byte 0x%x (FIN + Text frame), got 0x%x", wsOpcodeText, b0)
	}

	// Byte 1: Mask and Length
	b1, err := reader.ReadByte()
	if err != nil {
		t.Fatalf("failed to read second byte of frame: %v", err)
	}
	isMasked := (b1 & testMaskBit) != 0
	if isMasked {
		t.Error("expected server-to-client frame to be unmasked, but it is masked")
	}

	length := int(b1 & testLenMask)
	if length == testLenMarkerMedium {
		var lBytes [2]byte
		_, err := reader.Read(lBytes[:])
		if err != nil {
			t.Fatalf("failed to read 2-byte length: %v", err)
		}
		length = int(lBytes[0])<<8 | int(lBytes[1])
	}

	// Read payload
	payload := make([]byte, length)
	_, err = reader.Read(payload)
	if err != nil {
		t.Fatalf("failed to read payload: %v", err)
	}

	if string(payload) != testExpectedPayload {
		t.Errorf("expected payload %q, got %q", testExpectedPayload, string(payload))
	}
}
