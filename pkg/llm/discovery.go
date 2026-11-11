package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Standard probe targets for local LLM engines.
var defaultProbeTargets = []struct {
	EngineType ProviderEngineType
	Port       int
	ProbePath  string
	Format     string // "ollama" or "openai"
}{
	{EngineOllama, 11434, "/api/tags", "ollama"},
	{EngineVLLM, 8000, "/v1/models", "openai"},
	{EngineLlamaServer, 8080, "/v1/models", "openai"},
	{EngineLlamaServer, 8443, "/v1/models", "openai"},
}

// LocalEngineHarvester performs discovery and interrogation of local inference runtimes.
type LocalEngineHarvester struct {
	Client  *http.Client
	Timeout time.Duration
}

// NewLocalEngineHarvester creates a new harvester with safe default timeouts.
func NewLocalEngineHarvester(timeout time.Duration) *LocalEngineHarvester {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return &LocalEngineHarvester{
		Client: &http.Client{
			Timeout: timeout,
		},
		Timeout: timeout,
	}
}

// Discover probes standard loopback ports and returns active EndpointProfiles.
func (h *LocalEngineHarvester) Discover(ctx context.Context) ([]EndpointProfile, error) {
	var discovered []EndpointProfile

	for _, target := range defaultProbeTargets {
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", target.Port)
		url := baseURL + target.ProbePath

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}

		start := time.Now()
		resp, err := h.Client.Do(req)
		if err != nil {
			continue
		}
		latency := time.Since(start).Milliseconds()

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		models := parseModels(bodyBytes, target.Format)
		defaultModel := ""
		if len(models) > 0 {
			defaultModel = models[0]
		}

		profile := EndpointProfile{
			EngineType:       target.EngineType,
			BaseURL:          baseURL,
			DefaultModel:     defaultModel,
			DiscoveredModels: models,
			MaxContextTokens: 4096,
			SupportsTools:    true,
			IsHealthy:        true,
			LatencyMs:        latency,
		}

		if err := profile.Validate(); err == nil {
			discovered = append(discovered, profile)
		}
	}

	return discovered, nil
}

// BuildFallbackChain constructs an ordered fallback sequence of profiles.
func BuildFallbackChain(profiles []EndpointProfile) ([]EndpointProfile, error) {
	if len(profiles) == 0 {
		return nil, errors.New("cannot build fallback chain: no profiles provided")
	}

	var healthy []EndpointProfile
	for _, p := range profiles {
		if p.IsHealthy {
			healthy = append(healthy, p)
		}
	}

	if len(healthy) == 0 {
		return nil, errors.New("cannot build fallback chain: zero healthy profiles available")
	}

	return healthy, nil
}

func parseModels(payload []byte, format string) []string {
	var out []string
	switch format {
	case "ollama":
		var ollamaResp struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(payload, &ollamaResp); err == nil {
			for _, m := range ollamaResp.Models {
				if strings.TrimSpace(m.Name) != "" {
					out = append(out, strings.TrimSpace(m.Name))
				}
			}
		}
	case "openai":
		var openaiResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(payload, &openaiResp); err == nil {
			for _, d := range openaiResp.Data {
				if strings.TrimSpace(d.ID) != "" {
					out = append(out, strings.TrimSpace(d.ID))
				}
			}
		}
	}
	return out
}
