package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder"
	"github.com/lanceman/zqk/pkg/specbuilder/api_builders"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_v2"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// OpenAITTSGenerator implements the MediaGenerator interface for OpenAI TTS API.
type OpenAITTSGenerator struct {
	apiKey  string
	baseURL string
	client  specbuilder.APIClient
}

// NewOpenAITTSGenerator creates a new instance.
func NewOpenAITTSGenerator() (*OpenAITTSGenerator, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, ErrInvalidConfig
	}

	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	spec := specbuilder.APISpec{
		Name:    "OpenAI TTS",
		BaseURL: baseURL,
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

	return &OpenAITTSGenerator{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  apiClient,
	}, nil
}

// GenerateAudio synchronously generates audio using OpenAI TTS API.
func (o *OpenAITTSGenerator) GenerateAudio(ctx context.Context, req AudioRequest) (string, error) {
	logger := logging.GetLoggerFromContext(ctx)
	url := o.baseURL + "/audio/speech"

	// Using the provided Prompt as input text
	payload := map[string]any{
		"model": "tts-1",
		"input": req.Prompt,
		"voice": "alloy", // default voice
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	logging.FluentEvent(logger).Info("openai_tts_start").URL(url).Log()

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "openai",
		bldr_v2.APISpecFieldEndpoint:     url,
		"action":                         "generate_audio",
		objects.FieldKeyStatus:           "started",
	})

	builder := api_builders.NewBuilder().
		SetURL(url).
		SetMethod(http.MethodPost).
		AddHeader("Content-Type", "application/json").
		AddHeader("Authorization", "Bearer "+o.apiKey).
		SetBody(body).
		SetClient(o.client).
		SetLogger(logger)

	resp, err := builder.Build(ctx)
	if err != nil {
		logging.FluentEvent(logger).Error("openai_tts_failure", err).Log()
		return "", fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBytes))
		logging.FluentEvent(logger).Error("openai_tts_failure", err).Log()
		return "", fmt.Errorf("%w: %v", ErrGenerationFailed, err)
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.FluentEvent(logger).Error("openai_tts_failure", err).Log()
		return "", err
	}

	// We'll write this to a temporary file and return the path since OpenAI's TTS is synchronous.
	tmpFile, err := fileutil.CreateTemp("", "openai_tts_*.mp3")
	if err != nil {
		logging.FluentEvent(logger).Error("openai_tts_failure", err).Log()
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(audioData); err != nil {
		logging.FluentEvent(logger).Error("openai_tts_failure", err).Log()
		return "", err
	}

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "openai",
		bldr_v2.APISpecFieldEndpoint:     url,
		"action":                         "generate_audio",
		objects.FieldKeyStatus:           "success",
	})

	logging.FluentEvent(logger).Info("openai_tts_success").Log()

	// Prepending file:// to simulate an asset URL that can be copied
	return "file://" + tmpFile.Name(), nil
}

func (o *OpenAITTSGenerator) GenerateVideo(ctx context.Context, req VideoRequest) (string, error) {
	return "", fmt.Errorf("openai tts generator does not support video generation")
}

func (o *OpenAITTSGenerator) PollStatus(ctx context.Context, jobID string) (string, error) {
	// For synchronous generators that return local files via GenerateAudio
	return jobID, nil
}
