package scheduler

import "testing"

func TestValidateConvergenceSessionTickTargetID(t *testing.T) {
	t.Parallel()
	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		id := "CONV-EXAMPLE"
		if err := validateConvergenceSessionTickTargetID(id); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("reject wrong prefix", func(t *testing.T) {
		t.Parallel()
		if err := validateConvergenceSessionTickTargetID("PLAN-EXAMPLE"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("reject uppercase hex", func(t *testing.T) {
		t.Parallel()
		if err := validateConvergenceSessionTickTargetID("CONV-EXAMPLE-A726D525"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("reject empty", func(t *testing.T) {
		t.Parallel()
		if err := validateConvergenceSessionTickTargetID(""); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("reject pathy", func(t *testing.T) {
		t.Parallel()
		if err := validateConvergenceSessionTickTargetID("CONV-../1776080007703030000-a726d525"); err == nil {
			t.Fatal("expected error")
		}
	})
}
