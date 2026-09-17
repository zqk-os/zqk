package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestFalGenerator_Config(t *testing.T) {
	_, err := NewFalGenerator("", "https://queue.fal.run", "/fal-ai/minimax-video")
	if err != ErrInvalidConfig {
		t.Errorf("expected ErrInvalidConfig when apiKey is empty, got: %v", err)
	}

	gen, err := NewFalGenerator("test-fal-key", "https://queue.fal.run", "/fal-ai/minimax-video")
	if err != nil {
		t.Fatalf("expected successful creation, got: %v", err)
	}
	if gen.apiKey != "test-fal-key" {
		t.Errorf("expected test-fal-key, got: %s", gen.apiKey)
	}
}

func TestFalGenerator_GenerateVideo(t *testing.T) {
	_ = zqkenv.FalAPIKey().Set("test-key")
	defer zqkenv.FalAPIKey().Unset()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Key test-key" {
			t.Errorf("missing or invalid Authorization header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"request_id":"fal-12345"}`))
	}))
	defer ts.Close()

	gen, _ := NewFalGenerator("test-key", "https://queue.fal.run", "/fal-ai/minimax-video")
	gen.baseURL = ts.URL

	reqID, err := gen.GenerateVideo(context.Background(), VideoRequest{Prompt: "A futuristic city"})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if reqID != "fal-12345" {
		t.Errorf("expected fal-12345, got: %s", reqID)
	}
}

func TestFalGenerator_PollStatus_Completed(t *testing.T) {
	_ = zqkenv.FalAPIKey().Set("test-key")
	defer zqkenv.FalAPIKey().Unset()

	mux := http.NewServeMux()

	// Status endpoint mock
	mux.HandleFunc("/fal-ai/minimax-video/requests/fal-12345/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"COMPLETED"}`))
	})

	// Final result endpoint mock
	mux.HandleFunc("/fal-ai/minimax-video/requests/fal-12345", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"video":{"url":"https://fal.media/video.mp4"}}`))
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	gen, _ := NewFalGenerator("test-key", "https://queue.fal.run", "/fal-ai/minimax-video")
	gen.baseURL = ts.URL

	url, err := gen.PollStatus(context.Background(), "fal-12345")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if url != "https://fal.media/video.mp4" {
		t.Errorf("expected video URL, got: %s", url)
	}
}

func TestFalGenerator_PollStatus_InProgress(t *testing.T) {
	_ = zqkenv.FalAPIKey().Set("test-key")
	defer zqkenv.FalAPIKey().Unset()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"IN_PROGRESS"}`))
	}))
	defer ts.Close()

	gen, _ := NewFalGenerator("test-key", "https://queue.fal.run", "/fal-ai/minimax-video")
	gen.baseURL = ts.URL

	url, err := gen.PollStatus(context.Background(), "fal-12345")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if url != "" {
		t.Errorf("expected empty URL for IN_PROGRESS, got: %s", url)
	}
}
