package releasegate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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

	// 1. Locate and parse checksums.txt
	checksumPath := filepath.Join(opts.ReleaseDir, "checksums.txt")
	checksumData, err := fileutil.ReadFile(checksumPath)
	if err != nil {
		return nil, fmt.Errorf("missing or unreadable checksums.txt in %s: %w", opts.ReleaseDir, err)
	}

	manifest, err := community.ParseChecksumManifest(string(checksumData))
	if err != nil {
		return nil, fmt.Errorf("corrupt checksums manifest: %w", err)
	}

	// 2. Verify all required platforms are covered
	for _, p := range platforms {
		expectedArchive := community.FormatArchiveName(opts.Version, p.OS, p.Arch)
		expectedHash, ok := manifest[expectedArchive]
		if !ok {
			errMsg := fmt.Sprintf("missing checksum entry for required platform %s/%s (%s)", p.OS, p.Arch, expectedArchive)
			res.Issues = append(res.Issues, errMsg)
			continue
		}

		archivePath := filepath.Join(opts.ReleaseDir, expectedArchive)
		f, err := fileutil.OpenRead(archivePath)
		if err != nil {
			errMsg := fmt.Sprintf("missing archive for platform %s/%s: %v", p.OS, p.Arch, err)
			res.Issues = append(res.Issues, errMsg)
			continue
		}

		hasher := sha256.New()
		_, copyErr := io.Copy(hasher, f)
		_ = f.Close()
		if copyErr != nil {
			errMsg := fmt.Sprintf("failed to read archive %s: %v", expectedArchive, copyErr)
			res.Issues = append(res.Issues, errMsg)
			continue
		}

		actualHash := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(actualHash, expectedHash) {
			errMsg := fmt.Sprintf("checksum mismatch for %s: expected %s, computed %s", expectedArchive, expectedHash, actualHash)
			res.Issues = append(res.Issues, errMsg)
			continue
		}

		// 3. Inspect archive contents for hermetic safety (license check & secret scan)
		if err := verifyArchiveContents(archivePath, opts); err != nil {
			res.Issues = append(res.Issues, fmt.Sprintf("archive integrity check failed for %s: %v", expectedArchive, err))
			continue
		}

		res.VerifiedArchives = append(res.VerifiedArchives, expectedArchive)
		res.PlatformsCovered = append(res.PlatformsCovered, p)
	}

	if len(res.Issues) > 0 {
		res.ChecksumsValid = false
		return res, fmt.Errorf("public release payload check failed with %d issue(s): %s", len(res.Issues), strings.Join(res.Issues, "; "))
	}

	res.ChecksumsValid = true
	return res, nil
}

// verifyArchiveContents inspects entries inside the tar.gz to verify required contents and absence of leaks.
func verifyArchiveContents(archivePath string, opts PayloadVerificationOptions) error {
	f, err := fileutil.OpenRead(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("corrupted gzip stream: %w", err)
	}
	defer func() { _ = gzr.Close() }()

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

		baseName := filepath.Base(hdr.Name)
		if strings.EqualFold(baseName, "LICENSE") || strings.EqualFold(baseName, "LICENSE.txt") {
			hasLicense = true
		}
		if strings.HasPrefix(baseName, "zqk") || strings.HasPrefix(baseName, "zcom") {
			hasBinary = true
		}

		// Check for forbidden secret patterns in archive entry names
		if opts.CheckNoSecrets {
			lower := strings.ToLower(hdr.Name)
			if strings.Contains(lower, ".env") || strings.Contains(lower, "id_rsa") || strings.Contains(lower, "credentials") {
				return fmt.Errorf("sensitive artifact found in archive: %s", hdr.Name)
			}
		}
	}

	if !hasBinary {
		return errors.New("archive does not contain the zqk/zcom executable binary")
	}
	if opts.RequireLicense && !hasLicense {
		return errors.New("archive is missing required LICENSE file")
	}

	return nil
}
