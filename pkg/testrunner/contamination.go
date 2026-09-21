package testrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ContaminationCheckOptions defines settings for guarding a directory plane during execution.
type ContaminationCheckOptions struct {
	ProjectRoot string
	PlanePath   string
}

// PlaneSnapshot maps relative file paths to their SHA-256 hex checksums.
type PlaneSnapshot map[string]string

// SnapshotPlane scans the plane path and hashes all YAML files into a PlaneSnapshot.
func SnapshotPlane(projectRoot, planePath string) (PlaneSnapshot, error) {
	fullPlane := planePath
	if !filepath.IsAbs(fullPlane) {
		fullPlane = filepath.Join(projectRoot, planePath)
	}

	info, err := os.Stat(fullPlane)
	if err != nil {
		return nil, errfmt.Newf("stat plane directory %q", fullPlane).Wrap(err)
	}
	if !info.IsDir() {
		return nil, errfmt.Errorf("plane path %q is not a directory", fullPlane)
	}

	snap := make(PlaneSnapshot)
	err = filepath.WalkDir(fullPlane, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".json") {
			return nil
		}

		data, err := fileutil.ReadFile(path)
		if err != nil {
			return errfmt.Newf("read file %q for snapshot", path).Wrap(err)
		}

		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(fullPlane, path)
		if err != nil {
			rel = path
		}
		snap[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, errfmt.Newf("snapshot plane %q", fullPlane).Wrap(err)
	}

	return snap, nil
}

// ContaminationDiff captures files that changed during command execution.
type ContaminationDiff struct {
	Added    []string
	Modified []string
	Deleted  []string
}

// HasContamination returns true if any files were added, modified, or deleted.
func (d ContaminationDiff) HasContamination() bool {
	return len(d.Added) > 0 || len(d.Modified) > 0 || len(d.Deleted) > 0
}

// AllChanged returns all changed relative paths sorted.
func (d ContaminationDiff) AllChanged() []string {
	var all []string
	for _, p := range d.Added {
		all = append(all, fmt.Sprintf("[added] %s", p))
	}
	for _, p := range d.Modified {
		all = append(all, fmt.Sprintf("[modified] %s", p))
	}
	for _, p := range d.Deleted {
		all = append(all, fmt.Sprintf("[deleted] %s", p))
	}
	sort.Strings(all)
	return all
}

// DiffSnapshots compares a before and after snapshot to detect modifications.
func DiffSnapshots(before, after PlaneSnapshot) ContaminationDiff {
	var diff ContaminationDiff

	for path, afterHash := range after {
		beforeHash, exists := before[path]
		if !exists {
			diff.Added = append(diff.Added, path)
		} else if beforeHash != afterHash {
			diff.Modified = append(diff.Modified, path)
		}
	}

	for path := range before {
		if _, exists := after[path]; !exists {
			diff.Deleted = append(diff.Deleted, path)
		}
	}

	sort.Strings(diff.Added)
	sort.Strings(diff.Modified)
	sort.Strings(diff.Deleted)
	return diff
}

// RunWithContaminationCheck runs an external command and fails if the plane directory was mutated.
func RunWithContaminationCheck(ctx context.Context, opts ContaminationCheckOptions, command []string, stdout, stderr io.Writer) (int, ContaminationDiff, error) {
	if len(command) == 0 {
		return 2, ContaminationDiff{}, errfmt.Errorf("no command specified")
	}

	before, err := SnapshotPlane(opts.ProjectRoot, opts.PlanePath)
	if err != nil {
		return 2, ContaminationDiff{}, err
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec // test runner exec
	cmd.Dir = opts.ProjectRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	after, err := SnapshotPlane(opts.ProjectRoot, opts.PlanePath)
	if err != nil {
		return exitCode, ContaminationDiff{}, errfmt.Newf("post-run snapshot failed").Wrap(err)
	}

	diff := DiffSnapshots(before, after)
	return exitCode, diff, runErr
}
