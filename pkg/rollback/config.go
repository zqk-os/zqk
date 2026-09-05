package rollback

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

const (
	emptyValue       = ""
	envTruthyNumeric = "1"
	envTruthyLiteral = "true"
	defaultCaptureOn = true

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
		RetainCount:    DefaultRetainCount,
		RetainDuration: DefaultRetainDuration,
		CaptureEnabled: defaultCaptureOn,
	}
	if n := os.Getenv(zqkenv.RollbackRetainCount()); n != emptyValue {
		if v, err := strconv.Atoi(n); err == nil && v >= 0 {
			c.RetainCount = v
		}
	}
	if d := os.Getenv(zqkenv.RollbackRetainDuration()); d != emptyValue {
		if v, err := time.ParseDuration(d); err == nil && v >= 0 {
			c.RetainDuration = v
		}
	}
	if isTruthyEnv(os.Getenv(zqkenv.RollbackCaptureDisabled())) {
		c.CaptureEnabled = false
	}
	return c
}

func isTruthyEnv(value string) bool {
	return value == envTruthyNumeric || strings.EqualFold(value, envTruthyLiteral)
}
