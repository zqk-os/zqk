package audit

import (
	"testing"
	"time"
)

func TestDefaultRules(t *testing.T) {
	t.Parallel()
	rules := DefaultRules()
	if len(rules) != 3 {
		t.Fatalf("len=%d", len(rules))
	}
	if rules[0].Window != time.Hour || rules[0].Threshold != 10 {
		t.Fatalf("first rule: %+v", rules[0])
	}
	if rules[2].Window != 5*time.Minute {
		t.Fatalf("scheduler window: %s", rules[2].Window)
	}
}
