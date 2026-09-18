package scheduler

import (
	"os"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCriteriaAutoValidateDisabled(t *testing.T) {
	key := zqkenv.DisableCriteriaAutoValidate()
	t.Run("unset is enabled path", func(t *testing.T) {
		if err := os.Unsetenv(key.Name()); err != nil {
			t.Fatal(err)
		}
		if criteriaAutoValidateDisabled() {
			t.Fatal("expected auto-validate on when unset")
		}
	})
	t.Run("truthy disables", func(t *testing.T) {
		t.Setenv(key.Name(), "1")
		if !criteriaAutoValidateDisabled() {
			t.Fatal("expected disabled when env set")
		}
	})
}
