// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package paths_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestPathsResolver(t *testing.T) {
	root := "/tmp/testroot"
	resolver := paths.NewPathResolver(root)
	if resolver == nil {
		t.Fatalf("expected non-nil resolver")
	}
	if resolver.ProjectRoot() != root {
		t.Errorf("expected root %s, got %s", root, resolver.ProjectRoot())
	}
}
