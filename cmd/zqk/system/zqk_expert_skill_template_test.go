package system

import (
	"strings"
	"testing"
)

func TestZQKExpertSkillContent_IncludesNativePrimitiveMap(t *testing.T) {
	required := []string{
		"zqk --help",
		"zqk eval validate|run|matrix",
		"zqk gate stage|review|approve|reject",
		"zqk test discover",
		"zqk test bind",
		"zqk test run --all",
		"zqk test dashboard",
		"zqk new object-spec",
		"zqk pack",
	}
	for _, needle := range required {
		if !strings.Contains(ZQKExpertSkillContent, needle) {
			t.Errorf("ZQKExpertSkillContent missing native primitive directive %q", needle)
		}
	}
}
