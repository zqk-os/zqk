package filter

import (
	"testing"
	"time"
)

func TestResolveTimeTokens(t *testing.T) {
	// String test
	now := ResolveTimeTokens("now")
	if str, ok := now.(string); !ok || str == "" {
		t.Errorf("expected string for 'now', got %v", now)
	}

	today := ResolveTimeTokens("today")
	if str, ok := today.(string); !ok || str == "" {
		t.Errorf("expected string for 'today', got %v", today)
	}

	yesterday := ResolveTimeTokens("yesterday")
	if str, ok := yesterday.(string); !ok || str == "" {
		t.Errorf("expected string for 'yesterday', got %v", yesterday)
	}

	lastWeek := ResolveTimeTokens("last week")
	if str, ok := lastWeek.(string); !ok || str == "" {
		t.Errorf("expected string for 'last week', got %v", lastWeek)
	}

	last2Hours := ResolveTimeTokens("last 2 hours")
	if str, ok := last2Hours.(string); !ok || str == "" {
		t.Errorf("expected string for 'last 2 hours', got %v", last2Hours)
	}

	nonToken := ResolveTimeTokens("regular-value")
	if nonToken != "regular-value" {
		t.Errorf("expected 'regular-value' unchanged, got %v", nonToken)
	}

	// Map test
	m := map[string]any{
		"time":  "now",
		"other": "hello",
	}
	resolvedMap := ResolveTimeTokens(m).(map[string]any)
	if _, err := time.Parse(time.RFC3339, resolvedMap["time"].(string)); err != nil {
		t.Errorf("expected valid RFC3339 string, got %v", resolvedMap["time"])
	}

	// Slice test
	s := []any{"now", "today"}
	resolvedSlice := ResolveTimeTokens(s).([]any)
	if len(resolvedSlice) != 2 {
		t.Errorf("expected slice length 2, got %d", len(resolvedSlice))
	}

	// Non-string test
	if ResolveTimeTokens(123) != 123 {
		t.Errorf("expected integer unchanged")
	}
}
