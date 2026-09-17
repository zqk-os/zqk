package specbuilder

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

type mockTransport struct {
	mu           sync.Mutex
	reqCount     int
	failRequests int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reqCount++

	if m.failRequests > 0 {
		m.failRequests--
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("Error")),
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("OK")),
	}, nil
}

func TestBuilderResiliency(t *testing.T) {
	mockTx := &mockTransport{failRequests: 2} // fail 2 times

	resTx := newResiliencyTransport(mockTx, RetryPolicy{
		MaxRetries: 3,
		Backoff:    time.Millisecond * 10,
	})

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	resp, err := resTx.RoundTrip(req)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %v", resp.StatusCode)
	}

	if mockTx.reqCount != 3 {
		t.Fatalf("Expected 3 requests (2 failures, 1 success), got %v", mockTx.reqCount)
	}
}

func TestBuilderThreadSafety(t *testing.T) {
	builder := NewBuilder()
	client, err := builder.
		WithSpec(APISpec{
			Name: "TestAPI",
		}).
		Build()

	if err != nil {
		t.Fatalf("Failed to build client: %v", err)
	}

	defaultClient := client.(*defaultAPIClient)
	defaultClient.client.Transport = &mockTransport{}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			_, _ = client.Do(req)
		}()
	}
	wg.Wait()
}
