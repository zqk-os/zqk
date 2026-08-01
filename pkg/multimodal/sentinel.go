package multimodal

import (
	"context"
)

// VerificationResult holds the outcome of a multimodal truth check.
type VerificationResult struct {
	IsValid bool
	Reason  string
}

// LLMClient represents an abstract interface for multimodal LLMs.
type LLMClient interface {
	AnalyzeVideo(ctx context.Context, script string, videoData []byte) (bool, string, error)
	AnalyzeAudio(ctx context.Context, script string, audioData []byte) (bool, string, error)
}

// DefaultLLMClient is a stub client for the initial hook setup.
type DefaultLLMClient struct{}

// AnalyzeVideo analyzes a video against a script.
func (c *DefaultLLMClient) AnalyzeVideo(ctx context.Context, script string, videoData []byte) (bool, string, error) {
	if len(videoData) > 0 && script != "" {
		return true, "stub validation passed via multimodal hook", nil
	}
	return false, "missing input data", nil
}

// AnalyzeAudio analyzes audio against a script.
func (c *DefaultLLMClient) AnalyzeAudio(ctx context.Context, script string, audioData []byte) (bool, string, error) {
	if len(audioData) > 0 && script != "" {
		return true, "stub audio validation passed", nil
	}
	return false, "missing audio data", nil
}

// VideoSentinel handles video-specific interception.
type VideoSentinel struct {
	client LLMClient
}

// InterceptVideoGeneration is the core hook invoked during video render cycles.
func (s *VideoSentinel) InterceptVideoGeneration(ctx context.Context, jobID string, script string, frameData []byte) error {
	isValid, _, err := s.client.AnalyzeVideo(ctx, script, frameData)
	if err != nil {
		return err
	}
	if !isValid {
		return context.Canceled
	}
	return nil
}

// AudioSentinel handles audio-specific interception.
type AudioSentinel struct {
	client LLMClient
}

// InterceptAudioGeneration intercepts audio render cycles to verify logic/truthfulness against the script.
func (s *AudioSentinel) InterceptAudioGeneration(ctx context.Context, jobID string, script string, audioData []byte) error {
	isValid, _, err := s.client.AnalyzeAudio(ctx, script, audioData)
	if err != nil {
		return err
	}
	if !isValid {
		return context.Canceled
	}
	return nil
}

// SentinelManager consolidates the functionality of all sentinels.
type SentinelManager struct {
	Video *VideoSentinel
	Audio *AudioSentinel
}

// NewSentinelManager initializes a new multimodal sentinel manager.
func NewSentinelManager(client LLMClient) *SentinelManager {
	if client == nil {
		client = &DefaultLLMClient{}
	}
	return &SentinelManager{
		Video: &VideoSentinel{client: client},
		Audio: &AudioSentinel{client: client},
	}
}

// HookSetup provides interception configuration for generation events.
type HookSetup struct {
	Manager *SentinelManager
}
