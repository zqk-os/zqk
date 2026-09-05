package hostload

import (
	"testing"

	"github.com/lanceman/zqk/pkg/brand"
)

func TestDisabled_ReadsBrandEnv(t *testing.T) {
	t.Setenv(brand.EnvVar("HOSTLOAD_DISABLE"), "1")
	if !Disabled() {
		t.Fatal("HOSTLOAD_DISABLE=1 must disable sensing")
	}
	t.Setenv(brand.EnvVar("HOSTLOAD_DISABLE"), "")
	if Disabled() {
		t.Fatal("empty HOSTLOAD_DISABLE is not disabled")
	}
}
