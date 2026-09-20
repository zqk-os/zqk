package config

import (
	"fmt"
	"os"
	"strings"
)

// Property is a generic wrapper around a configuration value pointer.
type Property[T any] struct {
	Name  string
	Value *T
}

func envKey(name string) string {
	// Convert dot to underscore
	name = strings.ReplaceAll(name, ".", "_")

	// Convert camelCase to snake_case
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' && name[i-1] >= 'a' && name[i-1] <= 'z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}

	return "ZQK_" + strings.ToUpper(result.String())
}

func envKeys(name string) []string {
	primary := envKey(name)
	keys := []string{primary}

	// Also check section-stripped alias, e.g. "System.HostloadDisable" -> "ZQK_HOSTLOAD_DISABLE"
	if idx := strings.Index(name, "."); idx != -1 {
		shortKey := envKey(name[idx+1:])
		if shortKey != primary {
			keys = append(keys, shortKey)
		}
	}
	return keys
}

// OrDefault returns the environment variable if present, then the configuration value if present, otherwise returns the default.
func (p Property[T]) OrDefault(def T) T {
	// 1. Environment variables take highest precedence (canonical first, then section-stripped alias)
	for _, k := range envKeys(p.Name) {
		if val := os.Getenv(k); val != "" {
			switch any(def).(type) {
			case string:
				return any(val).(T)
			case bool:
				if val == "true" || val == "1" {
					return any(true).(T)
				}
				return any(false).(T)
			case int:
				var i int
				_, _ = fmt.Sscanf(val, "%d", &i)
				return any(i).(T)
			case float64:
				var f float64
				_, _ = fmt.Sscanf(val, "%f", &f)
				return any(f).(T)
			}
		}
	}

	// 2. Config file value takes precedence over default
	if p.Value != nil {
		return *p.Value
	}

	// 3. Fallback to default
	// Silence the warning output completely to avoid spamming the console during CLI execution.
	return def
}

// Required returns the configuration value or panics if it is missing.
func (p Property[T]) Required() T {
	if val := os.Getenv(envKey(p.Name)); val != "" {
		var zero T
		switch any(zero).(type) {
		case string:
			return any(val).(T)
		}
	}
	if p.Value == nil {
		panic(fmt.Sprintf("Missing required configuration property: %s", p.Name))
	}
	return *p.Value
}

// Safe returns the value or its zero value if missing.
func (p Property[T]) Safe() T {
	var zero T
	return p.OrDefault(zero)
}
