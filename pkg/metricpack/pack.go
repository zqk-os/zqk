// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package metricpack

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	_ "github.com/zqk-os/zqk/packs/metric/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/metric/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/metric/bldr_v2"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// Name is the pack id the composition root links.
	Name = "metric"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/metric/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/metric/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_metricpack"
)

// ErrUnknownKind is returned when an unsupported kind is queried.
var ErrUnknownKind = errors.New("unknown kind for metric pack")

// ErrNoLifecycle is returned when a kind does not define an explicit lifecycle.
var ErrNoLifecycle = errors.New("no lifecycle defined for kind in metric pack")

// getwd allows mocking fileutil.Getwd in tests for error path verification.
var getwd = fileutil.Getwd

// PackInfo provides structured metadata about the metric pack.
type PackInfo struct {
	Name           string   `json:"name"`
	SpecDir        string   `json:"spec_dir"`
	LifecycleDir   string   `json:"lifecycle_dir"`
	OmitBuildTag   string   `json:"omit_build_tag"`
	Kinds          []string `json:"kinds"`
	LifecycleKinds []string `json:"lifecycle_kinds"`
}

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
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
}

// LifecycleKinds returns the subset of object kinds that declare explicit lifecycle state machines.
func LifecycleKinds() []string {
	return []string{
		"audit_aggregation_metric",
		"scheduler_health_metric",
	}
}

// HasKind reports whether the given object kind belongs to this pack.
func HasKind(kind string) bool {
	for _, k := range Kinds() {
		if k == kind {
			return true
		}
	}
	return false
}

// HasLifecycle reports whether the given object kind defines a lifecycle in this pack.
func HasLifecycle(kind string) bool {
	for _, k := range LifecycleKinds() {
		if k == kind {
			return true
		}
	}
	return false
}

// ValidateKind checks if kind is supported by this pack, returning ErrUnknownKind or nil.
func ValidateKind(kind string) error {
	if !HasKind(kind) {
		return fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	return nil
}

// Info returns the PackInfo descriptor for the metric pack.
func Info() PackInfo {
	return PackInfo{
		Name:           Name,
		SpecDir:        SpecDir,
		LifecycleDir:   LifecycleDir,
		OmitBuildTag:   OmitBuildTag,
		Kinds:          Kinds(),
		LifecycleKinds: LifecycleKinds(),
	}
}

func resolveKindPath(baseDir, dir, file string) string {
	rel := filepath.Join(dir, file)
	if baseDir == "" {
		return rel
	}
	return filepath.Join(baseDir, rel)
}

// SpecPath returns the relative or resolved path to the spec file for a kind.
// If baseDir is non-empty, the returned path is joined with baseDir.
func SpecPath(baseDir, kind string) (string, error) {
	if err := ValidateKind(kind); err != nil {
		return "", err
	}
	return resolveKindPath(baseDir, SpecDir, kind+".yaml"), nil
}

// LifecyclePath returns the relative or resolved path to the lifecycle file for a kind.
// If baseDir is non-empty, the returned path is joined with baseDir.
// Returns ErrNoLifecycle if the kind exists but does not declare a lifecycle.
func LifecyclePath(baseDir, kind string) (string, error) {
	if err := ValidateKind(kind); err != nil {
		return "", err
	}
	if !HasLifecycle(kind) {
		return "", fmt.Errorf("%w: %q", ErrNoLifecycle, kind)
	}
	return resolveKindPath(baseDir, LifecycleDir, kind+"_lifecycle.yaml"), nil
}

// VerifyKindFiles validates that spec (and lifecycle when defined) files for the specified kinds
// exist under baseDir and are readable.
func VerifyKindFiles(baseDir string, kinds []string) error {
	for _, kind := range kinds {
		if err := ValidateKind(kind); err != nil {
			return fmt.Errorf("metric pack kind error: %w", err)
		}
		specPath := resolveKindPath(baseDir, SpecDir, kind+".yaml")
		if _, err := fileutil.Stat(specPath); err != nil {
			return fmt.Errorf("metric pack spec for %q missing or unreadable: %w", kind, err)
		}
		if HasLifecycle(kind) {
			lifecyclePath := resolveKindPath(baseDir, LifecycleDir, kind+"_lifecycle.yaml")
			if _, err := fileutil.Stat(lifecyclePath); err != nil {
				return fmt.Errorf("metric pack lifecycle for %q missing or unreadable: %w", kind, err)
			}
		}
	}
	return nil
}

// VerifyFiles validates that all spec and lifecycle files owned by this pack
// exist under baseDir and are readable.
func VerifyFiles(baseDir string) error {
	return VerifyKindFiles(baseDir, Kinds())
}

// ResolveModuleRoot searches up from startDir (or current working directory if empty)
// to find the root of the repository containing go.mod with the zqk module declaration.
func ResolveModuleRoot(startDir string) (string, error) {
	dir := startDir
	if dir == "" {
		cwd, err := getwd()
		if err != nil {
			return "", fmt.Errorf("resolve module root getwd: %w", err)
		}
		dir = cwd
	}
	dir = filepath.Clean(dir)
	for {
		goModPath := filepath.Join(dir, "go.mod")
		data, err := fileutil.ReadFile(goModPath)
		if err == nil {
			line, _, _ := strings.Cut(string(data), "\n")
			if strings.TrimSpace(line) == "module github.com/zqk-os/zqk" {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("module root containing github.com/zqk-os/zqk not found starting from %s", startDir)
		}
		dir = parent
	}
}

// VerifyPackInRepo locates the module root starting from startDir and verifies all pack files.
func VerifyPackInRepo(startDir string) error {
	root, err := ResolveModuleRoot(startDir)
	if err != nil {
		return err
	}
	return VerifyFiles(root)
}
