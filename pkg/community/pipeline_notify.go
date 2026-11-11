package community

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Pipeline notification constants.
const (
	DefaultNotificationTimeout = 5 * time.Second
	DefaultNotificationRetries = 3
	DefaultNotificationBackoff = 50 * time.Millisecond

	HeaderPipelineSignature = "X-ZQK-Signature-256"
	HeaderPipelineTimestamp = "X-ZQK-Timestamp"
	HeaderPipelineStage     = "X-ZQK-Stage"
	HeaderPipelineStatus    = "X-ZQK-Status"
	HeaderPipelineExitCode  = "X-ZQK-ExitCode"

	StageStatusSuccess = "success"
	StageStatusFailed  = "failed"
	StageStatusRunning = "running"
	StageStatusBailed  = "bailed"
)

// Pipeline notification error invariants.
var (
	ErrEmptyWebhookURL  = errors.New("pipeline notification: webhook url is required")
	ErrEmptyHMACSecret  = errors.New("pipeline notification: hmac secret is required")
	ErrInvalidTimeout   = errors.New("pipeline notification: timeout must be positive")
	ErrEmptySHA         = errors.New("pipeline notification: commit sha is required")
	ErrEmptyStage       = errors.New("pipeline notification: stage name is required")
	ErrEmptyStatus      = errors.New("pipeline notification: stage status is required")
	ErrPipelineAborted  = errors.New("pipeline notification: execution aborted by fail-fast governor")
	ErrHMACVerification = errors.New("pipeline notification: invalid hmac signature")
)

// PipelineNotificationConfig configures webhook dispatch for pipeline stage events.
type PipelineNotificationConfig struct {
	WebhookURL   string        `json:"webhook_url" yaml:"webhook_url"`
	HMACSecret   string        `json:"hmac_secret" yaml:"hmac_secret"`
	Timeout      time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries   int           `json:"max_retries" yaml:"max_retries"`
	RetryBackoff time.Duration `json:"retry_backoff" yaml:"retry_backoff"`
}

// Validate checks the invariants of PipelineNotificationConfig.
func (c *PipelineNotificationConfig) Validate() error {
	if strings.TrimSpace(c.WebhookURL) == "" {
		return ErrEmptyWebhookURL
	}
	if strings.TrimSpace(c.HMACSecret) == "" {
		return ErrEmptyHMACSecret
	}
	if c.Timeout < 0 {
		return ErrInvalidTimeout
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultNotificationTimeout
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultNotificationRetries
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = DefaultNotificationBackoff
	}
	return nil
}

// PipelineStatusPayload represents a structured event emitted at pipeline stage transitions.
type PipelineStatusPayload struct {
	SHA            string            `json:"sha" yaml:"sha"`
	Stage          string            `json:"stage" yaml:"stage"`
	Status         string            `json:"status" yaml:"status"`
	ExitCode       int               `json:"exit_code" yaml:"exit_code"`
	DurationMS     int64             `json:"duration_ms" yaml:"duration_ms"`
	FailureSummary string            `json:"failure_summary,omitempty" yaml:"failure_summary,omitempty"`
	Timestamp      time.Time         `json:"timestamp" yaml:"timestamp"`
	Metadata       map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Validate checks the invariants of PipelineStatusPayload.
func (p *PipelineStatusPayload) Validate() error {
	if strings.TrimSpace(p.SHA) == "" {
		return ErrEmptySHA
	}
	if strings.TrimSpace(p.Stage) == "" {
		return ErrEmptyStage
	}
	if strings.TrimSpace(p.Status) == "" {
		return ErrEmptyStatus
	}
	if p.Timestamp.IsZero() {
		p.Timestamp = time.Now().UTC()
	}
	return nil
}

// ToJSON serializes the status payload to JSON bytes.
func (p *PipelineStatusPayload) ToJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// ComputePipelineHMAC generates a hex-encoded SHA-256 HMAC signature.
func ComputePipelineHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyPipelineHMAC verifies that a provided signature matches the HMAC of payload.
func VerifyPipelineHMAC(secret string, payload []byte, signature string) bool {
	expected := ComputePipelineHMAC(secret, payload)
	return hmac.Equal([]byte(strings.TrimSpace(signature)), []byte(expected))
}

// PipelineNotificationDispatcher handles authenticated HTTP delivery of status payloads.
type PipelineNotificationDispatcher struct {
	cfg        PipelineNotificationConfig
	httpClient *http.Client
	wg         sync.WaitGroup
}

// NewPipelineNotificationDispatcher instantiates a validated dispatcher.
func NewPipelineNotificationDispatcher(cfg PipelineNotificationConfig, client *http.Client) (*PipelineNotificationDispatcher, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &PipelineNotificationDispatcher{
		cfg:        cfg,
		httpClient: client,
	}, nil
}

// Dispatch sends a PipelineStatusPayload synchronously with retries and exponential backoff.
func (d *PipelineNotificationDispatcher) Dispatch(ctx context.Context, payload PipelineStatusPayload) error {
	data, err := payload.ToJSON()
	if err != nil {
		return fmt.Errorf("serialize payload: %w", err)
	}

	sig := ComputePipelineHMAC(d.cfg.HMACSecret, data)
	backoff := d.cfg.RetryBackoff

	var lastErr error
	for attempt := 0; attempt <= d.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.WebhookURL, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(HeaderPipelineSignature, sig)
		req.Header.Set(HeaderPipelineTimestamp, strconv.FormatInt(payload.Timestamp.Unix(), 10))
		req.Header.Set(HeaderPipelineStage, payload.Stage)
		req.Header.Set(HeaderPipelineStatus, payload.Status)
		req.Header.Set(HeaderPipelineExitCode, strconv.Itoa(payload.ExitCode))

		resp, err := d.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Read and discard body to allow connection reuse.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("webhook responded with HTTP %d", resp.StatusCode)
	}

	return fmt.Errorf("dispatch failed after %d retries: %w", d.cfg.MaxRetries, lastErr)
}

// DispatchAsync queues non-blocking dispatch of a status payload in a separate goroutine.
func (d *PipelineNotificationDispatcher) DispatchAsync(ctx context.Context, payload PipelineStatusPayload) {
	d.wg.Add(1)
	goroutinelabels.NewGoroutine("pipeline_notification_dispatch", "dispatch webhook event async").
		StartSimple(func() {
			defer d.wg.Done()
			_ = d.Dispatch(ctx, payload)
		})
}

// Drain blocks until all in-flight asynchronous dispatches complete.
func (d *PipelineNotificationDispatcher) Drain() {
	d.wg.Wait()
}

// PipelineFailFastGovernor halts pipeline execution when a blocking stage fails.
type PipelineFailFastGovernor struct {
	mu             sync.RWMutex
	blockingStages map[string]bool
	aborted        bool
	abortReason    string
	failedStage    string
}

// NewPipelineFailFastGovernor creates a governor with initial blocking stage names.
func NewPipelineFailFastGovernor(blockingStages []string) *PipelineFailFastGovernor {
	stages := make(map[string]bool, len(blockingStages))
	for _, s := range blockingStages {
		stages[strings.TrimSpace(s)] = true
	}
	return &PipelineFailFastGovernor{
		blockingStages: stages,
	}
}

// RegisterBlockingStage marks a stage name as blocking.
func (g *PipelineFailFastGovernor) RegisterBlockingStage(stage string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockingStages[strings.TrimSpace(stage)] = true
}

// RecordStageResult evaluates a completed stage and trips the governor if a blocking stage failed.
func (g *PipelineFailFastGovernor) RecordStageResult(payload PipelineStatusPayload) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.aborted {
		return fmt.Errorf("%w: %s", ErrPipelineAborted, g.abortReason)
	}

	stage := strings.TrimSpace(payload.Stage)
	isBlocking := g.blockingStages[stage]

	if isBlocking && (payload.ExitCode != 0 || payload.Status == StageStatusFailed) {
		g.aborted = true
		g.failedStage = stage
		g.abortReason = fmt.Sprintf("blocking stage %q failed with exit code %d (summary: %s)",
			stage, payload.ExitCode, payload.FailureSummary)
		return fmt.Errorf("%w: %s", ErrPipelineAborted, g.abortReason)
	}

	return nil
}

// ShouldAbort returns true if a blocking failure has tripped the governor.
func (g *PipelineFailFastGovernor) ShouldAbort() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.aborted
}

// AbortReason returns the human-readable explanation of why execution was halted.
func (g *PipelineFailFastGovernor) AbortReason() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.abortReason
}

// FailedStage returns the name of the stage that triggered early termination.
func (g *PipelineFailFastGovernor) FailedStage() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.failedStage
}

// Reset clears the governor state for a new pipeline run.
func (g *PipelineFailFastGovernor) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.aborted = false
	g.abortReason = ""
	g.failedStage = ""
}
