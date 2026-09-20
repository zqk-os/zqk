package mesh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// FFmpegJobRequest matches the struct in cmd/zqk-ffmpeg-worker.
type FFmpegJobRequest struct {
	JobID        string `json:"job_id"`
	InputS3URL   string `json:"input_s3_url"`
	AudioS3URL   string `json:"audio_s3_url,omitempty"`
	OutputS3URL  string `json:"output_s3_url"`
	WebhookURL   string `json:"webhook_url"`
	VideoFilters string `json:"video_filters,omitempty"`
}

type ToolPodCoordinator interface {
	DispatchFFmpegJob(ctx context.Context, endpoint string, req FFmpegJobRequest) error
}

type HTTPToolPodCoordinator struct {
	Client *http.Client
}

func NewHTTPToolPodCoordinator() *HTTPToolPodCoordinator {
	return &HTTPToolPodCoordinator{
		Client: &http.Client{},
	}
}

func (c *HTTPToolPodCoordinator) DispatchFFmpegJob(ctx context.Context, endpoint string, req FFmpegJobRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal job request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/process", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to dispatch job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
