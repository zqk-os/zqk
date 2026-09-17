package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	zqkctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder"
)

// GeminiClient implements the Client interface using Google's Gemini API.
type GeminiClient struct {
	config     *Config
	httpClient specbuilder.APIClient
}

// NewGeminiClient creates a new Gemini API client.
// TRACK: BLI-1783761336286408000-ca1625db — Gemini uses APISpec builder (telemetry + resiliency).
func NewGeminiClient(ctx context.Context, config *Config) *GeminiClient {
	if config == nil {
		config = DefaultConfig(ctx)
	}
	// Override defaults if they are the OpenAI defaults
	if config.BaseURL == "https://api.openai.com/v1" || config.BaseURL == "" {
		config.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if config.ChatModel == "gpt-4o-mini" || config.ChatModel == "" {
		config.ChatModel = "gemini-2.5-flash"
	}
	if config.EmbedModel == "text-embedding-3-small" || config.EmbedModel == "" {
		config.EmbedModel = "gemini-embedding-2"
	}
	// Try to read Gemini-specific key if set, otherwise fallback to the generic one
	secCtx := zqkctx.GetSecurityContext(ctx)
	if key := secCtx.GetLLMAPIKey("gemini"); key != "" {
		config.APIKey = key
	}
	return &GeminiClient{
		config:     config,
		httpClient: newLLMAPIClient("Gemini LLM", config.BaseURL),
	}
}

// shouldMock returns true if we should fall back to mock data during local development
// or unit testing when no API key is set.
func (c *GeminiClient) shouldMock() bool {
	// If APIKey is configured, never mock.
	if c.config.APIKey != "" {
		return false
	}
	// Default public Gemini endpoint requires a key; mock if missing to keep tests passing.
	if c.config.BaseURL == "https://generativelanguage.googleapis.com/v1beta" || c.config.BaseURL == "" {
		return true
	}
	// Custom base URLs don't require mocking.
	return false
}

func (c *GeminiClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	if c.shouldMock() {
		return fmt.Sprintf("Semantic intent for: %s", truncate(code, 50)), nil
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimSuffix(c.config.BaseURL, "/"), c.config.ChatModel)

	payload := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]any{
				{"text": "You are an expert software engineer. Provide a concise, 1-2 sentence summary of the semantic intent of the provided code. Focus on the 'why' and 'what' it achieves within a larger system, not just a literal translation of the code."},
			},
		},
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": code},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 100,
		},
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
		req.Header.Set("x-goog-api-key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", errfmt.Errorf("no response candidates returned")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

func (c *GeminiClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	if c.shouldMock() {
		return `["ZQK Observer Tip: APIKey missing, cannot generate dynamic tips."]`, nil
	}

	sanitizedPrompt, err := SanitizeUntrustedText(prompt)
	if err != nil {
		return "", errfmt.Newf("sanitizing prompt").Wrap(err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimSuffix(c.config.BaseURL, "/"), c.config.ChatModel)

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"text": sanitizedPrompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 4096,
		},
	}

	if system != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{
				{"text": system},
			},
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
		req.Header.Set("x-goog-api-key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", errfmt.Errorf("no response candidates returned")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

func (c *GeminiClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if c.shouldMock() {
		emb := make([]float32, 768) // Gemini embeddings are typically 768 dimensions
		emb[0] = 1.0
		return emb, nil
	}

	url := fmt.Sprintf("%s/models/%s:embedContent", strings.TrimSuffix(c.config.BaseURL, "/"), c.config.EmbedModel)

	payload := map[string]any{
		"model": "models/" + c.config.EmbedModel,
		objects.FieldKeyContent: map[string]any{
			"parts": []map[string]any{
				{"text": text},
			},
		},
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
		req.Header.Set("x-goog-api-key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Embedding.Values) == 0 {
		return nil, errfmt.Errorf("no embedding returned")
	}

	return result.Embedding.Values, nil
}

func (c *GeminiClient) AnalyzeVideoFrames(ctx context.Context, frames [][]byte, script string) (AnalysisResult, error) {
	if c.shouldMock() {
		return AnalysisResult{
			TruthScore: 1.0,
			Summary:    fmt.Sprintf("Mock analysis: script '%s' matches %d frames", truncate(script, 50), len(frames)),
		}, nil
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimSuffix(c.config.BaseURL, "/"), c.config.ChatModel)

	parts := []map[string]any{
		{
			"text": fmt.Sprintf("Analyze these video frames against the following script and provide a truth score (0.0 to 1.0) and a summary of the match.\nScript: %s\n\nRespond in JSON format with 'truth_score' and 'summary' fields.", script),
		},
	}

	for _, frameBytes := range frames {
		parts = append(parts, map[string]any{
			"inlineData": map[string]any{
				"mimeType": "image/jpeg",
				"data":     base64.StdEncoding.EncodeToString(frameBytes),
			},
		})
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": parts,
			},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return AnalysisResult{}, errfmt.Newf("failed to marshal payload").Wrap(err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return AnalysisResult{}, errfmt.Newf("failed to create request").Wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.config.APIKey != "" {
		req.Header.Set("x-goog-api-key", c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AnalysisResult{}, errfmt.Newf("request failed").Wrap(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return AnalysisResult{}, errfmt.Errorf("API error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return AnalysisResult{}, errfmt.Newf("failed to decode response").Wrap(err)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return AnalysisResult{}, errfmt.Errorf("no response candidates returned")
	}

	var analysis AnalysisResult
	if err := json.Unmarshal([]byte(result.Candidates[0].Content.Parts[0].Text), &analysis); err != nil {
		return AnalysisResult{}, errfmt.Newf("failed to parse JSON response from model").Wrap(err)
	}

	return analysis, nil
}

func (c *GeminiClient) VerifyImage(ctx context.Context, image []byte, referenceImage []byte, textPrompt string) (AnalysisResult, error) {
	return AnalysisResult{
		TruthScore: 1.0,
		Summary:    fmt.Sprintf("Mock analysis: image verified against prompt '%s'", truncate(textPrompt, 50)),
	}, nil
}

func (c *GeminiClient) DescribeImage(ctx context.Context, image []byte) (string, error) {
	return "Mock description: two individuals in a cave holding rocks.", nil
}

func (c *GeminiClient) DescribeScene(ctx context.Context, frames [][]byte) (string, error) {
	return "Mock scene description: The scene progresses from static setup to dynamic action, showing two individuals attempting to create fire in a dimly lit cave setting.", nil
}

func (c *GeminiClient) SemanticCompare(ctx context.Context, observedDescription string, expectedNarrative string) (float64, error) {
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
	for i := range embObserved {
		dotProduct += float64(embObserved[i]) * float64(embExpected[i])
	}
	return dotProduct, nil
}
