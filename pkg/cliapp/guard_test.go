package cli

import (
	"errors"
	"strings"
	"testing"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"
)

func TestGuard_Return_nilError(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	err := Guard(cmd).Return()
	if err != nil {
		t.Errorf("Guard().Return() with no error: got %v", err)
	}
	err = Guard(cmd).Err(nil).Return()
	if err != nil {
		t.Errorf("Guard().Err(nil).Return(): got %v", err)
	}
}

func TestGuard_Return_withError(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	e := errors.New("test error")
	err := Guard(cmd).Err(e).Return()
	if err == nil {
		t.Fatal("Guard().Err(e).Return(): expected non-nil")
	}
	if !strings.Contains(err.Error(), "test error") {
		t.Errorf("error should contain original message: %q", err.Error())
	}
}

func TestGuard_Require(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	err := Guard(cmd).Require(true, "required").Return()
	if err != nil {
		t.Errorf("Require(true, ...).Return(): got %v", err)
	}
	err = Guard(cmd).Require(false, "object ID is required").Return()
	if err == nil {
		t.Fatal("Require(false, ...).Return(): expected non-nil")
	}
	if !strings.Contains(err.Error(), "object ID is required") {
		t.Errorf("error should contain message: %q", err.Error())
	}
}

func TestGuard_Wrapf(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	e := errors.New("underlying")
	err := Guard(cmd).Err(e).Wrapf("failed to create processor: %w").Return()
	if err == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(err.Error(), "failed to create processor") {
		t.Errorf("error should contain wrap message: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "underlying") {
		t.Errorf("error should contain underlying: %q", err.Error())
	}
}

func TestGuard_firstFailureWins(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	e := errors.New("err")
	// Require passes; Err sets error
	err := Guard(cmd).Require(true, "x").Err(e).Return()
	if err == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(err.Error(), "err") {
		t.Errorf("Err should win: %q", err.Error())
	}
	// Require fails first; Err is not applied
	err = Guard(cmd).Require(false, "required").Err(e).Return()
	if err == nil {
		t.Fatal("expected non-nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("Require should win: %q", err.Error())
	}
}

func TestEnhanceError_deduplicatesHint(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("object").Build()
	e := errors.New("validation failed: field invalid")
	err1 := EnhanceError(cmd, e)
	if err1 == nil {
		t.Fatal("expected non-nil error")
	}
	err2 := EnhanceError(cmd, err1)
	if err2 == nil {
		t.Fatal("expected non-nil error")
	}
	if strings.Count(err2.Error(), "Check field names and values") > 1 {
		t.Errorf("expected suggestion to appear at most once, got %q", err2.Error())
	}
}

func TestGuard_Requiref(t *testing.T) {
	cmd := pkgcli.NewCommandBuilder("x").Build()
	err := Guard(cmd).Requiref(true, "limit %d", 10).Return()
	if err != nil {
		t.Errorf("Requiref(true, ...): got %v", err)
	}
	err = Guard(cmd).Requiref(false, "count %d exceeds max %d", 15, 10).Return()
	if err == nil {
		t.Fatal("Requiref(false, ...): expected non-nil")
	}
	if !strings.Contains(err.Error(), "count 15 exceeds max 10") {
		t.Errorf("expected formatted string in error: %s", err.Error())
	}
}
