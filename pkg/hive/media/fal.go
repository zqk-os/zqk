package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/lanceman/zqk/pkg/circuitbreaker"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder"
	"github.com/lanceman/zqk/pkg/specbuilder/api_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
)

// FalGenerator implements the MediaGenerator interface for the VEED/Fal.ai API.
type FalGenerator struct {
	apiKey    string
	baseURL   string
	modelPath string
	client    specbuilder.APIClient
	cb        *circuitbreaker.DefaultCircuitBreaker
}

// NewFalGenerator creates a new instance.
func NewFalGenerator(apiKey, baseURL, modelPath string) (*FalGenerator, error) {
	if apiKey == "" {
		return nil, ErrInvalidConfig
	}

	if baseURL == "" {
		baseURL = "https://queue.fal.run"
	}
	if modelPath == "" {
		modelPath = "/fal-ai/minimax-video"
	}

	spec := specbuilder.APISpec{
		Name:    "FAL AI",
		BaseURL: "https://fal.run",
		Timeout: 30 * time.Second,
		RetryPolicy: specbuilder.RetryPolicy{
			MaxRetries: 3,
			Backoff:    2 * time.Second,
		},
	}
	apiClient, _ := specbuilder.NewBuilder().
		WithSpec(spec).
		WithTelemetry(true).
		WithResiliency(true).
		Build()

	return &FalGenerator{
		apiKey:    apiKey,
		baseURL:   baseURL,
		modelPath: modelPath,
		client:    apiClient,
		cb:        circuitbreaker.NewCircuitBreaker(),
	}, nil
}

// GenerateAudio is not natively supported by this specific Fal.ai video endpoint block.
func (v *FalGenerator) GenerateAudio(ctx context.Context, req AudioRequest) (string, error) {
	return "", fmt.Errorf("fal generator does not support audio-only generation in this context")
}

// GenerateVideo creates a new video asset using Fal.ai async queue.
func (v *FalGenerator) GenerateVideo(ctx context.Context, req VideoRequest) (string, error) {
	// Construct the payload. Fal.ai models vary, but standard Luma/Runway inputs accept prompt and image_url.
	payload := map[string]any{
		"prompt": req.Prompt,
	}

	if req.ImageURL != "" {
		payload["image_url"] = req.ImageURL
	}
	if req.AspectRatio != "" {
		payload["aspect_ratio"] = req.AspectRatio
	}
	if req.Duration != "" {
		payload["duration"] = req.Duration
	}

	// If there's audio to sync
	if req.AudioURL != "" {
		payload["audio_url"] = req.AudioURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	// Pointing to a standard Fal.ai text/image to video model (e.g. luma or minimax)
	// For VEED fabric specifically, you'd use their exact model path here.
	modelPath := v.modelPath
	url := v.baseURL + modelPath

	apiSpec := map[string]any{
		objects.FieldKeyKind:             bldr_v2.NewAPISpecBuilder().GetOntology(),
		bldr_v2.APISpecFieldTargetSystem: "fal",
		bldr_v2.APISpecFieldEndpoint:     url,
		bldr_v2.APISpecFieldMethod:       http.MethodPost,
		bldr_v2.APISpecFieldPayloadSchema: map[string]any{
			"prompt":       "string",
			"image_url":    "string",
			"aspect_ratio": "string",
			"duration":     "string",
			"audio_url":    "string",
		},
	}

	schema := apiSpec[bldr_v2.APISpecFieldPayloadSchema].(map[string]any)
	for k := range payload {
		if _, ok := schema[k]; !ok {
			return "", fmt.Errorf("payload contains unknown field %s", k)
		}
	}

	logger := logging.GetLoggerFromContext(ctx)
	logging.FluentEvent(logger).Info("fal_generate_video_start").URL(url).Log()

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "fal",
		bldr_v2.APISpecFieldEndpoint:     url,
		"action":                         "generate_video",
		objects.FieldKeyStatus:           "started",
	})

	builder := api_builders.NewBuilder().
		SetURL(url).
		SetMethod(http.MethodPost).
		AddHeader("Content-Type", "application/json").
		AddHeader("Authorization", "Key "+v.apiKey).
		SetBody(body).
		SetClient(v.client).
		SetLogger(logger)

	resp, err := v.executeWithResilience(ctx, builder)
	if err != nil {
		logging.FluentEvent(logger).Error("fal_generate_video_failure", err).Log()
		return "", fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			respBytes = []byte(fmt.Sprintf("failed to read body: %v", err))
		}
		err = fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBytes))
		logging.FluentEvent(logger).Error("fal_generate_video_failure", err).Log()
		return "", fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	var result struct {
		RequestID string `json:"request_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logging.FluentEvent(logger).Error("fal_generate_video_failure", err).Log()
		return "", fmt.Errorf("failed to decode veed/fal response: %v", err)
	}

	if result.RequestID == "" {
		err := fmt.Errorf("no request_id returned")
		logging.FluentEvent(logger).Error("fal_generate_video_failure", err).Log()
		return "", fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "fal",
		bldr_v2.APISpecFieldEndpoint:     url,
		"action":                         "generate_video",
		objects.FieldKeyStatus:           "success",
		"job_id":                         result.RequestID,
	})

	logging.FluentEvent(logger).Info("fal_generate_video_success").Log()
	return result.RequestID, nil
}

func (v *FalGenerator) PollStatus(ctx context.Context, jobID string) (string, error) {
	// Fal status endpoint: https://queue.fal.run/fal-ai/luma-dream-machine/requests/{request_id}/status
	modelPath := v.modelPath
	url := v.baseURL + modelPath + "/requests/" + jobID + "/status"

	logger := logging.GetLoggerFromContext(ctx)
	logging.FluentEvent(logger).Info("fal_poll_status_start").URL(url).Log()

	builder := api_builders.NewBuilder().
		SetURL(url).
		SetMethod(http.MethodGet).
		AddHeader("Authorization", "Key "+v.apiKey).
		SetClient(v.client).
		SetLogger(logger)

	resp, err := v.executeWithResilience(ctx, builder)
	if err != nil {
		logging.FluentEvent(logger).Error("fal_poll_status_failure", err).Log()
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		err := fmt.Errorf("failed to poll status, got HTTP %d", resp.StatusCode)
		logging.FluentEvent(logger).Error("fal_poll_status_failure", err).Log()
		return "", err
	}

	// To handle Fal's structure cleanly:
	// If it's completed, we hit a different endpoint or extract the URL.
	// For simplicity in the mock, we assume the status payload returns the result if done.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.FluentEvent(logger).Error("fal_poll_status_read_failure", err).Log()
		return "", fmt.Errorf("failed to read status body: %w", err)
	}

	// A naive check for completion
	var generic map[string]any
	if err := json.Unmarshal(bodyBytes, &generic); err != nil {
		logging.FluentEvent(logger).Error("fal_poll_status_unmarshal_failure", err).JobID(jobID).Log()
		// Ignore the unmarshal error to keep polling
		return "", nil
	}

	if status, ok := generic[objects.FieldKeyStatus].(string); ok {
		if status == "COMPLETED" {
			logging.FluentEvent(logger).Info("fal_poll_status_completed").Log()
			// Actually need to fetch the final result from the result endpoint:
			// GET /requests/{request_id}
			return v.fetchFinalResult(ctx, modelPath, jobID)
		}
		if status == "FAILED" {
			err := fmt.Errorf("video generation failed upstream")
			logging.FluentEvent(logger).Error("fal_poll_status_failure", err).Log()
			return "", err
		}
		// Still processing
		logging.FluentEvent(logger).Info("fal_poll_status_processing").Log()
		return "", nil
	}

	err = fmt.Errorf("unexpected status payload format")
	logging.FluentEvent(logger).Error("fal_poll_status_failure", err).Log()
	return "", err
}

func (v *FalGenerator) fetchFinalResult(ctx context.Context, modelPath, jobID string) (string, error) {
	url := v.baseURL + modelPath + "/requests/" + jobID

	logger := logging.GetLoggerFromContext(ctx)
	logging.FluentEvent(logger).Info("fal_fetch_final_result_start").URL(url).Log()

	builder := api_builders.NewBuilder().
		SetURL(url).
		SetMethod(http.MethodGet).
		AddHeader("Authorization", "Key "+v.apiKey).
		SetClient(v.client).
		SetLogger(logger)

	resp, err := v.executeWithResilience(ctx, builder)
	if err != nil {
		logging.FluentEvent(logger).Error("fal_fetch_final_result_failure", err).Log()
		return "", err
	}
	defer resp.Body.Close()

	var final struct {
		Video struct {
			URL string `json:"url"`
		} `json:"video"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&final); err != nil {
		logging.FluentEvent(logger).Error("fal_fetch_final_result_failure", err).Log()
		return "", err
	}

	if final.Video.URL != "" {
		logging.FluentEvent(logger).Info("fal_fetch_final_result_success").URL(final.Video.URL).Log()
		return final.Video.URL, nil
	}

	err = fmt.Errorf("video URL not found in final payload")
	logging.FluentEvent(logger).Error("fal_fetch_final_result_failure", err).Log()
	return "", err
}

func (v *FalGenerator) executeWithResilience(ctx context.Context, builder api_builders.APIBuilder) (*http.Response, error) {
	if err := v.cb.AllowRequest(); err != nil {
		return nil, err
	}

	maxRetries := 3
	backoff := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err := builder.Build(ctx)

		if err != nil {
			if i == maxRetries-1 {
				v.cb.RecordFailure()
				return nil, err
			}
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			select {
			case <-ctx.Done():
				v.cb.RecordFailure()
				return nil, ctx.Err()
			case <-time.After(backoff + jitter):
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			_, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				logger := logging.GetLoggerFromContext(ctx)
				logging.FluentEvent(logger).Error("fal_resilience_read_body_error", readErr).Log()
			}
			resp.Body.Close()

			if i == maxRetries-1 {
				v.cb.RecordFailure()
				return resp, nil
			}
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			select {
			case <-ctx.Done():
				v.cb.RecordFailure()
				return nil, ctx.Err()
			case <-time.After(backoff + jitter):
			}
			backoff *= 2
			continue
		}

		v.cb.RecordSuccess()
		return resp, nil
	}

	v.cb.RecordFailure()
	return nil, fmt.Errorf("max retries exceeded")
}
