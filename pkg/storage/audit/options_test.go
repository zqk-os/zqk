package audit

import (
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestApplySessionEnv(t *testing.T) {
	t.Setenv(zqkenv.SessionID(), "")
	opts := &EventOptions{SessionID: "kept"}
	ApplySessionEnv(opts)
	if opts.SessionID != "kept" {
		t.Fatalf("explicit session overwritten: %q", opts.SessionID)
	}

	t.Setenv(zqkenv.SessionID(), "  SESS-env  ")
	empty := &EventOptions{}
	ApplySessionEnv(empty)
	if empty.SessionID != "SESS-env" {
		t.Fatalf("env session=%q", empty.SessionID)
	}
}
