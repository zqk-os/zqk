//go:build !zqk_omit_workpack

package main

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/workpack"
)

func TestIncludedWorkPackLinked(t *testing.T) {
	if !workpack.Enabled() {
		t.Fatal("composition root did not link the included work pack")
	}
	if workpack.Name != "work" {
		t.Fatalf("pack name %q", workpack.Name)
	}
}
