package when_test

import (
	"errors"
	"testing"

	"github.com/lanceman/zqk/pkg/when"
)

func TestTry_UnlessErrPreservesError(t *testing.T) {
	t.Parallel()
	base := errors.New("boom")
	v, err := when.Try("x", base).
		UnlessErr(func(e error) error { return errors.New("wrapped: " + e.Error()) }).
		OrElse(func() string { return "fallback" }).
		Get()
	if err == nil || err.Error() != "wrapped: boom" {
		t.Fatalf("err=%v", err)
	}
	if v != "fallback" {
		t.Fatalf("val=%q", v)
	}
}

func TestTry_ThenOnSuccess(t *testing.T) {
	t.Parallel()
	v, err := when.Try(2, nil).Then(func(n int) int { return n * 3 }).Get()
	if err != nil || v != 6 {
		t.Fatalf("got %d %v", v, err)
	}
}

func TestIfTrue_ThenUnlessErrOrElse(t *testing.T) {
	t.Parallel()
	err := when.IfTrue(true).
		Then(func() error { return errors.New("fail") }).
		UnlessErr(func(e error) error { return errors.New("u:" + e.Error()) }).
		OrElse(func() error { return errors.New("should-not-run") }).
		Err()
	if err == nil || err.Error() != "u:fail" {
		t.Fatalf("err=%v", err)
	}

	err = when.IfTrue(false).
		Then(func() error { return errors.New("then") }).
		OrElse(func() error { return errors.New("else") }).
		Err()
	if err == nil || err.Error() != "else" {
		t.Fatalf("err=%v", err)
	}
}
