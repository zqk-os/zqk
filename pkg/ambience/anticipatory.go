package ambience

import "context"

// Intent represents a predicted user intent based on ambient events.
type Intent struct {
	Action     string
	Target     string
	Confidence float64
}

// AnticipatoryEngine defines the interface for components that predict user intent.
type AnticipatoryEngine interface {
	Start(ctx context.Context) error
	Stop() error
	PredictIntent(event AmbientEvent) (Intent, error)
}
