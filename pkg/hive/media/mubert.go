package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder"
	"github.com/zqk-os/zqk/pkg/specbuilder/api_builders"
	"github.com/zqk-os/zqk/packs/interface/bldr_v2"
)

// MubertGenerator implements the MediaGenerator interface for the Mubert API.
type MubertGenerator struct {
	companyID   string
	licenseTok  string
	customerID  string
	accessToken string
	baseURL     string
	client      specbuilder.APIClient
	logger      *logging.EventLogger
}

// NewMubertGenerator creates a new instance and authenticates.
func NewMubertGenerator(ctx context.Context) (*MubertGenerator, error) {
	companyID := zqkenv.MubertCompanyID().Get()
	licenseTok := zqkenv.MubertLicToken().Get()

	if companyID == "" || licenseTok == "" {
		return nil, ErrInvalidConfig
	}

	baseURL := config.LLMMubertBaseURL().OrDefault("https://music-api.mubert.com/api/v3")

	spec := specbuilder.APISpec{
		Name:    "Mubert AI",
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

	gen := &MubertGenerator{
		companyID:  companyID,
		licenseTok: licenseTok,
		baseURL:    baseURL,
		client:     apiClient,
		logger:     logging.GetLogger(),
	}

	authCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := gen.authenticate(authCtx); err != nil {
		return nil, fmt.Errorf("mubert auth failed: %w", err)
	}

	return gen, nil
}

func (m *MubertGenerator) authenticate(ctx context.Context) error {
	payload := map[string]string{
		"custom_id": "zqk_marketing_agent",
	}
	body, _ := json.Marshal(payload)

	if m.logger != nil {
		logging.FluentEvent(m.logger).Info("Mubert authentication started").Log()
	}

	apiSpec := map[string]any{
		objects.FieldKeyKind:             bldr_v2.NewAPISpecBuilder().GetOntology(),
		bldr_v2.APISpecFieldTargetSystem: "mubert",
		bldr_v2.APISpecFieldEndpoint:     m.baseURL + "/service/customers",
		bldr_v2.APISpecFieldMethod:       http.MethodPost,
		bldr_v2.APISpecFieldPayloadSchema: map[string]any{
			"custom_id": "string",
		},
	}

	schema := apiSpec[bldr_v2.APISpecFieldPayloadSchema].(map[string]any)
	for k := range payload {
		if _, ok := schema[k]; !ok {
			return fmt.Errorf("payload contains unknown field %s", k)
		}
	}

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "mubert",
		bldr_v2.APISpecFieldEndpoint:     m.baseURL + "/service/customers",
		"action":                         "authenticate",
		objects.FieldKeyStatus:           "started",
	})

	resp, err := api_builders.NewBuilder().
		SetURL(m.baseURL+"/service/customers").
		SetMethod(http.MethodPost).
		SetBody(body).
		AddHeader("Content-Type", "application/json").
		AddHeader("company-id", m.companyID).
		AddHeader("license-token", m.licenseTok).
		SetClient(m.client).
		SetLogger(m.logger).
		Build(ctx)

	if err != nil {
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert authentication failed", err).Log()
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("auth error %d: %s", resp.StatusCode, string(respBytes))
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert authentication failed", err).Log()
		}
		return err
	}

	var result struct {
		Data struct {
			ID     string `json:"id"`
			Access struct {
				Token string `json:"token"`
			} `json:"access"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert authentication decode failed", err).Log()
		}
		return err
	}

	m.customerID = result.Data.ID
	m.accessToken = result.Data.Access.Token

	if m.logger != nil {
		logging.FluentEvent(m.logger).Info("Mubert authentication completed").Log()
	}

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "mubert",
		bldr_v2.APISpecFieldEndpoint:     m.baseURL + "/service/customers",
		"action":                         "authenticate",
		objects.FieldKeyStatus:           "success",
	})

	return nil
}

// GenerateAudio creates a new background track using Mubert's generation API.
func (m *MubertGenerator) GenerateAudio(ctx context.Context, req AudioRequest) (string, error) {
	if m.logger != nil {
		logging.FluentEvent(m.logger).Info("Mubert GenerateAudio task started").Log()
	}

	payload := map[string]any{
		"mode":                 "track",
		"duration":             15,
		"bitrate":              128,
		objects.FieldKeyFormat: "mp3",
		"intensity":            "medium",
		"prompt":               req.Prompt,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert GenerateAudio task failed", err).Log()
		}
		return "", err
	}

	apiSpec := map[string]any{
		objects.FieldKeyKind:             bldr_v2.NewAPISpecBuilder().GetOntology(),
		bldr_v2.APISpecFieldTargetSystem: "mubert",
		bldr_v2.APISpecFieldEndpoint:     m.baseURL + "/public/tracks",
		bldr_v2.APISpecFieldMethod:       http.MethodPost,
		bldr_v2.APISpecFieldPayloadSchema: map[string]any{
			"mode":                 "string",
			"duration":             "integer",
			"bitrate":              "integer",
			objects.FieldKeyFormat: "string",
			"intensity":            "string",
			"prompt":               "string",
		},
	}

	schema := apiSpec[bldr_v2.APISpecFieldPayloadSchema].(map[string]any)
	for k := range payload {
		if _, ok := schema[k]; !ok {
			return "", fmt.Errorf("payload contains unknown field %s", k)
		}
	}

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "mubert",
		bldr_v2.APISpecFieldEndpoint:     m.baseURL + "/public/tracks",
		"action":                         "generate_audio",
		objects.FieldKeyStatus:           "started",
	})

	resp, err := api_builders.NewBuilder().
		SetURL(m.baseURL+"/public/tracks").
		SetMethod(http.MethodPost).
		SetBody(body).
		AddHeader("Content-Type", "application/json").
		AddHeader("customer-id", m.customerID).
		AddHeader("access-token", m.accessToken).
		SetClient(m.client).
		SetLogger(m.logger).
		Build(ctx)

	if err != nil {
		wrappedErr := fmt.Errorf("%w: %v", ErrGenerationFailed, err)
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert GenerateAudio task failed", wrappedErr).Log()
		}
		return "", wrappedErr
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		wrappedErr := fmt.Errorf("%w: API returned %d: %s", ErrGenerationFailed, resp.StatusCode, string(respBytes))
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert GenerateAudio task failed", wrappedErr).Log()
		}
		return "", wrappedErr
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		decodeErr := fmt.Errorf("failed to decode mubert response: %v", err)
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert GenerateAudio task failed", decodeErr).Log()
		}
		return "", decodeErr
	}

	if result.Data.ID == "" {
		emptyIDErr := fmt.Errorf("%w: no track id returned", ErrGenerationFailed)
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert GenerateAudio task failed", emptyIDErr).Log()
		}
		return "", emptyIDErr
	}

	if m.logger != nil {
		logging.FluentEvent(m.logger).Info("Mubert GenerateAudio task completed").JobID(result.Data.ID).Log()
	}

	_, _ = metrics.MetricPipelineForProject(nil, "").Sample(map[string]any{
		objects.FieldKeyKind:             "ai_interaction",
		bldr_v2.APISpecFieldTargetSystem: "mubert",
		bldr_v2.APISpecFieldEndpoint:     m.baseURL + "/public/tracks",
		"action":                         "generate_audio",
		objects.FieldKeyStatus:           "success",
		"job_id":                         result.Data.ID,
	})

	return result.Data.ID, nil
}

// GenerateVideo is not supported by Mubert.
func (m *MubertGenerator) GenerateVideo(ctx context.Context, req VideoRequest) (string, error) {
	return "", fmt.Errorf("mubert does not support video generation")
}

// PollStatus checks if the Mubert task is complete.
func (m *MubertGenerator) PollStatus(ctx context.Context, jobID string) (string, error) {
	if m.logger != nil {
		logging.FluentEvent(m.logger).Info("Mubert PollStatus task started").JobID(jobID).Log()
	}

	resp, err := api_builders.NewBuilder().
		SetURL(m.baseURL+"/public/tracks/"+jobID).
		SetMethod(http.MethodGet).
		AddHeader("customer-id", m.customerID).
		AddHeader("access-token", m.accessToken).
		SetClient(m.client).
		SetLogger(m.logger).
		Build(ctx)

	if err != nil {
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert PollStatus task failed", err).JobID(jobID).Log()
		}
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Generations []struct {
				Status string `json:"status"`
				URL    string `json:"url"`
			} `json:"generations"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		if m.logger != nil {
			logging.FluentEvent(m.logger).Error("Mubert PollStatus task failed", err).JobID(jobID).Log()
		}
		return "", err
	}

	if len(result.Data.Generations) > 0 {
		gen := result.Data.Generations[0]
		if gen.Status == "done" && gen.URL != "" {
			if m.logger != nil {
				logging.FluentEvent(m.logger).Info("Mubert PollStatus task completed").JobID(jobID).URL(gen.URL).Log()
			}
			return gen.URL, nil
		}
	}

	if m.logger != nil {
		logging.FluentEvent(m.logger).Info("Mubert PollStatus task in progress").JobID(jobID).Log()
	}
	return "", nil
}
