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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type testArchiveEntry struct {
	Name     string
	Data     []byte
	Mode     int64
	Typeflag byte
	Linkname string
}

func writeTarGzFixture(t *testing.T, targetPath string, entries []testArchiveEntry) {
	t.Helper()
	f, err := fileutil.Create(targetPath)
	if err != nil {
		t.Fatalf("create tar.gz fixture failed: %v", err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil {
			t.Fatalf("close tar.gz fixture failed: %v", cErr)
		}
	}()

	gw := gzip.NewWriter(f)
	defer func() {
		if cErr := gw.Close(); cErr != nil {
			t.Fatalf("close gzip fixture failed: %v", cErr)
		}
	}()

	tw := tar.NewWriter(gw)
	defer func() {
		if cErr := tw.Close(); cErr != nil {
			t.Fatalf("close tar fixture failed: %v", cErr)
		}
	}()

	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.Name,
			Mode:     e.Mode,
			Size:     int64(len(e.Data)),
			Typeflag: e.Typeflag,
			Linkname: e.Linkname,
		}
		if hdr.Typeflag == 0 {
			hdr.Typeflag = tar.TypeReg
		}
		if hdr.Mode == 0 {
			hdr.Mode = 0755
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header failed for %s: %v", e.Name, err)
		}
		if len(e.Data) > 0 {
			if _, err := tw.Write(e.Data); err != nil {
				t.Fatalf("write tar data failed for %s: %v", e.Name, err)
			}
		}
	}
}

func writeZipFixture(t *testing.T, targetPath string, entries []testArchiveEntry) {
	t.Helper()
	f, err := fileutil.Create(targetPath)
	if err != nil {
		t.Fatalf("create zip fixture failed: %v", err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil {
			t.Fatalf("close zip fixture failed: %v", cErr)
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if cErr := zw.Close(); cErr != nil {
			t.Fatalf("close zip writer failed: %v", cErr)
		}
	}()

	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.Name}
		mode := os.FileMode(e.Mode)
		if mode == 0 {
			mode = 0755
		}
		if e.Typeflag == tar.TypeSymlink {
			mode |= os.ModeSymlink
		}
		fh.SetMode(mode)

		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatalf("create zip header failed for %s: %v", e.Name, err)
		}
		data := e.Data
		if e.Typeflag == tar.TypeSymlink && len(data) == 0 && e.Linkname != "" {
			data = []byte(e.Linkname)
		}
		if len(data) > 0 {
			if _, err := w.Write(data); err != nil {
				t.Fatalf("write zip data failed for %s: %v", e.Name, err)
			}
		}
	}
}

func hashFixture(t *testing.T, targetPath string) string {
	t.Helper()
	f, err := fileutil.OpenRead(targetPath)
	if err != nil {
		t.Fatalf("open fixture for hashing failed: %v", err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil {
			t.Fatalf("close fixture after hashing failed: %v", cErr)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash fixture failed: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func standardValidEntries() []testArchiveEntry {
	return []testArchiveEntry{
		{Name: "zqk", Data: []byte("#!/bin/sh\necho zqk\n"), Mode: 0755},
		{Name: "LICENSE", Data: []byte("Apache License 2.0\n"), Mode: 0644},
	}
}

func writeManifest(t *testing.T, releaseDir string, lines []string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	err := fileutil.WriteFile(filepath.Join(releaseDir, "checksums.txt"), []byte(content), paths.FilePerm644)
	if err != nil {
		t.Fatalf("write checksums.txt failed: %v", err)
	}
}

func TestPublicPayloadChecker_LifecycleAndOptions(t *testing.T) {
	opts := PayloadVerificationOptions{
		ReleaseDir:     "/tmp/sample",
		Version:        "v1.2.3",
		RequireLicense: true,
		CheckNoSecrets: true,
		DryRunOnly:     true,
	}

	checker := NewPublicPayloadChecker(opts)
	if checker == nil {
		t.Fatal("expected non-nil checker")
	}

	retrieved := checker.Options()
	if retrieved.ReleaseDir != opts.ReleaseDir || retrieved.Version != opts.Version {
		t.Errorf("options mismatch: got %+v, want %+v", retrieved, opts)
	}
	if !retrieved.RequireLicense || !retrieved.CheckNoSecrets || !retrieved.DryRunOnly {
		t.Errorf("boolean flags mismatch: %+v", retrieved)
	}
}

func TestVerifyPublicReleasePayload_ConfigurationValidation(t *testing.T) {
	t.Run("EmptyReleaseDirectory", func(t *testing.T) {
		checker := NewPublicPayloadChecker(PayloadVerificationOptions{
			ReleaseDir: "",
			Version:    "v1.0.0",
		})
		_, err := checker.Verify()
		if err == nil || !strings.Contains(err.Error(), "release directory is required") {
			t.Fatalf("expected release directory required error, got: %v", err)
		}
	})

	t.Run("WhitespaceReleaseDirectory", func(t *testing.T) {
		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir: "   \t\n  ",
			Version:    "v1.0.0",
		})
		if err == nil || !strings.Contains(err.Error(), "release directory is required") {
			t.Fatalf("expected release directory required error, got: %v", err)
		}
	})

	t.Run("NonExistentReleaseDirectory", func(t *testing.T) {
		nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir: nonExistent,
			Version:    "v1.0.0",
		})
		if err == nil || !strings.Contains(err.Error(), "release directory inaccessible") {
			t.Fatalf("expected release directory inaccessible error, got: %v", err)
		}
	})

	t.Run("ReleaseDirectoryIsFile", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "regular-file.txt")
		if err := fileutil.WriteFile(tmpFile, []byte("content"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}
		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir: tmpFile,
			Version:    "v1.0.0",
		})
		if err == nil || !strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("expected is not a directory error, got: %v", err)
		}
	})

	t.Run("InvalidReleaseTags", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidTags := []string{"", "1.0.0", "vBeta", "release-1.0.0", "v1.0"}
		for _, tag := range invalidTags {
			_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
				ReleaseDir: tmpDir,
				Version:    tag,
			})
			if err == nil || !strings.Contains(err.Error(), "invalid release tag") {
				t.Errorf("expected invalid release tag error for %q, got: %v", tag, err)
			}
		}
	})
}

func TestVerifyPublicReleasePayload_ChecksumManifestParsing(t *testing.T) {
	t.Run("MissingChecksumsFile", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir: tmpDir,
			Version:    "v1.0.0",
		})
		if err == nil || !strings.Contains(err.Error(), "missing or unreadable checksums.txt") {
			t.Fatalf("expected missing checksums.txt error, got: %v", err)
		}
	})

	t.Run("CorruptedChecksumsManifest", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeManifest(t, tmpDir, []string{
			"badhash-not-64-hex-characters archive.tar.gz",
		})
		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir: tmpDir,
			Version:    "v1.0.0",
		})
		if err == nil || !strings.Contains(err.Error(), "corrupt checksums manifest") {
			t.Fatalf("expected corrupt checksums manifest error, got: %v", err)
		}
	})

	t.Run("ExtraWhitespaceAndCommentsHandledCleanly", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := community.TargetPlatform{OS: "linux", Arch: "amd64"}
		archiveName := community.FormatArchiveName("v1.0.0", p.OS, p.Arch)
		archivePath := filepath.Join(tmpDir, archiveName)
		writeTarGzFixture(t, archivePath, standardValidEntries())
		hash := hashFixture(t, archivePath)

		writeManifest(t, tmpDir, []string{
			"# Header comment",
			"",
			"   # Indented comment",
			"  " + hash + "   " + archiveName + "  ",
			"",
		})

		res, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
			RequireLicense:   true,
			CheckNoSecrets:   true,
		})
		if err != nil {
			t.Fatalf("expected clean pass with comments/whitespace, got: %v", err)
		}
		if !res.ChecksumsValid || len(res.VerifiedArchives) != 1 {
			t.Errorf("unexpected payload result: %+v", res)
		}
	})
}

func TestVerifyPublicReleasePayload_PlatformCoverageAndChecksumMismatch(t *testing.T) {
	t.Run("MissingPlatformInManifest", func(t *testing.T) {
		tmpDir := t.TempDir()
		writeManifest(t, tmpDir, []string{
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef other_archive.tar.gz",
		})

		p := community.TargetPlatform{OS: "linux", Arch: "amd64"}
		res, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
		})
		if err == nil {
			t.Fatal("expected failure due to missing platform in manifest")
		}
		if res == nil || res.ChecksumsValid || len(res.Issues) == 0 {
			t.Fatalf("expected issues recorded in result, got: %+v", res)
		}
		if !strings.Contains(res.Issues[0], "missing checksum entry for required platform") {
			t.Errorf("unexpected issue message: %s", res.Issues[0])
		}
	})

	t.Run("MissingArchiveFileOnDisk", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := community.TargetPlatform{OS: "linux", Arch: "amd64"}
		archiveName := community.FormatArchiveName("v1.0.0", p.OS, p.Arch)
		writeManifest(t, tmpDir, []string{
			"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef " + archiveName,
		})

		res, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
		})
		if err == nil {
			t.Fatal("expected failure due to missing archive file on disk")
		}
		if res == nil || len(res.Issues) == 0 || !strings.Contains(res.Issues[0], "missing archive for platform") {
			t.Fatalf("unexpected issues: %+v", res)
		}
	})

	t.Run("ChecksumMismatchDetected", func(t *testing.T) {
		tmpDir := t.TempDir()
		p := community.TargetPlatform{OS: "darwin", Arch: "arm64"}
		archiveName := community.FormatArchiveName("v1.0.0", p.OS, p.Arch)
		archivePath := filepath.Join(tmpDir, archiveName)
		writeTarGzFixture(t, archivePath, standardValidEntries())

		writeManifest(t, tmpDir, []string{
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa " + archiveName,
		})

		res, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
		})
		if err == nil {
			t.Fatal("expected failure due to checksum mismatch")
		}
		if res == nil || len(res.Issues) == 0 || !strings.Contains(res.Issues[0], "checksum mismatch for") {
			t.Fatalf("expected checksum mismatch issue, got: %+v", res)
		}
	})
}

func TestVerifyPublicReleasePayload_TarGzIntegrityChecks(t *testing.T) {
	p := community.TargetPlatform{OS: "linux", Arch: "amd64"}
	archiveName := community.FormatArchiveName("v2.1.0", p.OS, p.Arch)

	runPayloadTest := func(t *testing.T, entries []testArchiveEntry, optsModifier func(o *PayloadVerificationOptions)) (*PayloadVerificationResult, error) {
		t.Helper()
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, archiveName)
		writeTarGzFixture(t, archivePath, entries)
		hash := hashFixture(t, archivePath)
		writeManifest(t, tmpDir, []string{hash + " " + archiveName})

		opts := PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v2.1.0",
			RequirePlatforms: []community.TargetPlatform{p},
			RequireLicense:   true,
			CheckNoSecrets:   true,
		}
		if optsModifier != nil {
			optsModifier(&opts)
		}
		return VerifyPublicReleasePayload(opts)
	}

	t.Run("ValidTarGzPasses", func(t *testing.T) {
		res, err := runPayloadTest(t, standardValidEntries(), nil)
		if err != nil {
			t.Fatalf("expected valid tar.gz to pass, got: %v", err)
		}
		if !res.ChecksumsValid || len(res.VerifiedArchives) != 1 || len(res.PlatformsCovered) != 1 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("ArchiveWithZcomBinaryPasses", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zcom", Data: []byte("#!/bin/sh\necho zcom\n"), Mode: 0755},
			{Name: "LICENSE.txt", Data: []byte("License content"), Mode: 0644},
		}
		res, err := runPayloadTest(t, entries, nil)
		if err != nil {
			t.Fatalf("expected zcom binary to pass, got: %v", err)
		}
		if !res.ChecksumsValid {
			t.Fatalf("expected checksums valid: %+v", res)
		}
	})

	t.Run("MissingBinaryExecutableFails", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "other_tool", Data: []byte("echo hi"), Mode: 0755},
			{Name: "LICENSE", Data: []byte("License"), Mode: 0644},
		}
		_, err := runPayloadTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "does not contain the zqk/zcom executable binary") {
			t.Fatalf("expected missing binary error, got: %v", err)
		}
	})

	t.Run("BinaryMissingExecutableBitFails", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk", Data: []byte("echo hi"), Mode: 0644},
			{Name: "LICENSE", Data: []byte("License"), Mode: 0644},
		}
		_, err := runPayloadTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "missing executable permissions") {
			t.Fatalf("expected missing executable permissions error, got: %v", err)
		}
	})

	t.Run("MissingLicenseWhenRequiredFails", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk", Data: []byte("echo hi"), Mode: 0755},
		}
		_, err := runPayloadTest(t, entries, func(o *PayloadVerificationOptions) {
			o.RequireLicense = true
		})
		if err == nil || !strings.Contains(err.Error(), "missing required LICENSE file") {
			t.Fatalf("expected missing LICENSE error, got: %v", err)
		}
	})

	t.Run("MissingLicenseWhenNotRequiredPasses", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk", Data: []byte("echo hi"), Mode: 0755},
		}
		res, err := runPayloadTest(t, entries, func(o *PayloadVerificationOptions) {
			o.RequireLicense = false
		})
		if err != nil {
			t.Fatalf("expected pass when RequireLicense is false, got: %v", err)
		}
		if !res.ChecksumsValid {
			t.Errorf("expected checksums valid: %+v", res)
		}
	})
}

func TestVerifyPublicReleasePayload_SecurityAndAdversarialRejections(t *testing.T) {
	p := community.TargetPlatform{OS: "linux", Arch: "arm64"}
	archiveName := community.FormatArchiveName("v1.5.0", p.OS, p.Arch)

	verifyAdversarialTar := func(t *testing.T, entries []testArchiveEntry, expectedErrSubstr string) {
		t.Helper()
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, archiveName)
		writeTarGzFixture(t, archivePath, entries)
		hash := hashFixture(t, archivePath)
		writeManifest(t, tmpDir, []string{hash + " " + archiveName})

		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.5.0",
			RequirePlatforms: []community.TargetPlatform{p},
			RequireLicense:   true,
			CheckNoSecrets:   true,
		})
		if err == nil || !strings.Contains(err.Error(), expectedErrSubstr) {
			t.Fatalf("expected error containing %q, got: %v", expectedErrSubstr, err)
		}
	}

	t.Run("DetectsSecretEnvFile", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: ".env.production",
			Data: []byte("API_SECRET=supersecret\n"),
			Mode: 0600,
		})
		verifyAdversarialTar(t, entries, "sensitive artifact found in archive")
	})

	t.Run("DetectsSSHPrivateKey", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: "keys/id_rsa",
			Data: []byte("mock-rsa-key-content\n"),
			Mode: 0600,
		})
		verifyAdversarialTar(t, entries, "sensitive artifact found in archive")
	})

	t.Run("DetectsCredentialsFile", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: "config/credentials.json",
			Data: []byte(`{"token": "xyz"}`),
			Mode: 0600,
		})
		verifyAdversarialTar(t, entries, "sensitive artifact found in archive")
	})

	t.Run("DetectsGitDirectory", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: ".git/HEAD",
			Data: []byte("ref: refs/heads/main\n"),
			Mode: 0644,
		})
		verifyAdversarialTar(t, entries, "sensitive artifact found in archive")
	})

	t.Run("DetectsDSStoreArtifact", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: "dir/.DS_Store",
			Data: []byte{0x00, 0x00, 0x00, 0x01},
			Mode: 0644,
		})
		verifyAdversarialTar(t, entries, "sensitive artifact found in archive")
	})

	t.Run("DetectsPrivateKeyExtensions", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: "certs/server.pem",
			Data: []byte("-----BEGIN CERTIFICATE-----\n"),
			Mode: 0600,
		})
		verifyAdversarialTar(t, entries, "sensitive artifact found in archive")
	})

	t.Run("DetectsPathTraversalRelative", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: "../../etc/passwd",
			Data: []byte("root:x:0:0:::\n"),
			Mode: 0644,
		})
		verifyAdversarialTar(t, entries, "insecure path traversal detected")
	})

	t.Run("DetectsPathTraversalAbsolute", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name: "/tmp/exploit",
			Data: []byte("malicious"),
			Mode: 0755,
		})
		verifyAdversarialTar(t, entries, "insecure path traversal detected")
	})

	t.Run("DetectsInsecureSymlinkTargetAbsolute", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name:     "zqk_link",
			Typeflag: tar.TypeSymlink,
			Linkname: "/etc/shadow",
		})
		verifyAdversarialTar(t, entries, "insecure symlink target in archive")
	})

	t.Run("DetectsInsecureSymlinkTargetRelativeEscape", func(t *testing.T) {
		entries := append(standardValidEntries(), testArchiveEntry{
			Name:     "zqk_link",
			Typeflag: tar.TypeSymlink,
			Linkname: "../../../root/.ssh/id_rsa",
		})
		verifyAdversarialTar(t, entries, "insecure symlink target in archive")
	})
}

func TestVerifyPublicReleasePayload_ZipArchiveSupport(t *testing.T) {
	p := community.TargetPlatform{OS: "windows", Arch: "amd64"}
	zipArchiveName := "zqk-community_3.0.0_windows_amd64.zip"

	runZipTest := func(t *testing.T, entries []testArchiveEntry, optsModifier func(o *PayloadVerificationOptions)) (*PayloadVerificationResult, error) {
		t.Helper()
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, zipArchiveName)
		writeZipFixture(t, archivePath, entries)
		hash := hashFixture(t, archivePath)
		writeManifest(t, tmpDir, []string{hash + " " + zipArchiveName})

		opts := PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v3.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
			RequireLicense:   true,
			CheckNoSecrets:   true,
		}
		if optsModifier != nil {
			optsModifier(&opts)
		}
		return VerifyPublicReleasePayload(opts)
	}

	t.Run("ValidZipArchivePasses", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk.exe", Data: []byte("binary data"), Mode: 0755},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
		}
		res, err := runZipTest(t, entries, nil)
		if err != nil {
			t.Fatalf("expected valid zip archive to pass, got: %v", err)
		}
		if !res.ChecksumsValid || len(res.VerifiedArchives) != 1 {
			t.Errorf("unexpected zip result: %+v", res)
		}
	})

	t.Run("ZipMissingBinaryFails", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "readme.txt", Data: []byte("readme"), Mode: 0644},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
		}
		_, err := runZipTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "does not contain the zqk/zcom executable binary") {
			t.Fatalf("expected missing binary error, got: %v", err)
		}
	})

	t.Run("ZipMissingLicenseWhenRequiredFails", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk.exe", Data: []byte("binary data"), Mode: 0755},
		}
		_, err := runZipTest(t, entries, func(o *PayloadVerificationOptions) {
			o.RequireLicense = true
		})
		if err == nil || !strings.Contains(err.Error(), "missing required LICENSE file") {
			t.Fatalf("expected missing LICENSE error, got: %v", err)
		}
	})

	t.Run("ZipSensitiveArtifactRejection", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk.exe", Data: []byte("binary data"), Mode: 0755},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
			{Name: "secrets/.env", Data: []byte("SECRET=1"), Mode: 0600},
		}
		_, err := runZipTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "sensitive artifact found in archive") {
			t.Fatalf("expected sensitive artifact error, got: %v", err)
		}
	})

	t.Run("ZipPathTraversalRejection", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "../../windows/system32/cmd.exe", Data: []byte("exploit"), Mode: 0755},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
		}
		_, err := runZipTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "insecure path traversal detected") {
			t.Fatalf("expected path traversal error, got: %v", err)
		}
	})

	t.Run("ZipBinaryMissingExecutableBitFails", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk", Data: []byte("binary data"), Mode: 0644},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
		}
		_, err := runZipTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "missing executable permissions") {
			t.Fatalf("expected missing executable permissions error, got: %v", err)
		}
	})

	t.Run("ZipSafeSymlinkPasses", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk.exe", Data: []byte("binary data"), Mode: 0755},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
			{Name: "bin_link", Linkname: "zqk.exe", Typeflag: tar.TypeSymlink},
		}
		res, err := runZipTest(t, entries, nil)
		if err != nil {
			t.Fatalf("expected safe symlink in zip to pass, got: %v", err)
		}
		if !res.ChecksumsValid {
			t.Fatalf("expected checksums valid: %+v", res)
		}
	})

	t.Run("ZipInsecureSymlinkRejection", func(t *testing.T) {
		entries := []testArchiveEntry{
			{Name: "zqk.exe", Data: []byte("binary data"), Mode: 0755},
			{Name: "LICENSE", Data: []byte("License text"), Mode: 0644},
			{Name: "bad_symlink", Linkname: "/etc/passwd", Typeflag: tar.TypeSymlink},
		}
		_, err := runZipTest(t, entries, nil)
		if err == nil || !strings.Contains(err.Error(), "insecure symlink target in archive") {
			t.Fatalf("expected insecure symlink error, got: %v", err)
		}
	})
}

func TestVerifyPublicReleasePayload_CorruptedArchivesAndExtensions(t *testing.T) {
	p := community.TargetPlatform{OS: "linux", Arch: "amd64"}
	archiveName := community.FormatArchiveName("v1.0.0", p.OS, p.Arch)

	t.Run("CorruptedGzipArchive", func(t *testing.T) {
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, archiveName)
		if err := fileutil.WriteFile(archivePath, []byte("NOT_A_VALID_GZIP_STREAM"), paths.FilePerm644); err != nil {
			t.Fatalf("write fake archive failed: %v", err)
		}
		hash := hashFixture(t, archivePath)
		writeManifest(t, tmpDir, []string{hash + " " + archiveName})

		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
		})
		if err == nil || !strings.Contains(err.Error(), "corrupted gzip stream") {
			t.Fatalf("expected corrupted gzip stream error, got: %v", err)
		}
	})

	t.Run("CorruptedTarArchiveAfterValidGzip", func(t *testing.T) {
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, archiveName)

		f, err := fileutil.Create(archivePath)
		if err != nil {
			t.Fatalf("failed to create archive: %v", err)
		}
		gw := gzip.NewWriter(f)
		if _, err := gw.Write([]byte("random tar bytes not conforming to tar format")); err != nil {
			t.Fatalf("failed to write gzip payload: %v", err)
		}
		if err := gw.Close(); err != nil {
			t.Fatalf("failed to close gzip writer: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("failed to close file: %v", err)
		}

		hash := hashFixture(t, archivePath)
		writeManifest(t, tmpDir, []string{hash + " " + archiveName})

		_, err = VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{p},
		})
		if err == nil || !strings.Contains(err.Error(), "corrupted tar archive") {
			t.Fatalf("expected corrupted tar archive error, got: %v", err)
		}
	})

	t.Run("CorruptedZipArchive", func(t *testing.T) {
		tmpDir := t.TempDir()
		zipName := "zqk-community_1.0.0_windows_amd64.zip"
		archivePath := filepath.Join(tmpDir, zipName)
		if err := fileutil.WriteFile(archivePath, []byte("NOT_A_ZIP_HEADER"), paths.FilePerm644); err != nil {
			t.Fatalf("write fake zip failed: %v", err)
		}
		hash := hashFixture(t, archivePath)
		writeManifest(t, tmpDir, []string{hash + " " + zipName})

		pWin := community.TargetPlatform{OS: "windows", Arch: "amd64"}
		_, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
			ReleaseDir:       tmpDir,
			Version:          "v1.0.0",
			RequirePlatforms: []community.TargetPlatform{pWin},
		})
		if err == nil || !strings.Contains(err.Error(), "corrupted zip archive") {
			t.Fatalf("expected corrupted zip archive error, got: %v", err)
		}
	})

	t.Run("NonExistentArchiveDirect", func(t *testing.T) {
		err := verifyArchiveContents(filepath.Join(t.TempDir(), "missing.tar.gz"), PayloadVerificationOptions{})
		if err == nil {
			t.Fatal("expected error for missing tar.gz file")
		}
		errZip := verifyArchiveContents(filepath.Join(t.TempDir(), "missing.zip"), PayloadVerificationOptions{})
		if errZip == nil {
			t.Fatal("expected error for missing zip file")
		}
	})

	t.Run("UnsupportedArchiveExtensionDirect", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "archive.rar")
		if err := fileutil.WriteFile(tmpFile, []byte("rar content"), paths.FilePerm644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
		err := verifyArchiveContents(tmpFile, PayloadVerificationOptions{})
		if err == nil || !strings.Contains(err.Error(), "unsupported archive format") {
			t.Fatalf("expected unsupported archive format error, got: %v", err)
		}
	})
}

func TestVerifyPublicReleasePayload_DefaultPlatformsMatrix(t *testing.T) {
	tmpDir := t.TempDir()
	version := "v4.0.0"
	var manifestLines []string

	for _, p := range community.SupportedPlatforms {
		archiveName := community.FormatArchiveName(version, p.OS, p.Arch)
		archivePath := filepath.Join(tmpDir, archiveName)
		writeTarGzFixture(t, archivePath, standardValidEntries())
		h := hashFixture(t, archivePath)
		manifestLines = append(manifestLines, fmt.Sprintf("%s  %s", h, archiveName))
	}
	writeManifest(t, tmpDir, manifestLines)

	res, err := VerifyPublicReleasePayload(PayloadVerificationOptions{
		ReleaseDir:     tmpDir,
		Version:        version,
		RequireLicense: true,
		CheckNoSecrets: true,
	})
	if err != nil {
		t.Fatalf("expected all default supported platforms to pass, got: %v", err)
	}
	if !res.ChecksumsValid {
		t.Fatal("expected checksums valid")
	}
	if len(res.PlatformsCovered) != len(community.SupportedPlatforms) {
		t.Errorf("expected %d platforms covered, got %d", len(community.SupportedPlatforms), len(res.PlatformsCovered))
	}
	if len(res.VerifiedArchives) != len(community.SupportedPlatforms) {
		t.Errorf("expected %d verified archives, got %d", len(community.SupportedPlatforms), len(res.VerifiedArchives))
	}
}
