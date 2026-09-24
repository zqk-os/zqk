package errorsuggest

import (
	"errors"
	"strings"
	"testing"
)

func TestSuggest_notFound(t *testing.T) {
	err := errors.New("object BLI-999 not found")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Message != err.Error() {
		t.Errorf("Message: got %q", s.Message)
	}
	if s.Hint == emptyValue {
		t.Error("expected non-empty hint for not found")
	}
	if !strings.Contains(s.Hint, "list") {
		t.Errorf("hint should mention list: %q", s.Hint)
	}
}

func TestSuggest_notFoundBeginner(t *testing.T) {
	err := errors.New("no such file")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceBeginner}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("expected hint")
	}
	if !strings.Contains(s.Hint, "list") {
		t.Errorf("hint should mention list: %q", s.Hint)
	}
}

func TestSuggest_validation(t *testing.T) {
	err := errors.New("validation failed: invalid field X")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("expected hint for validation")
	}
	if !strings.Contains(s.Hint, "dry-run") {
		t.Errorf("hint should mention dry-run: %q", s.Hint)
	}
}

func TestSuggest_validationPromote(t *testing.T) {
	err := errors.New("validation failed: goal_refs required")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard, CommandPath: "zqk object promote"}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Fatal("expected hint for promote validation")
	}
	if strings.Contains(s.Hint, "--dry-run") || strings.Contains(s.Hint, "--relaxed") {
		t.Errorf("promote hint must not mention non-existent flags: %q", s.Hint)
	}
	if !strings.Contains(s.Hint, "update") {
		t.Errorf("promote hint should mention update: %q", s.Hint)
	}
}

func TestSuggest_permission(t *testing.T) {
	err := errors.New("permission denied")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("expected hint for permission")
	}
}

func TestSuggest_unknownFlagFieldSuggestsFields(t *testing.T) {
	err := errors.New("unknown flag: --field")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard, CommandPath: "zqk object list"}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Fatal("expected hint for unknown flag")
	}
	if !strings.Contains(s.Hint, "--fields") {
		t.Errorf("hint should suggest --fields: %q", s.Hint)
	}
	if !strings.Contains(strings.ToLower(s.Hint), "did you mean") {
		t.Errorf("hint should say did you mean: %q", s.Hint)
	}
}

func TestSuggest_unknownCommand(t *testing.T) {
	err := errors.New("unknown command: obect")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("expected hint for unknown command")
	}
	if !strings.Contains(s.Hint, "help") {
		t.Errorf("hint should mention help: %q", s.Hint)
	}
}

func TestSuggest_required(t *testing.T) {
	err := errors.New("required flag --id not set")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("expected hint for required")
	}
}

func TestSuggest_expertTruncates(t *testing.T) {
	err := errors.New("object not found")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceExpert}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("expected hint")
	}
	if len(s.Hint) > 83 {
		t.Errorf("expert hint should be truncated: len=%d", len(s.Hint))
	}
}

func TestSuggest_verboseAddsHint(t *testing.T) {
	err := errors.New("not found")
	opts := Options{Verbose: true, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("verbose should still produce hint")
	}
}

func TestSuggest_genericNoHint(t *testing.T) {
	err := errors.New("something went wrong")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint != emptyValue {
		t.Errorf("generic error should have no hint by default, got %q", s.Hint)
	}
}

func TestSuggest_genericVerboseHint(t *testing.T) {
	err := errors.New("something went wrong")
	opts := Options{Verbose: true, ExperienceLevel: ExperienceStandard}
	s := Suggest(err, opts)
	if s.Hint == emptyValue {
		t.Error("verbose generic error should get hint")
	}
	if !strings.Contains(s.Hint, "verbose") {
		t.Errorf("hint should mention verbose: %q", s.Hint)
	}
}

func TestExperienceFromProfile(t *testing.T) {
	tests := []struct {
		profile string
		want    ExperienceLevel
	}{
		{"human", ExperienceStandard},
		{"", ExperienceStandard},
		{"ai-agent", ExperienceExpert},
		{"mcp", ExperienceExpert},
		{"system", ExperienceExpert},
		{"debug", ExperienceExpert},
		{"quiet", ExperienceExpert},
		{"Human", ExperienceStandard},
	}
	for _, tt := range tests {
		got := ExperienceFromProfile(tt.profile)
		if got != tt.want {
			t.Errorf("ExperienceFromProfile(%q) = %q, want %q", tt.profile, got, tt.want)
		}
	}
}

func TestFormat(t *testing.T) {
	err := errors.New("object not found")
	opts := Options{Verbose: false, ExperienceLevel: ExperienceStandard}
	out := Format(err, opts)
	if out == emptyValue {
		t.Error("Format should not return empty")
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("output should contain error message: %q", out)
	}
	if !strings.Contains(out, "list") {
		t.Errorf("output should contain hint: %q", out)
	}
}

func TestSuggest_nil(t *testing.T) {
	s := Suggest(nil, Options{})
	if s.Message != emptyValue || s.Hint != emptyValue {
		t.Errorf("Suggest(nil) should return zero value: %+v", s)
	}
}
