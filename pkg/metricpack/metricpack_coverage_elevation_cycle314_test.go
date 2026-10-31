// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package metricpack

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestConstantsAndPackInfo(t *testing.T) {
	assert.Equal(t, "metric", Name)
	assert.Equal(t, "packs/metric/specs", SpecDir)
	assert.Equal(t, "packs/metric/lifecycles", LifecycleDir)
	assert.Equal(t, "zqk_omit_metricpack", OmitBuildTag)

	info := Info()
	assert.Equal(t, Name, info.Name)
	assert.Equal(t, SpecDir, info.SpecDir)
	assert.Equal(t, LifecycleDir, info.LifecycleDir)
	assert.Equal(t, OmitBuildTag, info.OmitBuildTag)
	assert.Equal(t, Kinds(), info.Kinds)
	assert.Equal(t, LifecycleKinds(), info.LifecycleKinds)
}

func TestKindsAndHasKind(t *testing.T) {
	kinds := Kinds()
	require.Len(t, kinds, 12)

	expectedKinds := []string{
		"audit_aggregation_metric",
		"base_sampler",
		"command_metric",
		"file_lock_metric",
		"kind_mapping_metric",
		"list_metric_sampler",
		"metadata_package",
		"ordered_list_metric_sampler",
		"sampler_profile",
		"scalar_metric_sampler",
		"scheduler_health_metric",
		"status_history_metric_sampler",
	}
	assert.Equal(t, expectedKinds, kinds)

	seen := make(map[string]bool)
	for _, k := range kinds {
		assert.NotEmpty(t, k)
		assert.False(t, seen[k], "duplicate kind found: "+k)
		seen[k] = true
		assert.True(t, HasKind(k))
	}

	assert.False(t, HasKind("unknown_kind"))
	assert.False(t, HasKind(""))
	assert.False(t, HasKind("METRIC"))

	lifecycleKinds := LifecycleKinds()
	require.Len(t, lifecycleKinds, 2)
	assert.Contains(t, lifecycleKinds, "audit_aggregation_metric")
	assert.Contains(t, lifecycleKinds, "scheduler_health_metric")

	for _, k := range lifecycleKinds {
		assert.True(t, HasLifecycle(k))
	}
	assert.False(t, HasLifecycle("base_sampler"))
	assert.False(t, HasLifecycle("unknown_kind"))
	assert.False(t, HasLifecycle(""))
}

func TestValidateKind(t *testing.T) {
	for _, k := range Kinds() {
		assert.NoError(t, ValidateKind(k))
	}

	err := ValidateKind("invalid_kind")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnknownKind))
}

func TestSpecPath(t *testing.T) {
	t.Run("empty baseDir", func(t *testing.T) {
		p, err := SpecPath("", "base_sampler")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(SpecDir, "base_sampler.yaml"), p)
	})

	t.Run("with baseDir", func(t *testing.T) {
		base := "/opt/zqk"
		p, err := SpecPath(base, "command_metric")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(base, SpecDir, "command_metric.yaml"), p)
	})

	t.Run("unknown kind", func(t *testing.T) {
		_, err := SpecPath("", "invalid_kind")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnknownKind))
	})
}

func TestLifecyclePath(t *testing.T) {
	t.Run("empty baseDir with lifecycle", func(t *testing.T) {
		p, err := LifecyclePath("", "audit_aggregation_metric")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(LifecycleDir, "audit_aggregation_metric_lifecycle.yaml"), p)
	})

	t.Run("with baseDir with lifecycle", func(t *testing.T) {
		base := "/opt/zqk"
		p, err := LifecyclePath(base, "scheduler_health_metric")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(base, LifecycleDir, "scheduler_health_metric_lifecycle.yaml"), p)
	})

	t.Run("kind without lifecycle", func(t *testing.T) {
		_, err := LifecyclePath("", "base_sampler")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoLifecycle))
	})

	t.Run("unknown kind", func(t *testing.T) {
		_, err := LifecyclePath("", "unknown_kind")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnknownKind))
	})
}

func TestVerifyKindFilesAndVerifyFiles(t *testing.T) {
	root, err := ResolveModuleRoot("")
	require.NoError(t, err)

	t.Run("verify existing kinds in repo", func(t *testing.T) {
		err := VerifyKindFiles(root, Kinds())
		assert.NoError(t, err)
	})

	t.Run("verify all files in repo", func(t *testing.T) {
		err := VerifyFiles(root)
		assert.NoError(t, err)
	})

	t.Run("verify invalid kind", func(t *testing.T) {
		err := VerifyKindFiles(root, []string{"nonexistent_kind"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUnknownKind))
	})

	t.Run("missing spec file in mock directory", func(t *testing.T) {
		tmp := t.TempDir()
		err := VerifyKindFiles(tmp, []string{"base_sampler"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing or unreadable")
	})

	t.Run("spec exists but missing lifecycle for lifecycle kind", func(t *testing.T) {
		tmp := t.TempDir()
		specDir := filepath.Join(tmp, SpecDir)
		require.NoError(t, fileutil.MkdirAll(specDir, 0755))
		specFile := filepath.Join(specDir, "audit_aggregation_metric.yaml")
		require.NoError(t, fileutil.WriteFile(specFile, []byte("kind: audit_aggregation_metric\n"), 0644))

		err := VerifyKindFiles(tmp, []string{"audit_aggregation_metric"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lifecycle for \"audit_aggregation_metric\" missing or unreadable")
	})
}

func TestResolveModuleRoot(t *testing.T) {
	t.Run("from cwd", func(t *testing.T) {
		root, err := ResolveModuleRoot("")
		require.NoError(t, err)
		assert.NotEmpty(t, root)
		stat, err := fileutil.Stat(filepath.Join(root, "go.mod"))
		require.NoError(t, err)
		assert.False(t, stat.IsDir())
	})

	t.Run("from subdirectory", func(t *testing.T) {
		root, err := ResolveModuleRoot("")
		require.NoError(t, err)
		sub := filepath.Join(root, "pkg", "metricpack")
		found, err := ResolveModuleRoot(sub)
		require.NoError(t, err)
		assert.Equal(t, root, found)
	})

	t.Run("from temp tree with valid go.mod", func(t *testing.T) {
		tmp := t.TempDir()
		goMod := filepath.Join(tmp, "go.mod")
		require.NoError(t, fileutil.WriteFile(goMod, []byte("module github.com/zqk-os/zqk\n\ngo 1.24\n"), 0644))
		nested := filepath.Join(tmp, "a", "b", "c")
		require.NoError(t, fileutil.MkdirAll(nested, 0755))

		found, err := ResolveModuleRoot(nested)
		require.NoError(t, err)
		assert.Equal(t, tmp, found)
	})

	t.Run("not found in temp dir", func(t *testing.T) {
		tmp := t.TempDir()
		nested := filepath.Join(tmp, "empty", "sub")
		require.NoError(t, fileutil.MkdirAll(nested, 0755))

		_, err := ResolveModuleRoot(nested)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "module root containing github.com/zqk-os/zqk not found")
	})

	t.Run("getwd error handling", func(t *testing.T) {
		origGetwd := getwd
		defer func() { getwd = origGetwd }()

		getwd = func() (string, error) {
			return "", errors.New("simulated getwd failure")
		}

		_, err := ResolveModuleRoot("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolve module root getwd")
	})
}

func TestVerifyPackInRepo(t *testing.T) {
	t.Run("success in real repo", func(t *testing.T) {
		require.NoError(t, VerifyPackInRepo(""))
	})

	t.Run("failure when not in repo", func(t *testing.T) {
		tmp := t.TempDir()
		err := VerifyPackInRepo(tmp)
		require.Error(t, err)
	})
}

func TestConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	const workerCount = 16

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("test_metricpack_concurrency", fmt.Sprintf("worker-%d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				info := Info()
				if info.Name != "metric" {
					t.Errorf("unexpected name: %s", info.Name)
				}
				kinds := Kinds()
				if len(kinds) != 12 {
					t.Errorf("unexpected kinds length: %d", len(kinds))
				}
				lifecycleKinds := LifecycleKinds()
				if len(lifecycleKinds) != 2 {
					t.Errorf("unexpected lifecycle kinds length: %d", len(lifecycleKinds))
				}
				for _, k := range kinds {
					if !HasKind(k) {
						t.Errorf("expected HasKind(%s) to be true", k)
					}
					if err := ValidateKind(k); err != nil {
						t.Errorf("unexpected ValidateKind error: %v", err)
					}
					_, _ = SpecPath("", k)
					if HasLifecycle(k) {
						_, _ = LifecyclePath("", k)
					}
				}
			})
	}
	wg.Wait()
}
