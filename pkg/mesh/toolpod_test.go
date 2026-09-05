package mesh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDispatchFFmpegJob_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process" {
			t.Errorf("expected path /process, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		var req FFmpegJobRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.JobID != "job-1" {
			t.Errorf("expected JobID 'job-1', got '%s'", req.JobID)
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	coordinator := NewHTTPToolPodCoordinator()
	err := coordinator.DispatchFFmpegJob(context.Background(), server.URL, FFmpegJobRequest{
		JobID:       "job-1",
		InputS3URL:  "s3://input",
		OutputS3URL: "s3://output",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestDispatchFFmpegJob_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	coordinator := NewHTTPToolPodCoordinator()
	err := coordinator.DispatchFFmpegJob(context.Background(), server.URL, FFmpegJobRequest{})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
