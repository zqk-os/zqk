package rollback

import (
	"time"

	"github.com/zqk-os/zqk/pkg/config"
)

const (
	emptyValue = ""
	// DefaultRetainCount is the default number of rollback points to retain.
	DefaultRetainCount = 50
	// DefaultRetainDuration is the default max age of retained rollback points.
	DefaultRetainDuration = 24 * time.Hour
)

// Config holds rollback retention and feature settings.
type Config struct {
	RetainCount    int           // Keep last N points (0 = use default)
	RetainDuration time.Duration // Keep points within this duration (0 = use default)
	CaptureEnabled bool          // If false, Capture is a no-op (default true)
}

// DefaultConfig returns config with defaults; overridden by env if set.
func DefaultConfig() Config {
	c := Config{
		RetainCount:    config.StorageRollbackRetainCount().OrDefault(5),
		RetainDuration: DefaultRetainDuration,
		CaptureEnabled: !config.StorageRollbackCaptureDisabled().OrDefault(false),
	}
	v := config.StorageRollbackRetainDuration().OrDefault("168h")
	if v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.RetainDuration = d
		}
	}
	return c
}
