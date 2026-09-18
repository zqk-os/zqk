package agentonboard

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// WriteResult summarizes a prime-workspace write pass.
type WriteResult struct {
	Written []string
	Skipped []string // existing files left untouched (use Force to overwrite)
}

// WriteVendorConfigs writes BootPayloadFor each vendor's config paths.
// Existing files are skipped unless force is true (non-destructive default).
func WriteVendorConfigs(projectRoot string, vendors []Vendor, dryRun, force bool) (WriteResult, error) {
	var out WriteResult
	for _, v := range vendors {
		payload := []byte(BootPayloadFor(v))
		for _, rel := range v.ConfigRelPaths {
			relSlash := filepath.ToSlash(rel)
			abs := filepath.Join(projectRoot, filepath.FromSlash(rel))
			exists := pathExists(abs)
			if exists && !force {
				out.Skipped = append(out.Skipped, relSlash)
				continue
			}
			if dryRun {
				out.Written = append(out.Written, relSlash)
				continue
			}
			if err := fileutil.EnsureDir(filepath.Dir(abs)); err != nil {
				return out, errfmt.Newf("ensure dir for %s", relSlash).Wrap(err)
			}
			if err := fileutil.WriteSecureFile(abs, payload); err != nil {
				return out, errfmt.Newf("write agent config %s", relSlash).Wrap(err)
			}
			out.Written = append(out.Written, relSlash)
		}
	}
	return out, nil
}

// SyncReport is the workspace→kernel registration artifact.
type SyncReport struct {
	Schema         string           `json:"schema"`
	UpdatedAt      string           `json:"updated_at"`
	Vector         string           `json:"vector"`
	Fingerprint    string           `json:"fingerprint,omitempty"`
	Detected       []DetectedVendor `json:"detected"`
	PrimedFiles    []string         `json:"primed_files"`
	SkippedFiles   []string         `json:"skipped_files,omitempty"`
	PackPaths      []string         `json:"pack_paths,omitempty"`
	EdgeSignals    []EdgeSignal     `json:"edge_signals,omitempty"`
	SeatingCreated int              `json:"seating_created"`
	SmokeOK        bool             `json:"smoke_ok"`
	Notes          []string         `json:"notes,omitempty"`
}

// WriteSyncReport persists the sync report under SyncReportRelPath.
func WriteSyncReport(projectRoot string, report SyncReport, dryRun bool) (relPath string, err error) {
	relPath = SyncReportRelPath
	if report.Schema == "" {
		report.Schema = SyncReportSchema
	}
	if report.UpdatedAt == "" {
		report.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if dryRun {
		return relPath, nil
	}
	abs := filepath.Join(projectRoot, filepath.FromSlash(relPath))
	if err := fileutil.EnsureDir(filepath.Dir(abs)); err != nil {
		return "", errfmt.Newf("ensure sync report dir").Wrap(err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", errfmt.Newf("marshal sync report").Wrap(err)
	}
	data = append(data, '\n')
	if err := fileutil.WriteSecureFile(abs, data); err != nil {
		return "", errfmt.Newf("write sync report").Wrap(err)
	}
	return relPath, nil
}

// ReadSyncReport loads an existing sync report if present.
func ReadSyncReport(projectRoot string) (*SyncReport, error) {
	abs := filepath.Join(projectRoot, filepath.FromSlash(SyncReportRelPath))
	data, err := fileutil.ReadFile(abs)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil, nil
		}
		return nil, errfmt.Newf("read sync report").Wrap(err)
	}
	var report SyncReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, errfmt.Newf("parse sync report").Wrap(err)
	}
	return &report, nil
}
