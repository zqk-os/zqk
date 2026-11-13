// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package releasegate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zqk-os/zqk/pkg/strutil"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ReleaseNotesOptions configures verification criteria for release note documentation.
type ReleaseNotesOptions struct {
	ReleasesDir               string
	MinCharacterCount         int
	RequiredSections          []string
	DisallowUnexpandedStubs   bool
	DisallowInternalKernelIDs bool
}

// ReleaseNotesFileResult records verification findings for a single release note markdown file.
type ReleaseNotesFileResult struct {
	Path           string   `json:"path"`
	Version        string   `json:"version"`
	CharacterCount int      `json:"character_count"`
	MissingHeaders []string `json:"missing_headers,omitempty"`
	Issues         []string `json:"issues,omitempty"`
	Passed         bool     `json:"passed"`
}

// ReleaseNotesVerificationResult captures aggregated audit metrics across all release notes.
type ReleaseNotesVerificationResult struct {
	TotalFilesVerified int                       `json:"total_files_verified"`
	PassedFiles        int                       `json:"passed_files"`
	FailedFiles        int                       `json:"failed_files"`
	FileResults        []*ReleaseNotesFileResult `json:"file_results"`
	AllPassed          bool                      `json:"all_passed"`
}

// ReleaseNotesChecker inspects release note documents to enforce comprehensive elaboration,
// component provenance, operator handoff matrices, and clean public sanitization.
type ReleaseNotesChecker struct {
	opts ReleaseNotesOptions
}

// DefaultReleaseNotesOptions returns hardened baseline criteria for public release manifests.
func DefaultReleaseNotesOptions(releasesDir string) ReleaseNotesOptions {
	return ReleaseNotesOptions{
		ReleasesDir:       releasesDir,
		MinCharacterCount: 2000,
		RequiredSections: []string{
			"Highlights & Strategic Capabilities",
			"Community Issue Burndown & Bug Fixes",
			"Upgrade Guide",
		},
		DisallowUnexpandedStubs:   true,
		DisallowInternalKernelIDs: true,
	}
}

// NewReleaseNotesChecker initializes a new ReleaseNotesChecker.
func NewReleaseNotesChecker(opts ReleaseNotesOptions) (*ReleaseNotesChecker, error) {
	if err := strutil.ErrorOnBlank(opts.ReleasesDir, "releases directory cannot be blank"); err != nil {
		return nil, err
	}
	if opts.MinCharacterCount <= 0 {
		opts.MinCharacterCount = 2000
	}
	if len(opts.RequiredSections) == 0 {
		opts.RequiredSections = []string{
			"Highlights & Strategic Capabilities",
			"Community Issue Burndown & Bug Fixes",
			"Upgrade Guide",
		}
	}
	return &ReleaseNotesChecker{opts: opts}, nil
}

// VerifyFile inspects a specific release note markdown file against all verification criteria.
func (c *ReleaseNotesChecker) VerifyFile(filePath string) (*ReleaseNotesFileResult, error) {
	if err := strutil.ErrorOnBlank(filePath, "release notes file path cannot be blank"); err != nil {
		return nil, err
	}

	contentBytes, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read release note at %s: %w", filePath, err)
	}

	rawContent := string(contentBytes)
	trimmedContent := strings.TrimSpace(rawContent)
	base := filepath.Base(filePath)
	version := strings.TrimSuffix(base, filepath.Ext(base))

	result := &ReleaseNotesFileResult{
		Path:           filePath,
		Version:        version,
		CharacterCount: len(trimmedContent),
		MissingHeaders: make([]string, 0),
		Issues:         make([]string, 0),
		Passed:         true,
	}

	if len(trimmedContent) < c.opts.MinCharacterCount {
		result.Issues = append(result.Issues, fmt.Sprintf(
			"release note is insufficiently elaborated: %d characters (minimum required: %d)",
			len(trimmedContent), c.opts.MinCharacterCount,
		))
	}

	// Verify required sections
	for _, reqSection := range c.opts.RequiredSections {
		escaped := regexp.QuoteMeta(reqSection)
		re := regexp.MustCompile("(?i)#+\\s+" + escaped)
		if !re.MatchString(rawContent) {
			result.MissingHeaders = append(result.MissingHeaders, reqSection)
			result.Issues = append(result.Issues, fmt.Sprintf("missing required section header: %q", reqSection))
		}
	}

	// Disallow unexpanded stubs
	if c.opts.DisallowUnexpandedStubs {
		stubRegex := regexp.MustCompile(`(?i)\b(TODO|TBD|FIXME|WIP|PLACEHOLDER)\b`)
		if matches := stubRegex.FindAllString(rawContent, -1); len(matches) > 0 {
			result.Issues = append(result.Issues, fmt.Sprintf(
				"release note contains %d unexpanded stub markers (%v)", len(matches), matches,
			))
		}
	}

	// Disallow unscrubbed tracking tags and internal kernel ids
	if c.opts.DisallowInternalKernelIDs {
		trackRegex := regexp.MustCompile(`(?i)(//\s*TRACK:|TRACK:\s*BLI-|TRACK:\s*REQ-)`)
		if matches := trackRegex.FindAllString(rawContent, -1); len(matches) > 0 {
			result.Issues = append(result.Issues, fmt.Sprintf(
				"release note contains %d studio TRACK markers (%v)", len(matches), matches,
			))
		}
	}

	if len(result.Issues) > 0 {
		result.Passed = false
	}

	return result, nil
}

// VerifyAll scans the configured releases directory and validates all markdown release notes.
func (c *ReleaseNotesChecker) VerifyAll() (*ReleaseNotesVerificationResult, error) {
	entries, err := fileutil.ReadDir(c.opts.ReleasesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read releases directory %s: %w", c.opts.ReleasesDir, err)
	}

	result := &ReleaseNotesVerificationResult{
		FileResults: make([]*ReleaseNotesFileResult, 0),
		AllPassed:   true,
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// Match release notes files like v0.3.3.md, v0.3.4.md, etc.
		if !strings.HasPrefix(entry.Name(), "v") {
			continue
		}

		fullPath := filepath.Join(c.opts.ReleasesDir, entry.Name())
		fileRes, err := c.VerifyFile(fullPath)
		if err != nil {
			return nil, err
		}

		result.TotalFilesVerified++
		result.FileResults = append(result.FileResults, fileRes)
		if fileRes.Passed {
			result.PassedFiles++
		} else {
			result.FailedFiles++
			result.AllPassed = false
		}
	}

	if result.TotalFilesVerified == 0 {
		return nil, fmt.Errorf("no release note markdown files found in %s", c.opts.ReleasesDir)
	}

	return result, nil
}
