package convergence

import "testing"

func TestControl(t *testing.T) {
	if !Control() {
		t.Error("failed")
	}
}
