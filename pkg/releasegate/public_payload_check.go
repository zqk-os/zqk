// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package releasegate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/community"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// PayloadVerificationOptions configures the public release payload verification.
// TRACK: /
type PayloadVerificationOptions struct {
	ReleaseDir       string
	Version          string
	RequirePlatforms []community.TargetPlatform
	RequireLicense   bool
	CheckNoSecrets   bool
	DryRunOnly       bool
}

// PayloadVerificationResult holds the outcome of a payload audit.
type PayloadVerificationResult struct {
	Version          string                     `json:"version"`
	VerifiedArchives []string                   `json:"verified_archives"`
	ChecksumsValid   bool                       `json:"checksums_valid"`
	PlatformsCovered []community.TargetPlatform `json:"platforms_covered"`
	Issues           []string                   `json:"issues,omitempty"`
}

// PublicPayloadChecker provides an ergonomic, configurable verifier for public release payloads.
type PublicPayloadChecker struct {
	opts PayloadVerificationOptions
}

// NewPublicPayloadChecker initializes a new PublicPayloadChecker with the given options.
func NewPublicPayloadChecker(opts PayloadVerificationOptions) *PublicPayloadChecker {
	return &PublicPayloadChecker{opts: opts}
}

// Options returns the underlying PayloadVerificationOptions.
func (c *PublicPayloadChecker) Options() PayloadVerificationOptions {
	return c.opts
}

// Verify executes verification on the configured payload.
func (c *PublicPayloadChecker) Verify() (*PayloadVerificationResult, error) {
	return VerifyPublicReleasePayload(c.opts)
}

// VerifyPublicReleasePayload ensures that release artifacts are fully verified,
// hermetic, and valid before any public release, enforcing that no push operations occur.
func VerifyPublicReleasePayload(opts PayloadVerificationOptions) (*PayloadVerificationResult, error) {
	if strings.TrimSpace(opts.ReleaseDir) == "" {
		return nil, errors.New("release directory is required")
	}

	info, err := fileutil.Stat(opts.ReleaseDir)
	if err != nil {
		return nil, fmt.Errorf("release directory inaccessible: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("release directory %q is not a directory", opts.ReleaseDir)
	}

	if err := community.ValidateReleaseTag(opts.Version); err != nil {
		return nil, fmt.Errorf("invalid release tag %q: %w", opts.Version, err)
	}

	platforms := opts.RequirePlatforms
	if len(platforms) == 0 {
		platforms = community.SupportedPlatforms
	}

	res := &PayloadVerificationResult{
		Version:          opts.Version,
		VerifiedArchives: make([]string, 0),
		PlatformsCovered: make([]community.TargetPlatform, 0),
		Issues:           make([]string, 0),
	}

	checksumPath := filepath.Join(opts.ReleaseDir, "checksums.txt")
	checksumData, err := fileutil.ReadFile(checksumPath)
	if err != nil {
		return nil, fmt.Errorf("missing or unreadable checksums.txt in %s: %w", opts.ReleaseDir, err)
	}

	manifest, err := community.ParseChecksumManifest(string(checksumData))
	if err != nil {
		return nil, fmt.Errorf("corrupt checksums manifest: %w", err)
	}

	for _, p := range platforms {
		verifyPlatformArchive(opts, manifest, p, res)
	}

	if len(res.Issues) > 0 {
		res.ChecksumsValid = false
		return res, fmt.Errorf("public release payload check failed with %d issue(s): %s", len(res.Issues), strings.Join(res.Issues, "; "))
	}

	res.ChecksumsValid = true
	return res, nil
}

func verifyPlatformArchive(opts PayloadVerificationOptions, manifest map[string]string, p community.TargetPlatform, res *PayloadVerificationResult) {
	expectedArchive := community.FormatArchiveName(opts.Version, p.OS, p.Arch)
	expectedHash, ok := manifest[expectedArchive]
	if !ok {
		zipCandidate := strings.TrimSuffix(expectedArchive, ".tar.gz") + ".zip"
		if zipHash, zipOk := manifest[zipCandidate]; zipOk {
			expectedArchive = zipCandidate
			expectedHash = zipHash
			ok = true
		}
	}

	if !ok {
		res.Issues = append(res.Issues, fmt.Sprintf("missing checksum entry for required platform %s/%s (%s)", p.OS, p.Arch, expectedArchive))
		return
	}

	archivePath := filepath.Join(opts.ReleaseDir, expectedArchive)
	f, err := fileutil.OpenRead(archivePath)
	if err != nil {
		res.Issues = append(res.Issues, fmt.Sprintf("missing archive for platform %s/%s: %v", p.OS, p.Arch, err))
		return
	}

	hasher := sha256.New()
	_, copyErr := io.Copy(hasher, f)
	closeErr := f.Close()
	if copyErr != nil {
		res.Issues = append(res.Issues, fmt.Sprintf("failed to read archive %s: %v", expectedArchive, copyErr))
		return
	}
	if closeErr != nil {
		res.Issues = append(res.Issues, fmt.Sprintf("failed to close archive %s: %v", expectedArchive, closeErr))
		return
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		res.Issues = append(res.Issues, fmt.Sprintf("checksum mismatch for %s: expected %s, computed %s", expectedArchive, expectedHash, actualHash))
		return
	}

	if err := verifyArchiveContents(archivePath, opts); err != nil {
		res.Issues = append(res.Issues, fmt.Sprintf("archive integrity check failed for %s: %v", expectedArchive, err))
		return
	}

	res.VerifiedArchives = append(res.VerifiedArchives, expectedArchive)
	res.PlatformsCovered = append(res.PlatformsCovered, p)
}

func verifyArchiveContents(archivePath string, opts PayloadVerificationOptions) error {
	lower := strings.ToLower(archivePath)
	if strings.HasSuffix(lower, ".zip") {
		return verifyZipContents(archivePath, opts)
	}
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return verifyTarGzContents(archivePath, opts)
	}
	return fmt.Errorf("unsupported archive format: %s", archivePath)
}

func containsSensitivePattern(lower string) bool {
	return strings.Contains(lower, ".env") ||
		strings.Contains(lower, "id_rsa") ||
		strings.Contains(lower, "credentials") ||
		strings.Contains(lower, ".git") ||
		strings.Contains(lower, ".ds_store") ||
		strings.HasSuffix(lower, ".pem") ||
		strings.HasSuffix(lower, ".key")
}

func verifyTarGzContents(archivePath string, opts PayloadVerificationOptions) (retErr error) {
	f, err := fileutil.OpenRead(archivePath)
	if err != nil {
		return err
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && retErr == nil {
			retErr = cErr
		}
	}()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("corrupted gzip stream: %w", err)
	}
	defer func() {
		if cErr := gzr.Close(); cErr != nil && retErr == nil {
			retErr = cErr
		}
	}()

	tr := tar.NewReader(gzr)
	hasLicense := false
	hasBinary := false

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("corrupted tar archive: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(hdr.Name, "/") || strings.Contains(hdr.Name, "../") {
			return fmt.Errorf("insecure path traversal detected in archive entry: %s", hdr.Name)
		}

		if hdr.Typeflag == tar.TypeSymlink {
			if strings.HasPrefix(hdr.Linkname, "/") || strings.Contains(hdr.Linkname, "..") {
				return fmt.Errorf("insecure symlink target in archive: %s -> %s", hdr.Name, hdr.Linkname)
			}
		}

		baseName := filepath.Base(hdr.Name)
		if strings.EqualFold(baseName, "LICENSE") || strings.EqualFold(baseName, "LICENSE.txt") {
			hasLicense = true
		}
		isBinary := strings.HasPrefix(baseName, "zqk") || strings.HasPrefix(baseName, "zcom")
		if isBinary {
			hasBinary = true
			if hdr.Typeflag == tar.TypeReg && (hdr.Mode&0111 == 0) && !strings.HasSuffix(baseName, ".exe") {
				return fmt.Errorf("binary %s is missing executable permissions (mode: %o)", hdr.Name, hdr.Mode)
			}
		}

		if opts.CheckNoSecrets {
			if containsSensitivePattern(strings.ToLower(hdr.Name)) {
				return fmt.Errorf("sensitive artifact found in archive: %s", hdr.Name)
			}
		}
	}

	return validateArchiveBasics(hasBinary, hasLicense, opts.RequireLicense)
}

func verifyZipContents(archivePath string, opts PayloadVerificationOptions) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("corrupted zip archive: %w", err)
	}
	defer func() {
		if cErr := zr.Close(); cErr != nil && err == nil {
			err = cErr
		}
	}()

	hasLicense := false
	hasBinary := false

	for _, f := range zr.File {
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "../") {
			return fmt.Errorf("insecure path traversal detected in archive entry: %s", f.Name)
		}

		baseName := filepath.Base(f.Name)
		if strings.EqualFold(baseName, "LICENSE") || strings.EqualFold(baseName, "LICENSE.txt") {
			hasLicense = true
		}
		isBinary := strings.HasPrefix(baseName, "zqk") || strings.HasPrefix(baseName, "zcom")
		if isBinary {
			hasBinary = true
			if f.Mode()&0111 == 0 && !strings.HasSuffix(baseName, ".exe") {
				return fmt.Errorf("binary %s is missing executable permissions", f.Name)
			}
		}

		if opts.CheckNoSecrets {
			if containsSensitivePattern(strings.ToLower(f.Name)) {
				return fmt.Errorf("sensitive artifact found in archive: %s", f.Name)
			}
		}

		if f.Mode()&os.ModeSymlink != 0 {
			if symErr := inspectZipSymlink(f); symErr != nil {
				return symErr
			}
		}
	}

	return validateArchiveBasics(hasBinary, hasLicense, opts.RequireLicense)
}

func validateArchiveBasics(hasBinary, hasLicense, requireLicense bool) error {
	if !hasBinary {
		return errors.New("archive does not contain the zqk/zcom executable binary")
	}
	if requireLicense && !hasLicense {
		return errors.New("archive is missing required LICENSE file")
	}
	return nil
}

func inspectZipSymlink(f *zip.File) (retErr error) {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open symlink in zip %s: %w", f.Name, err)
	}
	defer func() {
		if cErr := rc.Close(); cErr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close symlink reader: %w", cErr)
		}
	}()

	linkTarget, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read symlink target %s: %w", f.Name, err)
	}

	targetStr := string(linkTarget)
	if strings.HasPrefix(targetStr, "/") || strings.Contains(targetStr, "..") {
		return fmt.Errorf("insecure symlink target in archive: %s -> %s", f.Name, targetStr)
	}
	return nil
}
