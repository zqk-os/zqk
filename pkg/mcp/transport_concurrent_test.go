package mcp

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
)

// Regression: proxy used one DefaultTransport for stdin + TCP; concurrent
// json.Decoder.Decode panics inside encoding/json (IDE MCP stack at transport.go Decode).
func TestDefaultTransport_concurrentReadersDoNotPanic(t *testing.T) {
	t.Parallel()
	shared := NewDefaultTransport()

	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				r := bufio.NewReader(bytes.NewReader(payload))
				if _, _, err := shared.ReadMessage(r); err != nil {
					t.Errorf("ReadMessage: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
