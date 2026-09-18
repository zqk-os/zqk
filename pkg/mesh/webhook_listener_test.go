package mesh_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/mesh"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// mockStorageProvider implements storage.ObjectStorageProvider for testing
type mockStorageProvider struct {
	storage.ObjectStorageProvider
	Updates map[string]map[string]any
}

func (m *mockStorageProvider) Update(ctx context.Context, secCtx *storage.SecurityContext, id string, updates map[string]any) error {
	if m.Updates == nil {
		m.Updates = make(map[string]map[string]any)
	}
	m.Updates[id] = updates
	return nil
}

func TestWebhookListener_Integration(t *testing.T) {
	mockStore := &mockStorageProvider{}
	server := httptest.NewServer(mesh.WebhookHandler(mockStore))
	defer server.Close()

	t.Run("Valid Completion Callback", func(t *testing.T) {
		payload := mesh.WebhookPayload{
			JobID:  "media-job-999",
			Status: objects.ObjectStatusCompleted,
			URL:    "https://example.com/rendered-video.mp4",
		}

		body, _ := json.Marshal(payload)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(server.URL+"/webhooks/media", "application/json", bytes.NewReader(body))

		if err != nil {
			t.Fatalf("Failed to execute webhook callback: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 OK, got %v", resp.StatusCode)
		}

		// Verify store update
		update, ok := mockStore.Updates["media-job-999"]
		if !ok {
			t.Fatalf("Expected update to store for media-job-999")
		}
		if update[objects.FieldKeyStatus] != objects.ObjectStatusCompleted {
			t.Errorf("Expected status %v, got %v", objects.ObjectStatusCompleted, update[objects.FieldKeyStatus])
		}
		if update["artifact_url"] != "https://example.com/rendered-video.mp4" {
			t.Errorf("Expected artifact_url https://example.com/rendered-video.mp4, got %v", update["artifact_url"])
		}
	})

	t.Run("Invalid Payload", func(t *testing.T) {
		payload := mesh.WebhookPayload{
			JobID:  "media-job-111",
			Status: objects.ObjectStatusFailed, // Missing URL
		}
		body, _ := json.Marshal(payload)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(server.URL+"/webhooks/media", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to execute webhook callback: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("Expected status 422 Unprocessable Entity, got %v", resp.StatusCode)
		}
	})
}
