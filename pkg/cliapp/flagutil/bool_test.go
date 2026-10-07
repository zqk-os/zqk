package flagutil

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestIsTrue(t *testing.T) {
	// Nil flag
	if IsTrue(nil) {
		t.Errorf("expected IsTrue(nil) == false")
	}

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.Bool("active", false, "")
	fs.String("name", "foo", "")

	fBool := fs.Lookup("active")
	if IsTrue(fBool) {
		t.Errorf("expected IsTrue to be false for default false bool")
	}

	_ = fs.Set("active", "true")
	if !IsTrue(fBool) {
		t.Errorf("expected IsTrue to be true after set to true")
	}

	fStr := fs.Lookup("name")
	if IsTrue(fStr) {
		t.Errorf("expected IsTrue to be false for non-boolean string")
	}
}
