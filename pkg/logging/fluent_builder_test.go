package logging

import (
	"bytes"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestFluentWarnChainsFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, WarnLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	Fluent(logger).Warn("fluent_test_warn").String("k", "v").Int("n", 1).Log()
	out := buf.String()
	if !strings.Contains(out, "fluent_test_warn") || !strings.Contains(out, "k=v") {
		t.Fatalf("unexpected log output: %q", out)
	}
}
