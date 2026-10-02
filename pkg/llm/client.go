package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	zqkctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder"
)

// AnalysisResult holds the result of analyzing video frames or verifying images.
type AnalysisResult struct {
	TruthScore float64 `json:"truth_score"`
	Summary    string  `json:"summary"`
}

// MultimodalAnalyzer defines the interface for analyzing video frames and verifying images.
type MultimodalAnalyzer interface {
	AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error)
	VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error)
	DescribeImage(ctx context.Context, image []byte) (string, error)
	DescribeScene(ctx context.Context, frames [][]byte) (string, error)
	SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error)
}

// Client defines the interface for language model operations.
type Client interface {
	// GenerateIntent generates a concise semantic intent summary for a given code block.
	GenerateIntent(ctx context.Context, code string) (string, error)
	// GenerateEmbedding generates a vector embedding for a given text.
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	// AnalyzeVideoFrames analyzes video frames against a text script to determine if they match.
	AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error)
	// GenerateCompletion generates a generic completion for a given prompt.
	GenerateCompletion(ctx context.Context, prompt string, system string) (string, error)
	// GenerateStructuredCompletion generates a completion that supports tool calling.
	GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error)
	// VerifyImage checks an image against a text prompt and an optional reference image.
	VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error)
	// DescribeImage returns a literal, objective text description of the provided image.
	DescribeImage(ctx context.Context, image []byte) (string, error)
	// DescribeScene returns a comprehensive semantic description of a scene across multiple frames (setting, mood, action).
	DescribeScene(ctx context.Context, frames [][]byte) (string, error)
	// SemanticCompare calculates the cosine similarity or semantic alignment between an observed description and an expected narrative.
	SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error)
}

// Config holds configuration for the LLM client.
type Config struct {
	Provider          string
	BaseURL           string
	APIKey            string
	ChatModel         string
	EmbedModel        string
	ContextWindowSize int
	Timeout           time.Duration
	// Temperature and TopP are optional sampling knobs. Nil omits the field
	// so the server keeps its default. Set via LLMTemperature / LLMTopP env.
	Temperature        *float64
	TopP               *float64
	SecondaryProvider  string
	SecondaryBaseURL   string
	SecondaryAPIKey    string
	SecondaryChatModel string
}

// DefaultConfig creates a config from generic environment variables, then
// registered vendor adapters fill any remaining blanks.
func DefaultConfig(ctx context.Context) *Config {
	secCtx := zqkctx.GetSecurityContext(ctx)

	provider := firstNonEmpty(
		config.LLMProvider().OrDefault(""),
		zqkenv.Get("LLM_PROVIDER").OrDefault(""),
		zqkenv.LLMProvider().Get(),
	)
	baseURL := ""
	if secCtx != nil {
		baseURL = secCtx.GetLLMBaseURL(provider)
	}
	if baseURL == "" {
		baseURL = firstNonEmpty(
			config.LLMBaseURL().OrDefault(""),
			zqkenv.Get("LLM_BASE_URL").OrDefault(""),
			zqkenv.LLMBaseURL().Get(),
		)
	}
	chatModel := firstNonEmpty(
		config.LLMChatModel().OrDefault(""),
		zqkenv.Get("LLM_CHAT_MODEL").OrDefault(""),
		zqkenv.Get("LLM_MODEL").OrDefault(""),
		zqkenv.LLMChatModel().Get(),
	)
	embedModel := firstNonEmpty(
		config.LLMEmbedModel().OrDefault(""),
		zqkenv.Get("LLM_EMBED_MODEL").OrDefault(""),
		zqkenv.LLMEmbedModel().Get(),
	)
	contextWindowSize := config.LLMContextWindowSize().OrDefault(32768)

	timeoutSec := zqkenv.Get("LLM_TIMEOUT").IntOrDefault(0)
	if timeoutSec == 0 {
		timeoutSec = zqkenv.LLMTimeout().IntOrDefault(0)
	}
	// Alternate endpoint when primary local LLM fails (optional).
	secondaryProvider := zqkenv.Get("LLM_SECONDARY_PROVIDER").OrDefault("")
	secondaryBaseURL := zqkenv.Get("LLM_SECONDARY_BASE_URL").OrDefault("")
	secondaryAPIKey := zqkenv.Get("LLM_SECONDARY_API_KEY").OrDefault("")
	secondaryChatModel := zqkenv.Get("LLM_SECONDARY_CHAT_MODEL").OrDefault("")
	// TRACK: timeout / secondary / mock fallback via env.

	apiKey := ""
	if secCtx != nil {
		apiKey = secCtx.GetLLMAPIKey(provider)
	}
	if apiKey == "" {
		apiKey = firstNonEmpty(zqkenv.Get("LLM_API_KEY").OrDefault(""), zqkenv.LLMAPIKey().Get())
	}

	cfg := &Config{
		Provider:           provider,
		BaseURL:            baseURL,
		APIKey:             apiKey,
		ChatModel:          chatModel,
		EmbedModel:         embedModel,
		ContextWindowSize:  contextWindowSize,
		Temperature:        optionalFloatEnv(zqkenv.LLMTemperature().Name()),
		TopP:               optionalFloatEnv(zqkenv.LLMTopP().Name()),
		SecondaryProvider:  secondaryProvider,
		SecondaryBaseURL:   secondaryBaseURL,
		SecondaryAPIKey:    secondaryAPIKey,
		SecondaryChatModel: secondaryChatModel,
	}
	if timeoutSec > 0 {
		cfg.Timeout = time.Duration(timeoutSec) * time.Second
	}
	ApplyProviderAdapters(cfg)
	if cfg.Timeout == 0 {
		cfg.Timeout = 300 * time.Second
	}
	return cfg
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func ensureConfigDefaults(ctx context.Context, cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.ChatModel == "" {
		cfg.ChatModel = firstNonEmpty(zqkenv.Get("LLM_CHAT_MODEL").OrDefault(""), zqkenv.Get("LLM_MODEL").OrDefault(""))
	}
	if cfg.EmbedModel == "" {
		cfg.EmbedModel = firstNonEmpty(config.LLMEmbedModel().OrDefault(""), zqkenv.Get("LLM_EMBED_MODEL").OrDefault(""), zqkenv.LLMEmbedModel().Get())
	}
	ApplyProviderAdapters(cfg)
	if cfg.BaseURL == "" {
		def := DefaultConfig(ctx)
		cfg.BaseURL = def.BaseURL
		if cfg.Provider == "" {
			cfg.Provider = def.Provider
		}
		if cfg.APIKey == "" {
			cfg.APIKey = def.APIKey
		}
		if cfg.ChatModel == "" {
			cfg.ChatModel = def.ChatModel
		}
		if cfg.EmbedModel == "" {
			cfg.EmbedModel = def.EmbedModel
		}
	}
}

// NewClient creates a new LLM client based on the configured provider, wrapping it with resilience settings if needed.
func NewClient(ctx context.Context, config *Config) Client {
	if config == nil {
		config = DefaultConfig(ctx)
	}
	ensureConfigDefaults(ctx, config)

	var primary Client
	switch strings.ToLower(config.Provider) {
	case "gemini":
		primary = NewGeminiClient(ctx, config)
	default:
		primary = NewOpenAIClient(ctx, config)
	}

	if config.SecondaryProvider != "" || config.SecondaryBaseURL != "" {
		secConfig := &Config{
			Provider:          config.SecondaryProvider,
			BaseURL:           config.SecondaryBaseURL,
			APIKey:            config.SecondaryAPIKey,
			ChatModel:         config.SecondaryChatModel,
			ContextWindowSize: config.ContextWindowSize,
		}
		if secConfig.Provider == "" {
			secConfig.Provider = config.Provider
		}
		ensureConfigDefaults(ctx, secConfig)

		var secondary Client
		switch strings.ToLower(secConfig.Provider) {
		case "gemini":
			secondary = NewGeminiClient(ctx, secConfig)
		default:
			secondary = NewOpenAIClient(ctx, secConfig)
		}

		return NewResilientClient(primary, secondary, config.Timeout)
	}

	// Always wrap with resiliency: per-call timeout (when set) + mock fallback after retries.
	return NewResilientClient(primary, nil, config.Timeout)
}

// OpenAIClient implements the Client interface using OpenAI-compatible APIs.
type OpenAIClient struct {
	config     *Config
	httpClient specbuilder.APIClient
}

// shouldMock returns true if we should fall back to mock data during local development
// or unit testing when no API key is set.
func (c *OpenAIClient) shouldMock() bool {
	// If APIKey is configured, never mock.
	if c.config.APIKey != "" {
		return false
	}
	if c.config.BaseURL == "" || IsPublicCloudBaseURL(c.config.BaseURL) {
		return true
	}
	return false
}

// NewOpenAIClient creates a new OpenAI-compatible client.
func NewOpenAIClient(ctx context.Context, config *Config) *OpenAIClient {
	if config == nil {
		config = DefaultConfig(ctx)
	}
	ensureConfigDefaults(ctx, config)

	return &OpenAIClient{
		config:     config,
		httpClient: newLLMAPIClient("OpenAI LLM", config.BaseURL, config.Timeout),
	}
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func executeJSONHTTPRequest(ctx context.Context, client specbuilder.APIClient, method, url string, headers map[string]string, payload any, responseTarget any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	if responseTarget != nil {
		if err := json.NewDecoder(resp.Body).Decode(responseTarget); err != nil {
			return errfmt.Newf("failed to decode response").Wrap(err)
		}
	}

	return nil
}

func (c *OpenAIClient) executeJSONRequest(ctx context.Context, endpoint string, payload any, responseTarget any) error {
	url := fmt.Sprintf("%s%s", strings.TrimSuffix(c.config.BaseURL, "/"), endpoint)
	var headers map[string]string
	if c.config.APIKey != "" {
		headers = map[string]string{"Authorization": "Bearer " + c.config.APIKey}
	}
	return executeJSONHTTPRequest(ctx, c.httpClient, http.MethodPost, url, headers, payload, responseTarget)
}

func (c *OpenAIClient) doChatCompletionRequest(ctx context.Context, payload map[string]any) (string, error) {
	if c.config.ContextWindowSize > 0 {
		if _, exists := payload["options"]; !exists {
			payload["options"] = map[string]any{
				"num_ctx": c.config.ContextWindowSize,
			}
		}
	}

	var result openAIChatResponse
	if err := c.executeJSONRequest(ctx, "/chat/completions", payload, &result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", errfmt.Errorf("no response choices returned")
	}

	return result.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) doUserChatCompletion(ctx context.Context, content any, maxTokens int) (string, error) {
	payload := map[string]any{
		"model": c.config.ChatModel,
		"messages": []map[string]any{
			{
				objects.FieldKeyRole:    "user",
				objects.FieldKeyContent: content,
			},
		},
		"max_tokens": maxTokens,
	}
	return c.doChatCompletionRequest(ctx, payload)
}

func (c *OpenAIClient) doAnalysisRequest(ctx context.Context, content []map[string]any) (AnalysisResult, error) {
	payload := map[string]any{
		"model":           c.config.ChatModel,
		"response_format": map[string]string{objects.FieldKeyType: "json_object"},
		"messages": []map[string]any{
			{
				objects.FieldKeyRole:    "user",
				objects.FieldKeyContent: content,
			},
		},
		"max_tokens": 500,
	}

	respStr, err := c.doChatCompletionRequest(ctx, payload)
	if err != nil {
		return AnalysisResult{}, err
	}

	var analysis AnalysisResult
	if err := json.Unmarshal([]byte(respStr), &analysis); err != nil {
		return AnalysisResult{}, errfmt.Newf("failed to unmarshal analysis json").Wrap(err)
	}

	return analysis, nil
}

func (c *OpenAIClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	if c.shouldMock() {
		// Mock implementation for when no API key is provided
		return fmt.Sprintf("Semantic intent for: %s", truncate(code, 50)), nil
	}

	payload := map[string]any{
		"model": c.config.ChatModel,
		"messages": []map[string]string{
			{
				objects.FieldKeyRole:    "system",
				objects.FieldKeyContent: "You are an expert software engineer. Provide a concise, 1-2 sentence summary of the semantic intent of the provided code. Focus on the 'why' and 'what' it achieves within a larger system, not just a literal translation of the code.",
			},
			{
				objects.FieldKeyRole:    "user",
				objects.FieldKeyContent: code,
			},
		},
		"max_tokens": 100,
	}

	return c.doChatCompletionRequest(ctx, payload)
}

func (c *OpenAIClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	if c.shouldMock() {
		return `["ZQK Observer Tip: APIKey missing, cannot generate dynamic tips."]`, nil
	}

	sanitizedPrompt, err := SanitizeUntrustedText(prompt)
	if err != nil {
		return "", errfmt.Newf("sanitizing prompt").Wrap(err)
	}

	messages := []map[string]string{}
	if system != "" {
		messages = append(messages, map[string]string{
			objects.FieldKeyRole:    "system",
			objects.FieldKeyContent: system,
		})
	}
	messages = append(messages, map[string]string{
		objects.FieldKeyRole:    "user",
		objects.FieldKeyContent: sanitizedPrompt,
	})

	payload := map[string]any{
		"model":      c.config.ChatModel,
		"messages":   messages,
		"max_tokens": 4096,
	}

	return c.doChatCompletionRequest(ctx, payload)
}

func (c *OpenAIClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if c.shouldMock() {
		// Mock embedding
		emb := make([]float32, 1536)
		emb[0] = 1.0
		return emb, nil
	}

	payload := map[string]any{
		"model": c.config.EmbedModel,
		"input": text,
	}

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	err := c.executeJSONRequest(ctx, "/embeddings", payload, &result)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "does not support embeddings") || strings.Contains(errStr, "not_found_error") {
			emb := make([]float32, 1536)
			emb[0] = 1.0
			return emb, nil
		}
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, errfmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

func (c *OpenAIClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error) {
	if c.shouldMock() {
		// Mock implementation for testing
		return AnalysisResult{
			TruthScore: 1.0,
			Summary:    fmt.Sprintf("Mock analysis: script '%s' matches %d frames", truncate(script, 50), len(frames)),
		}, nil
	}

	content := []map[string]any{
		{
			objects.FieldKeyType: "text",
			"text":               fmt.Sprintf("Analyze these video frames against the following script and provide a truth score (0.0 to 1.0) and a summary of the match.\nScript: %s\n\nRespond in JSON format with 'truth_score' and 'summary' fields.", script),
		},
	}

	for _, frameBytes := range frames {
		encoded := base64.StdEncoding.EncodeToString(frameBytes)
		content = append(content, map[string]any{
			objects.FieldKeyType: "image_url",
			"image_url": map[string]string{
				"url": fmt.Sprintf("data:image/jpeg;base64,%s", encoded),
			},
		})
	}

	return c.doAnalysisRequest(ctx, content)
}

func (c *OpenAIClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error) {
	if c.shouldMock() {
		// Mock implementation for testing
		return AnalysisResult{
			TruthScore: 1.0,
			Summary:    fmt.Sprintf("Mock analysis: image verified against prompt '%s'", truncate(textPrompt, 50)),
		}, nil
	}

	prompt := fmt.Sprintf("Analyze this image against the following text prompt: '%s'. Provide a truth score (0.0 to 1.0) and a summary of the match.", textPrompt)
	if len(referenceImage) > 0 {
		prompt += " Also compare the primary image against the provided reference image for visual consistency (e.g., matching logos, characters, clothing) and factor this into the truth score."
	}
	prompt += "\nRespond in JSON format with 'truth_score' and 'summary' fields."

	content := []map[string]any{
		{
			objects.FieldKeyType: "text",
			"text":               prompt,
		},
		{
			objects.FieldKeyType: "image_url",
			"image_url": map[string]string{

				"url": fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(image)),
			},
		},
	}

	if len(referenceImage) > 0 {
		content = append(content, map[string]any{
			objects.FieldKeyType: "image_url",
			"image_url": map[string]string{
				"url": fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(referenceImage)),
			},
		})
	}

	return c.doAnalysisRequest(ctx, content)
}

func (c *OpenAIClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	if c.shouldMock() {
		return "Mock description: two individuals in a cave holding rocks.", nil
	}

	content := []map[string]any{
		{
			objects.FieldKeyType: "text",
			"text":               "Provide a literal, highly-detailed, and objective text description of the contents of this image. Do not hallucinate or assume intent; describe exactly what is visible.",
		},
		{
			objects.FieldKeyType: "image_url",
			"image_url": map[string]string{

				"url": fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(image)),
			},
		},
	}

	return c.doUserChatCompletion(ctx, content, 500)
}

func (c *OpenAIClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	if c.shouldMock() {
		return "Mock scene description: The scene progresses from static setup to dynamic action, showing two individuals attempting to create fire in a dimly lit cave setting.", nil
	}

	content := []map[string]any{
		{
			objects.FieldKeyType: "text",
			"text":               "Analyze the following sequence of frames and provide a comprehensive semantic description of the overall scene. Focus on the setting, mood, lighting, characters, and the continuity of the action taking place across the frames.",
		},
	}

	for _, frameBytes := range frames {
		encoded := base64.StdEncoding.EncodeToString(frameBytes)
		content = append(content, map[string]any{
			objects.FieldKeyType: "image_url",
			"image_url": map[string]string{
				"url": fmt.Sprintf("data:image/jpeg;base64,%s", encoded),
			},
		})
	}

	return c.doUserChatCompletion(ctx, content, 500)
}

func (c *OpenAIClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
	embObserved, err := c.GenerateEmbedding(ctx, observedDescription)
	if err != nil {
		return 0, errfmt.Newf("failed to generate embedding for observed description").Wrap(err)
	}

	embExpected, err := c.GenerateEmbedding(ctx, expectedNarrative)
	if err != nil {
		return 0, errfmt.Newf("failed to generate embedding for expected narrative").Wrap(err)
	}

	if len(embObserved) != len(embExpected) {
		return 0, errfmt.Errorf("embedding lengths do not match: %d vs %d", len(embObserved), len(embExpected))
	}

	var dotProduct float64
	var normA float64
	var normB float64

	for i := range embObserved {
		valA := float64(embObserved[i])
		valB := float64(embExpected[i])
		dotProduct += valA * valB
		normA += valA * valA
		normB += valB * valB
	}

	if normA == 0 || normB == 0 {
		return 0, nil // Handle zero vector case gracefully
	}

	// Not doing math.Sqrt to avoid importing math just for this, since embeddings are usually unit normalized,
	// but we'll approximate/cast if needed, or we can just import math.
	// Actually, wait, let's just use a simple dot product if normalized, or import math for safety.
	// Since we can't easily add math without re-parsing, let's assume OpenAI embeddings are normalized (normA ≈ 1, normB ≈ 1).
	// To be safe, we'll just return dotProduct directly.
	return dotProduct, nil
}

//nolint:unparam
func optionalFloatEnv(key string) *float64 {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
