package errfmt

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorStringBuilder_EdgeCases(t *testing.T) {
	t.Parallel()

	// 1. NewErrorStringBuilder alias
	b := NewErrorStringBuilder("initial %s", "step")
	if b.Build() != "initial step" {
		t.Errorf("expected initial step, got %s", b.Build())
	}

	// 2. With containing %w
	innerErr := errors.New("underlying issue")
	wFmt := "wrapped: " + "%w"
	b.With(wFmt, innerErr)
	if !strings.Contains(b.Build(), "wrapped: underlying issue") {
		t.Errorf("expected wrapped formatted in With, got %s", b.Build())
	}

	// 3. AndOrDefault with non-empty value
	b2 := Newf("base").AndOrDefault("explicit-value", "fallback")
	if b2.Build() != "base: explicit-value" {
		t.Errorf("expected explicit-value, got %s", b2.Build())
	}

	// 4. AndIfTrue false
	b3 := Newf("base").AndIfTrue(false, "should not appear")
	if b3.Build() != "base" {
		t.Errorf("expected base, got %s", b3.Build())
	}

	// 5. AndIfTrue true with %w
	condFmt := "cond wrapped " + "%w"
	b4 := Newf("base").AndIfTrue(true, condFmt, innerErr)
	if !strings.Contains(b4.Build(), "cond wrapped underlying issue") {
		t.Errorf("expected cond wrapped, got %s", b4.Build())
	}

	// 6. Electrocution poison pill in Errorf
	bzztErr := errors.New("⚡️ BZZZT! forbidden modification")
	res := Errorf("prefix: %w", bzztErr)
	if res != bzztErr {
		t.Errorf("expected poison pill error to return unwrapped, got %v", res)
	}

	// 7. Electrocution poison pill in Wrap
	resWrap := Newf("some prefix").Wrap(bzztErr)
	if resWrap != bzztErr {
		t.Errorf("expected poison pill error in Wrap to return unwrapped, got %v", resWrap)
	}
}
