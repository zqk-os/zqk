package fileutil

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestObjectYAMLIOLimit_Default(t *testing.T) {
	t.Setenv(zqkenv.MaxObjectYAMLIO().Name(), "")
	// Once is process-wide; if another test already initialized, skip exact default.
	n := ObjectYAMLIOLimit()
	if n < minMaxObjectYAMLIO || n > maxMaxObjectYAMLIO {
		t.Fatalf("ObjectYAMLIOLimit=%d want [%d,%d]", n, minMaxObjectYAMLIO, maxMaxObjectYAMLIO)
	}
}

func TestReadFileGated_Missing(t *testing.T) {
	_, err := ReadFileGated("/no/such/zqk-object-yaml-io-test.yaml")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}
