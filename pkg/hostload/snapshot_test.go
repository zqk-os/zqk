package hostload

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestDisabled_ReadsBrandEnv(t *testing.T) {
	t.Setenv(zqkenv.HostloadDisable().Name(), "1")
	if !Disabled() {
		t.Fatal("HOSTLOAD_DISABLE=1 must disable sensing")
	}
	t.Setenv(zqkenv.HostloadDisable().Name(), "")
	if Disabled() {
		t.Fatal("empty HOSTLOAD_DISABLE is not disabled")
	}
}
