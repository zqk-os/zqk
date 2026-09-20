package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WorkflowDispatchConfig configures the GitHub Actions workflow dispatcher.
type WorkflowDispatchConfig struct {
	Owner      string
	Repo       string
	WorkflowID string
	Ref        string
	Token      string
	BaseURL    string
	HTTPClient *http.Client
}

// WorkflowDispatchPayload is the JSON payload sent to the GitHub Actions dispatch API.
type WorkflowDispatchPayload struct {
	Ref    string                 `json:"ref"`
	Inputs map[string]interface{} `json:"inputs,omitempty"`
}

// DispatchResponse captures the outcome of a workflow dispatch request.
type DispatchResponse struct {
	StatusCode int    `json:"status_code"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
}

// ReleaseInfo contains release metadata from a GitHub webhook event.
type ReleaseInfo struct {
	TagName         string `json:"tag_name"`
	ReleaseName     string `json:"release_name"`
	TargetCommitish string `json:"target_commitish"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
}

// ReleaseEvent represents a GitHub release webhook event payload.
type ReleaseEvent struct {
	Action  string      `json:"action"`
	Release ReleaseInfo `json:"release"`
}

// WorkflowDispatcher executes GitHub Actions workflow dispatches and handles release triggers.
type WorkflowDispatcher struct {
	cfg WorkflowDispatchConfig
}

// NewWorkflowDispatcher constructs a validated WorkflowDispatcher.
func NewWorkflowDispatcher(cfg WorkflowDispatchConfig) (*WorkflowDispatcher, error) {
	if strings.TrimSpace(cfg.Owner) == "" {
		return nil, fmt.Errorf("repository owner is required")
	}
	if strings.TrimSpace(cfg.Repo) == "" {
		return nil, fmt.Errorf("repository name is required")
	}
	if strings.TrimSpace(cfg.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("authentication token is required")
	}
	if strings.TrimSpace(cfg.Ref) == "" {
		cfg.Ref = "main"
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://api.github.com"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	return &WorkflowDispatcher{cfg: cfg}, nil
}

// Dispatch triggers a workflow_dispatch event for the configured repository and workflow.
func (d *WorkflowDispatcher) Dispatch(ctx context.Context, inputs map[string]interface{}) (*DispatchResponse, error) {
	if err := ValidateDispatchInputs(inputs); err != nil {
		return nil, fmt.Errorf("invalid dispatch inputs: %w", err)
	}

	payload := WorkflowDispatchPayload{
		Ref:    d.cfg.Ref,
		Inputs: inputs,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal dispatch payload: %w", err)
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches",
		d.cfg.BaseURL, d.cfg.Owner, d.cfg.Repo, d.cfg.WorkflowID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed creating dispatch HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+d.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dispatch HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	switch resp.StatusCode {
	case http.StatusNoContent: // Standard successful GitHub dispatch status
		return &DispatchResponse{
			StatusCode: resp.StatusCode,
			Success:    true,
			Message:    "workflow dispatch triggered successfully",
		}, nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		return &DispatchResponse{
			StatusCode: resp.StatusCode,
			Success:    false,
			Message:    fmt.Sprintf("rate limit or permission denied: %s", string(respBody)),
		}, fmt.Errorf("rate limited or forbidden (status %d): %s", resp.StatusCode, string(respBody))
	default:
		return &DispatchResponse{
			StatusCode: resp.StatusCode,
			Success:    false,
			Message:    fmt.Sprintf("unexpected API response (%d): %s", resp.StatusCode, string(respBody)),
		}, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(respBody))
	}
}

// HandleWebhookRelease processes an incoming GitHub release webhook and dispatches a build workflow if published.
func (d *WorkflowDispatcher) HandleWebhookRelease(ctx context.Context, payload []byte) (*DispatchResponse, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty webhook payload")
	}

	var event ReleaseEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("invalid release webhook JSON: %w", err)
	}

	if event.Action != "published" {
		return &DispatchResponse{
			StatusCode: http.StatusOK,
			Success:    false,
			Message:    fmt.Sprintf("ignoring action %q; only 'published' triggers release dispatch", event.Action),
		}, nil
	}

	tagName := strings.TrimSpace(event.Release.TagName)
	if tagName == "" {
		return nil, fmt.Errorf("release event missing tag_name")
	}

	inputs := map[string]interface{}{
		"release_tag": tagName,
		"prerelease":  event.Release.Prerelease,
		"draft":       event.Release.Draft,
	}

	// Override dispatch ref with release tag if provided
	originalRef := d.cfg.Ref
	d.cfg.Ref = tagName
	defer func() { d.cfg.Ref = originalRef }()

	return d.Dispatch(ctx, inputs)
}

// ValidateDispatchInputs ensures input keys and values conform to GitHub Actions dispatch constraints.
func ValidateDispatchInputs(inputs map[string]interface{}) error {
	if inputs == nil {
		return nil
	}
	if len(inputs) > 50 {
		return fmt.Errorf("maximum of 50 inputs allowed, received %d", len(inputs))
	}

	for k, v := range inputs {
		trimmedKey := strings.TrimSpace(k)
		if trimmedKey == "" {
			return fmt.Errorf("input key cannot be empty")
		}
		if len(trimmedKey) > 100 {
			return fmt.Errorf("input key %q exceeds 100 characters", trimmedKey)
		}
		switch val := v.(type) {
		case string:
			if len(val) > 2000 {
				return fmt.Errorf("input %q value exceeds 2000 characters", trimmedKey)
			}
		case bool, int, int64, float64:
			// Allowed primitive types
		default:
			return fmt.Errorf("input %q has unsupported type %T; only strings, booleans, and numbers allowed", trimmedKey, v)
		}
	}
	return nil
}
