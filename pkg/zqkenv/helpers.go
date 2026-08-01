package zqkenv

import (
	"os"
	"strconv"
)

// EnvValue is a wrapper around an environment variable value that provides convenient
// fallback and type conversion methods.
type EnvValue struct {
	Key string
	Val string
}

// Get returns an EnvValue for the given environment variable key.
func Get(key string) EnvValue {
	return EnvValue{
		Key: key,
		Val: os.Getenv(key),
	}
}

// OrDefault returns the environment variable value if it is not empty,
// otherwise it returns the provided default value.
func (e EnvValue) OrDefault(def string) string {
	if e.Val != "" {
		return e.Val
	}
	return def
}

// IntOrDefault returns the environment variable value parsed as an integer
// if it is valid, otherwise it returns the provided default value.
func (e EnvValue) IntOrDefault(def int) int {
	if e.Val != "" {
		if parsed, err := strconv.Atoi(e.Val); err == nil {
			return parsed
		}
	}
	return def
}

// BoolOrDefault returns the environment variable value parsed as a boolean
// if it is valid, otherwise it returns the provided default value.
func (e EnvValue) BoolOrDefault(def bool) bool {
	if e.Val != "" {
		if parsed, err := strconv.ParseBool(e.Val); err == nil {
			return parsed
		}
	}
	return def
}
