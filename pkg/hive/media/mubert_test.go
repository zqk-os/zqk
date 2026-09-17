package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func setupMubertMock() *httptest.Server {
	mux := http.NewServeMux()

	// Mock Handshake
	mux.HandleFunc("/service/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("company-id") == "" || r.Header.Get("license-token") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"cust-123","access":{"token":"token-123"}}}`))
	})

	// Mock Generate
	mux.HandleFunc("/public/tracks", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"task-123"}}`))
	})

	// Mock Poll
	mux.HandleFunc("/public/tracks/task-123", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"generations":[{"status":"done","url":"https://mubert.local/song.mp3"}]}}`))
	})

	return httptest.NewServer(mux)
}

func TestMubertGenerator_Config(t *testing.T) {
	_ = zqkenv.MubertCompanyID().Unset()
	_ = zqkenv.MubertLicToken().Unset()

	_, err := NewMubertGenerator(context.Background())
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig when env vars are missing, got: %v", err)
	}

	_ = zqkenv.MubertCompanyID().Set("test-company")
	_ = zqkenv.MubertLicToken().Set("test-token")
	defer zqkenv.MubertCompanyID().Unset()
	defer zqkenv.MubertLicToken().Unset()

	ts := setupMubertMock()
	defer ts.Close()

	// Need to manually build and auth to test with mock URL
	gen := &MubertGenerator{
		companyID:  "test-company",
		licenseTok: "test-token",
		baseURL:    ts.URL,
		client:     &http.Client{},
	}
	err = gen.authenticate(context.Background())

	if err != nil {
		t.Fatalf("expected successful creation, got: %v", err)
	}
	if gen.customerID != "cust-123" {
		t.Errorf("expected cust-123, got: %s", gen.customerID)
	}
}

func TestMubertGenerator_GenerateAudio(t *testing.T) {
	ts := setupMubertMock()
	defer ts.Close()

	gen := &MubertGenerator{
		customerID:  "cust-123",
		accessToken: "token-123",
		baseURL:     ts.URL,
		client:      &http.Client{},
	}

	taskID, err := gen.GenerateAudio(context.Background(), AudioRequest{Prompt: "lofi hip hop"})
	if err != nil {
		t.Fatalf("expected successful generation, got: %v", err)
	}
	if taskID != "task-123" {
		t.Errorf("expected task_id task-123, got: %s", taskID)
	}
}

func TestMubertGenerator_PollStatus(t *testing.T) {
	ts := setupMubertMock()
	defer ts.Close()

	gen := &MubertGenerator{
		customerID:  "cust-123",
		accessToken: "token-123",
		baseURL:     ts.URL,
		client:      &http.Client{},
	}

	url, err := gen.PollStatus(context.Background(), "task-123")
	if err != nil {
		t.Fatalf("expected successful poll, got: %v", err)
	}
	if url != "https://mubert.local/song.mp3" {
		t.Errorf("expected download link, got: %s", url)
	}
}
