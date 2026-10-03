package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	zqkctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder"
)

// GeminiClient implements the Client interface using Google's Gemini API.
type GeminiClient struct {
	config     *Config
	httpClient specbuilder.APIClient
}

// NewGeminiClient creates a new Gemini API client.
// TRACK: Gemini uses APISpec builder (telemetry + resiliency).
func NewGeminiClient(ctx context.Context, config *Config) *GeminiClient {
	if config == nil {
		config = DefaultConfig(ctx)
	}
	if config.Provider == "" {
		config.Provider = "gemini"
	}
	secCtx := zqkctx.GetSecurityContext(ctx)
	if key := secCtx.GetLLMAPIKey("gemini"); key != "" {
		config.APIKey = key
	}
	ApplyProviderAdapters(config)
	return &GeminiClient{
		config:     config,
		httpClient: newLLMAPIClient("Gemini LLM", config.BaseURL, config.Timeout),
	}
}

// shouldMock returns true if we should fall back to mock data during local development
// or unit testing when no API key is set.
func (c *GeminiClient) shouldMock() bool {
	// If APIKey is configured, never mock.
	if c.config.APIKey != "" {
		return false
	}
	if c.config.BaseURL == "" || IsPublicCloudBaseURL(c.config.BaseURL) {
		return true
	}
	return false
}

type geminiGenerateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (c *GeminiClient) executeJSONRequest(ctx context.Context, endpoint string, payload any, responseTarget any) error {
	url := fmt.Sprintf("%s%s", strings.TrimSuffix(c.config.BaseURL, "/"), endpoint)
	var headers map[string]string
	if c.config.APIKey != "" {
		headers = map[string]string{"x-goog-api-key": c.config.APIKey}
	}
	return executeJSONHTTPRequest(ctx, c.httpClient, http.MethodPost, url, headers, payload, responseTarget)
}

func (c *GeminiClient) doGenerateContent(ctx context.Context, payload any) (string, error) {
	endpoint := fmt.Sprintf("/models/%s:generateContent", c.config.ChatModel)
	var result geminiGenerateContentResponse
	if err := c.executeJSONRequest(ctx, endpoint, payload, &result); err != nil {
		return "", err
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", errfmt.Errorf("no response candidates returned")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

func (c *GeminiClient) GenerateIntent(ctx context.Context, code string) (string, error) {
	if c.shouldMock() {
		return fmt.Sprintf("Semantic intent for: %s", truncate(code, 50)), nil
	}

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

	return c.doGenerateContent(ctx, payload)
}

func (c *GeminiClient) GenerateCompletion(ctx context.Context, prompt string, system string) (string, error) {
	sanitizedPrompt, isMock, err := SanitizeOrMockPrompt(c.shouldMock(), prompt)
	if isMock || err != nil {
		return sanitizedPrompt, err
	}

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

	return c.doGenerateContent(ctx, payload)
}

func (c *GeminiClient) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if c.shouldMock() {
		emb := make([]float32, 768) // Gemini embeddings are typically 768 dimensions
		emb[0] = 1.0
		return emb, nil
	}

	endpoint := fmt.Sprintf("/models/%s:embedContent", c.config.EmbedModel)
	payload := map[string]any{
		"model": "models/" + c.config.EmbedModel,
		objects.FieldKeyContent: map[string]any{
			"parts": []map[string]any{
				{"text": text},
			},
		},
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := c.executeJSONRequest(ctx, endpoint, payload, &result); err != nil {
		return nil, err
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

	text, err := c.doGenerateContent(ctx, payload)
	if err != nil {
		return AnalysisResult{}, err
	}

	var analysis AnalysisResult
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
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
	return CompareEmbeddings(ctx, c.GenerateEmbedding, observedDescription, expectedNarrative)
}
