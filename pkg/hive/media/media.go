package media

import (
	"context"
	"errors"
)

var (
	ErrGenerationFailed = errors.New("media generation failed")
	ErrInvalidConfig    = errors.New("invalid media generator configuration")
)

// AudioRequest defines the parameters for generating an audio asset.
type AudioRequest struct {
	Prompt string
	Lyrics string
	Style  string
}

// VideoRequest defines the parameters for generating a video asset.
type VideoRequest struct {
	ImageURL    string // Base background image
	AudioURL    string // Generated audio to lip-sync or overlay
	Prompt      string // Optional prompt for video generation
	AspectRatio string // e.g., "16:9", "9:16", "1:1"
	Duration    string // e.g., "5s", "10s" (depending on the model)
}

// MediaGenerator defines the abstraction for external media APIs.
// This interface allows the Symbiotic Mesh to request media generation
// without being tightly coupled to a specific provider (e.g., VEED or ILoveSong).
type MediaGenerator interface {
	// GenerateAudio asynchronously requests audio generation and returns a polling ID.
	GenerateAudio(ctx context.Context, req AudioRequest) (string, error)

	// GenerateVideo asynchronously requests video generation and returns a polling ID.
	GenerateVideo(ctx context.Context, req VideoRequest) (string, error)

	// PollStatus checks the status of a generation job and returns the asset URL when complete.
	// Returns ("", nil) if still processing.
	PollStatus(ctx context.Context, jobID string) (assetURL string, err error)
}
