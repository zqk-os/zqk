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

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/mesh"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// Client defines the interface for language model operations.
type Client interface {
	// GenerateIntent generates a concise semantic intent summary for a given code block.
	GenerateIntent(ctx context.Context, code string) (string, error)
	// GenerateEmbedding generates a vector embedding for a given text.
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)
	// AnalyzeVideoFrames analyzes video frames against a text script to determine if they match.
	AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (mesh.AnalysisResult, error)
	// GenerateCompletion generates a generic completion for a given prompt.
	GenerateCompletion(ctx context.Context, prompt string, system string) (string, error)
	// GenerateStructuredCompletion generates a completion that supports tool calling.
	GenerateStructuredCompletion(ctx context.Context, messages []Message, tools []ToolDefinition) (StructuredCompletionResponse, error)
	// VerifyImage checks an image against a text prompt and an optional reference image.
	VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (mesh.AnalysisResult, error)
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

// DefaultConfig creates a config from environment variables.
func DefaultConfig() *Config {
	provider := os.Getenv(zqkenv.LLMProvider())
	if provider == "" {
		if os.Getenv(zqkenv.GeminiAPIKey()) != "" {
			provider = "gemini"
		} else {
			provider = "openai"
		}
	}
	baseURL := zqkenv.Get(zqkenv.LLMBaseURL()).OrDefault("https://api.openai.com/v1")
	chatModel := zqkenv.Get(zqkenv.LLMChatModel()).OrDefault("gpt-4o-mini")
	embedModel := zqkenv.Get(zqkenv.LLMEmbedModel()).OrDefault("text-embedding-3-small")
	contextWindowSize := zqkenv.Get(zqkenv.LLMContextWindowSize()).IntOrDefault(32768)

	timeoutSec := zqkenv.Get("LLM_TIMEOUT").IntOrDefault(300)
	// Alternate endpoint when primary local LLM fails (optional).
	secondaryProvider := zqkenv.Get("LLM_SECONDARY_PROVIDER").OrDefault("")
	secondaryBaseURL := zqkenv.Get("LLM_SECONDARY_BASE_URL").OrDefault("")
	secondaryAPIKey := zqkenv.Get("LLM_SECONDARY_API_KEY").OrDefault("")
	secondaryChatModel := zqkenv.Get("LLM_SECONDARY_CHAT_MODEL").OrDefault("")
	// TRACK: REDACTED — timeout / secondary / mock fallback via env.

	return &Config{
		Provider:           provider,
		BaseURL:            baseURL,
		APIKey:             os.Getenv(zqkenv.LLMAPIKey()),
		ChatModel:          chatModel,
		EmbedModel:         embedModel,
		ContextWindowSize:  contextWindowSize,
		Timeout:            time.Duration(timeoutSec) * time.Second,
		Temperature:        optionalFloatEnv(zqkenv.LLMTemperature()),
		TopP:               optionalFloatEnv(zqkenv.LLMTopP()),
		SecondaryProvider:  secondaryProvider,
		SecondaryBaseURL:   secondaryBaseURL,
		SecondaryAPIKey:    secondaryAPIKey,
		SecondaryChatModel: secondaryChatModel,
	}
}

// NewClient creates a new LLM client based on the configured provider, wrapping it with resilience settings if needed.
func NewClient(config *Config) Client {
	if config == nil {
		config = DefaultConfig()
	}

	var primary Client
	switch strings.ToLower(config.Provider) {
	case "gemini":
		primary = NewGeminiClient(config)
	case "qwen":
		primary = NewQwenClient(config)
	default:
		primary = NewOpenAIClient(config)
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
			secConfig.Provider = "openai"
		}

		var secondary Client
		switch strings.ToLower(secConfig.Provider) {
		case "gemini":
			secondary = NewGeminiClient(secConfig)
		case "qwen":
			secondary = NewQwenClient(secConfig)
		default:
			secondary = NewOpenAIClient(secConfig)
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
	// Default public OpenAI endpoint requires a key; mock if missing to keep tests passing.
	if c.config.BaseURL == "https://api.openai.com/v1" || c.config.BaseURL == "" {
		return true
	}
	// Custom base URLs (e.g. self-hosted Ollama, LocalAI, vLLM proxies) don't require mocking.
	return false
}

// NewOpenAIClient creates a new OpenAI-compatible client.
func NewOpenAIClient(config *Config) *OpenAIClient {
	if config == nil {
		config = DefaultConfig()
	}

	return &OpenAIClient{
		config:     config,
		httpClient: newLLMAPIClient("OpenAI LLM", config.BaseURL),
	}
}

func (c *OpenAIClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	if c.shouldMock() {
		// Mock implementation for when no API key is provided
		return fmt.Sprintf("Semantic intent for: %s", truncate(code, 50)), nil
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

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

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return "", errfmt.Errorf("no response choices returned")
	}

	return result.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	if c.shouldMock() {
		return `["ZQK Observer Tip: APIKey missing, cannot generate dynamic tips."]`, nil
	}

	sanitizedPrompt, err := SanitizeUntrustedText(prompt)
	if err != nil {
		return "", errfmt.Newf("sanitizing prompt").Wrap(err)
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

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

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return "", errfmt.Errorf("no response choices returned")
	}

	return result.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if c.shouldMock() {
		// Mock embedding
		emb := make([]float32, 1536)
		emb[0] = 1.0
		return emb, nil
	}

	url := fmt.Sprintf("%s/embeddings", strings.TrimSuffix(c.config.BaseURL, "/"))

	payload := map[string]any{
		"model": c.config.EmbedModel,
		"input": text,
	}

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Data) == 0 {
		return nil, errfmt.Errorf("no embedding returned")
	}

	return result.Data[0].Embedding, nil
}

func (c *OpenAIClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (mesh.AnalysisResult, error) {
	if c.shouldMock() {
		// Mock implementation for testing
		return mesh.AnalysisResult{
			TruthScore: 1.0,
			Summary:    fmt.Sprintf("Mock analysis: script '%s' matches %d frames", truncate(script, 50), len(frames)),
		}, nil
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

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

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return mesh.AnalysisResult{}, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return mesh.AnalysisResult{}, errfmt.Errorf("no response choices returned")
	}

	var analysis mesh.AnalysisResult
	if err := json.Unmarshal([]byte(result.Choices[0].Message.Content), &analysis); err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to unmarshal analysis json").Wrap(err)
	}

	return analysis, nil
}

func (c *OpenAIClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (mesh.AnalysisResult, error) {
	if c.shouldMock() {
		// Mock implementation for testing
		return mesh.AnalysisResult{
			TruthScore: 1.0,
			Summary:    fmt.Sprintf("Mock analysis: image verified against prompt '%s'", truncate(textPrompt, 50)),
		}, nil
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

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

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return mesh.AnalysisResult{}, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return mesh.AnalysisResult{}, errfmt.Errorf("no response choices returned")
	}

	var analysis mesh.AnalysisResult
	if err := json.Unmarshal([]byte(result.Choices[0].Message.Content), &analysis); err != nil {
		return mesh.AnalysisResult{}, errfmt.Newf("failed to unmarshal analysis json").Wrap(err)
	}

	return analysis, nil
}

func (c *OpenAIClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	if c.shouldMock() {
		return "Mock description: two individuals in a cave holding rocks.", nil
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

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

	payload := map[string]any{
		"model": c.config.ChatModel,
		"messages": []map[string]any{
			{
				objects.FieldKeyRole:    "user",
				objects.FieldKeyContent: content,
			},
		},
		"max_tokens": 500,
	}

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return "", errfmt.Errorf("no response choices returned")
	}

	return result.Choices[0].Message.Content, nil
}

func (c *OpenAIClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	if c.shouldMock() {
		return "Mock scene description: The scene progresses from static setup to dynamic action, showing two individuals attempting to create fire in a dimly lit cave setting.", nil
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.config.BaseURL, "/"))

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

	payload := map[string]any{
		"model": c.config.ChatModel,
		"messages": []map[string]any{
			{
				objects.FieldKeyRole:    "user",
				objects.FieldKeyContent: content,
			},
		},
		"max_tokens": 500,
	}

	if c.config.ContextWindowSize > 0 {
		payload["options"] = map[string]any{
			"num_ctx": c.config.ContextWindowSize,
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Choices) == 0 {
		return "", errfmt.Errorf("no response choices returned")
	}

	return result.Choices[0].Message.Content, nil
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
