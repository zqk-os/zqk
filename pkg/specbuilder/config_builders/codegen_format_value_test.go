package config_builders

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestFormatValue_systemAccountUsesConstant(t *testing.T) {
	got := formatValue(objects.DefaultSystemAccountID, 1, defaultVersionPackage, "paths_config")
	if got != "objects.DefaultSystemAccountID" {
		t.Fatalf("formatValue(system account) = %q, want objects.DefaultSystemAccountID", got)
	}
}

func TestFormatValue_otherStringStaysQuoted(t *testing.T) {
	got := formatValue("paths_config", 1, defaultVersionPackage, "paths_config")
	if got != `"paths_config"` {
		t.Fatalf("formatValue(other) = %q, want quoted literal", got)
	}
}
