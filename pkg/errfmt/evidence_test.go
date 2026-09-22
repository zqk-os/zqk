// BLI-STARTER-COMMUNITY-018 / PRI-STARTER-COMMUNITY-048
package errfmt

import (
	"errors"
	"strings"
	"testing"
)

func TestEvidenceNewfWrap(t *testing.T) {
	err := Newf("boom %s", "x").Wrap(errors.New("inner"))
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "inner") {
		t.Fatalf("got %v", err)
	}
	if Errorf("plain %d", 1) == nil {
		t.Fatal("Errorf")
	}
}
