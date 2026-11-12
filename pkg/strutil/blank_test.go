package strutil

import (
	"testing"
)

func TestErrorOnBlank(t *testing.T) {
	tests := []struct {
		name       string
		val        string
		format     string
		args       []any
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "empty string with format and args",
			val:        "",
			format:     "field %s is required",
			args:       []any{"agent_id"},
			wantErr:    true,
			wantErrMsg: "field agent_id is required",
		},
		{
			name:       "whitespace only with format without args",
			val:        "   \t\n  ",
			format:     "cannot be blank",
			args:       nil,
			wantErr:    true,
			wantErrMsg: "cannot be blank",
		},
		{
			name:       "empty string without format",
			val:        "",
			format:     "",
			args:       nil,
			wantErr:    true,
			wantErrMsg: "value cannot be blank",
		},
		{
			name:       "valid non-blank string",
			val:        "valid-value",
			format:     "should not trigger",
			args:       nil,
			wantErr:    false,
			wantErrMsg: "",
		},
		{
			name:       "string with surrounding whitespace but valid content",
			val:        "  hello world  ",
			format:     "should not trigger",
			args:       nil,
			wantErr:    false,
			wantErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ErrorOnBlank(tt.val, tt.format, tt.args...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ErrorOnBlank() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err.Error() != tt.wantErrMsg {
				t.Errorf("ErrorOnBlank() err msg = %q, want %q", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestRequireNonBlank(t *testing.T) {
	if err := RequireNonBlank("   ", "username"); err == nil || err.Error() != "username cannot be blank" {
		t.Fatalf("expected 'username cannot be blank', got: %v", err)
	}

	if err := RequireNonBlank("", ""); err == nil || err.Error() != "value cannot be blank" {
		t.Fatalf("expected default blank error, got: %v", err)
	}

	if err := RequireNonBlank("valid", "username"); err != nil {
		t.Fatalf("expected nil error for valid input, got: %v", err)
	}
}

func TestIsBlankAndIsNonBlank(t *testing.T) {
	if !IsBlank("") {
		t.Errorf("expected empty string to be blank")
	}
	if !IsBlank("   \n ") {
		t.Errorf("expected whitespace to be blank")
	}
	if IsBlank("abc") {
		t.Errorf("expected 'abc' not to be blank")
	}

	if IsNonBlank("") {
		t.Errorf("expected empty string not to be non-blank")
	}
	if IsNonBlank("   ") {
		t.Errorf("expected whitespace not to be non-blank")
	}
	if !IsNonBlank("abc") {
		t.Errorf("expected 'abc' to be non-blank")
	}
}
