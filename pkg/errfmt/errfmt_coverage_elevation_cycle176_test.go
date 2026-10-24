// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package errfmt

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type customTestError struct {
	code    int
	message string
}

func (e *customTestError) Error() string {
	return fmt.Sprintf("[%d] %s", e.code, e.message)
}

func TestErrorf_StandardAndWrapping(t *testing.T) {
	t.Parallel()

	t.Run("standard formatting without wrap", func(t *testing.T) {
		err := Errorf("failed to process item %d with name %s", 42, "item-alpha")
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		expected := "failed to process item 42 with name item-alpha"
		if err.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("wrap verb with errors.Is and errors.As", func(t *testing.T) {
		root := &customTestError{code: 404, message: "resource missing"}
		wrapped := Errorf("request failed: %w", root)
		if wrapped == nil {
			t.Fatal("expected non-nil error")
		}
		if !errors.Is(wrapped, root) {
			t.Fatalf("expected errors.Is(wrapped, root) to be true")
		}

		var target *customTestError
		if !errors.As(wrapped, &target) {
			t.Fatalf("expected errors.As(wrapped, &target) to be true")
		}
		if target.code != 404 {
			t.Fatalf("expected target code 404, got %d", target.code)
		}
	})
}

func TestErrorf_ActionableProjectRoot(t *testing.T) {
	t.Parallel()

	t.Run("zero arguments matches sentinel", func(t *testing.T) {
		err := Errorf("project root not found")
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		if err.Error() != ActionableProjectRootNotFound {
			t.Fatalf("expected %q, got %q", ActionableProjectRootNotFound, err.Error())
		}
	})

	t.Run("with arguments preserves format", func(t *testing.T) {
		err := Errorf("project root not found in path: %s", "/var/tmp")
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		expected := "project root not found in path: /var/tmp"
		if err.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, err.Error())
		}
	})
}

func TestErrorf_PoisonPillDefense(t *testing.T) {
	t.Parallel()

	electrocution := errors.New("⚡️ BZZZT! fatal operator halt")
	normalErr := errors.New("underlying socket closed")

	t.Run("first argument is electrocution error", func(t *testing.T) {
		err := Errorf("operation context: %w", electrocution)
		if err != electrocution {
			t.Fatalf("expected poison pill error returned unmodified, got %v", err)
		}
	})

	t.Run("secondary argument is electrocution error", func(t *testing.T) {
		err := Errorf("multi error: %v, %v", normalErr, electrocution)
		if err != electrocution {
			t.Fatalf("expected poison pill error returned unmodified, got %v", err)
		}
	})

	t.Run("string containing poison pill does not trigger error short circuit", func(t *testing.T) {
		rawString := "⚡️ BZZZT! text only"
		err := Errorf("informational warning: %s", rawString)
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		expected := "informational warning: ⚡️ BZZZT! text only"
		if err.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, err.Error())
		}
	})
}

func TestNewfAndNewErrorStringBuilder(t *testing.T) {
	t.Parallel()

	t.Run("Newf with formatting", func(t *testing.T) {
		b := Newf("step %d: %s", 1, "initializing")
		if b.Build() != "step 1: initializing" {
			t.Fatalf("unexpected build output: %s", b.Build())
		}
	})

	t.Run("Newf with actionable project root sentinel", func(t *testing.T) {
		b := Newf("project root not found")
		if b.Build() != ActionableProjectRootNotFound {
			t.Fatalf("expected %q, got %q", ActionableProjectRootNotFound, b.Build())
		}
	})

	t.Run("Newf with project root and args", func(t *testing.T) {
		b := Newf("project root not found: %s", "checked /etc")
		expected := "project root not found: checked /etc"
		if b.Build() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Build())
		}
	})

	t.Run("NewErrorStringBuilder alias equivalence", func(t *testing.T) {
		b := NewErrorStringBuilder("component %s ready", "orchestrator")
		expected := "component orchestrator ready"
		if b.Build() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Build())
		}
	})
}

func TestErrorStringBuilder_WithAndChaining(t *testing.T) {
	t.Parallel()

	t.Run("standard format templates", func(t *testing.T) {
		b := Newf("base context").
			With("sub-action %s", "validation").
			With("attempt %d", 2)
		expected := "base context: sub-action validation: attempt 2"
		if b.Build() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Build())
		}
	})

	t.Run("percent w handling inside with template", func(t *testing.T) {
		root := errors.New("connection reset by peer")
		wrapFmt := "socket error: " + "%w"
		b := Newf("network layer").With(wrapFmt, root)
		expected := "network layer: socket error: connection reset by peer"
		if b.Build() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Build())
		}
	})
}

func TestErrorStringBuilder_AndOrDefault(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		value        string
		defaultValue string
		expectedPart string
	}{
		{
			name:         "empty string falls back to default",
			value:        "",
			defaultValue: "fallback-default",
			expectedPart: "fallback-default",
		},
		{
			name:         "whitespace string falls back to default",
			value:        "   \t\r\n  ",
			defaultValue: "fallback-whitespace",
			expectedPart: "fallback-whitespace",
		},
		{
			name:         "non-empty trimmed string is preserved",
			value:        "explicit-override",
			defaultValue: "fallback-ignored",
			expectedPart: "explicit-override",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := Newf("prefix").AndOrDefault(tc.value, tc.defaultValue)
			expected := "prefix: " + tc.expectedPart
			if b.Build() != expected {
				t.Fatalf("expected %q, got %q", expected, b.Build())
			}
		})
	}
}

func TestErrorStringBuilder_AndIfTrue(t *testing.T) {
	t.Parallel()

	t.Run("condition true appends segment", func(t *testing.T) {
		b := Newf("prefix").AndIfTrue(true, "flag enabled: %t", true)
		expected := "prefix: flag enabled: true"
		if b.Build() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Build())
		}
	})

	t.Run("condition false does not append segment", func(t *testing.T) {
		b := Newf("prefix").AndIfTrue(false, "should not exist")
		expected := "prefix"
		if b.Build() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Build())
		}
	})
}

func TestErrorStringBuilder_StringAndError(t *testing.T) {
	t.Parallel()

	t.Run("empty builder struct", func(t *testing.T) {
		empty := &ErrorStringBuilder{}
		if empty.Build() != "" {
			t.Fatalf("expected empty string from Build(), got %q", empty.Build())
		}
		if empty.String() != "" {
			t.Fatalf("expected empty string from String(), got %q", empty.String())
		}
		if empty.Error() != "" {
			t.Fatalf("expected empty string from Error(), got %q", empty.Error())
		}
	})

	t.Run("nil builder pointer", func(t *testing.T) {
		var nilBuilder *ErrorStringBuilder
		if nilBuilder.Build() != "" {
			t.Fatalf("expected empty string from nil Build(), got %q", nilBuilder.Build())
		}
		if nilBuilder.String() != "" {
			t.Fatalf("expected empty string from nil String(), got %q", nilBuilder.String())
		}
		if nilBuilder.Error() != "" {
			t.Fatalf("expected empty string from nil Error(), got %q", nilBuilder.Error())
		}
	})

	t.Run("single part builder", func(t *testing.T) {
		b := Newf("standalone error context")
		expected := "standalone error context"
		if b.String() != expected {
			t.Fatalf("expected %q, got %q", expected, b.String())
		}
		if b.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Error())
		}
		var err error = b
		if err.Error() != expected {
			t.Fatalf("expected error interface %q, got %q", expected, err.Error())
		}
	})

	t.Run("multiple parts builder", func(t *testing.T) {
		b := Newf("service auth").With("token expired").With("realm %s", "core")
		expected := "service auth: token expired: realm core"
		if b.String() != expected {
			t.Fatalf("expected %q, got %q", expected, b.String())
		}
		if b.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, b.Error())
		}
	})
}

type failingFmtState struct{}

func (f *failingFmtState) Write(b []byte) (int, error) {
	return 0, errors.New("write failure")
}

func (f *failingFmtState) Width() (int, bool)     { return 0, false }
func (f *failingFmtState) Precision() (int, bool) { return 0, false }
func (f *failingFmtState) Flag(c int) bool        { return false }

func TestErrorStringBuilder_Format(t *testing.T) {
	t.Parallel()

	t.Run("formatting with verb s and v", func(t *testing.T) {
		b := Newf("pipeline failure").With("stage %d", 3)
		expected := "pipeline failure: stage 3"

		sResult := fmt.Sprintf("%s", b)
		if sResult != expected {
			t.Fatalf("expected %q, got %q", expected, sResult)
		}

		vResult := fmt.Sprintf("%v", b)
		if vResult != expected {
			t.Fatalf("expected %q, got %q", expected, vResult)
		}
	})

	t.Run("formatting with verb q", func(t *testing.T) {
		b := Newf("quoted message")
		expected := `"quoted message"`
		qResult := fmt.Sprintf("%q", b)
		if qResult != expected {
			t.Fatalf("expected %q, got %q", expected, qResult)
		}
	})

	t.Run("formatting with unsupported verb", func(t *testing.T) {
		b := Newf("unsupported verb test")
		res := fmt.Sprintf("%d", b)
		if !strings.Contains(res, "%!d(*ErrorStringBuilder=unsupported verb test)") {
			t.Fatalf("expected custom verb format, got %q", res)
		}
	})

	t.Run("nil builder format", func(t *testing.T) {
		var nilBuilder *ErrorStringBuilder
		res := fmt.Sprintf("%s", nilBuilder)
		if res != "" {
			t.Fatalf("expected empty string for nil builder format, got %q", res)
		}
	})

	t.Run("write error handling on format verbs", func(t *testing.T) {
		b := Newf("test error branches")
		mockState := &failingFmtState{}
		// Verify Format gracefully handles write errors for 's', 'q', and default verbs
		b.Format(mockState, 's')
		b.Format(mockState, 'q')
		b.Format(mockState, 'x')
	})
}

func TestErrorStringBuilder_Wrap(t *testing.T) {
	t.Parallel()

	t.Run("wrap normal error", func(t *testing.T) {
		root := errors.New("disk I/O timeout")
		b := Newf("storage commit")
		err := b.Wrap(root)
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		expected := "storage commit: disk I/O timeout"
		if err.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, err.Error())
		}
		if !errors.Is(err, root) {
			t.Fatalf("expected wrapped error to match root with errors.Is")
		}
	})

	t.Run("wrap poison pill error", func(t *testing.T) {
		poison := errors.New("⚡️ BZZZT! agent electrocuted")
		b := Newf("ignored prefix")
		err := b.Wrap(poison)
		if err != poison {
			t.Fatalf("expected poison pill error returned directly, got %v", err)
		}
	})
}

func runConcurrencyWorker(t *testing.T, wg *sync.WaitGroup, workerID, iterationCount int) {
	defer wg.Done()
	for j := 0; j < iterationCount; j++ {
		b := Newf("worker %d", workerID).
			With("iter %d", j).
			AndOrDefault("", "default").
			AndIfTrue(j%2 == 0, "even")

		expectedPrefix := fmt.Sprintf("worker %d: iter %d: default", workerID, j)
		msg := b.Build()
		if !strings.HasPrefix(msg, expectedPrefix) {
			t.Errorf("worker %d unexpected build prefix: %s", workerID, msg)
			return
		}
		if b.String() != msg {
			t.Errorf("worker %d String() mismatch: %s vs %s", workerID, b.String(), msg)
			return
		}
		if b.Error() != msg {
			t.Errorf("worker %d Error() mismatch: %s vs %s", workerID, b.Error(), msg)
			return
		}
	}
}

func TestErrorStringBuilder_Concurrency(t *testing.T) {
	t.Parallel()

	const workerCount = 50
	const iterationCount = 100

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go runConcurrencyWorker(t, &wg, i, iterationCount)
	}

	wg.Wait()
}
