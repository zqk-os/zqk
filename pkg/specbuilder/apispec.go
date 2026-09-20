package specbuilder

import (
	"net/http"
	"time"
)

// RetryPolicy defines the backoff and retry strategy.
type RetryPolicy struct {
	MaxRetries int
	Backoff    time.Duration
}

// AuthStrategy defines how authentication is applied to a request.
type AuthStrategy interface {
	Apply(req *http.Request) error
}

// APISpec defines the configuration and requirements for an API client.
type APISpec struct {
	Name           string
	BaseURL        string
	Timeout        time.Duration
	RetryPolicy    RetryPolicy
	AuthStrategies []AuthStrategy
}

// APIClient is the resulting client that executes HTTP requests.
type APIClient interface {
	Do(req *http.Request) (*http.Response, error)
}
