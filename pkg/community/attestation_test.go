package community_test

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CRIT-1789717628518406000-14ef1e6b: Functional Acceptance
// Verifies SHA-256 and SHA-512 checksum digest calculation, checksum file format, and verification.
func TestAttestation_FunctionalAcceptance(t *testing.T) {
	tempDir := t.TempDir()
	sample1 := filepath.Join(tempDir, "release-v1.0.0-darwin-arm64.tar.gz")
	sample2 := filepath.Join(tempDir, "release-v1.0.0-linux-amd64.tar.gz")

	content1 := []byte("binary payload apple silicon release asset")
	content2 := []byte("binary payload linux x86_64 release asset")

	require.NoError(t, fileutil.WriteStandardFile(sample1, content1))
	require.NoError(t, fileutil.WriteStandardFile(sample2, content2))

	// SHA256 test
	manifest256, err := community.GenerateChecksumManifest([]string{sample1, sample2}, community.HashSHA256)
	require.NoError(t, err)
	assert.Len(t, manifest256.Records, 2)

	// Validate actual expected hash
	expectedSha256_1 := sha256.Sum256(content1)
	expectedSha256_1_str := hex.EncodeToString(expectedSha256_1[:])
	assert.Equal(t, expectedSha256_1_str, manifest256.Records[0].Hash)
	assert.Equal(t, "release-v1.0.0-darwin-arm64.tar.gz", manifest256.Records[0].Filename)

	// Verify file against checksum
	match, err := community.VerifyFileAgainstChecksum(sample1, expectedSha256_1_str, community.HashSHA256)
	require.NoError(t, err)
	assert.True(t, match)

	// Format checksum file
	formatted, err := community.FormatChecksumFile(manifest256)
	require.NoError(t, err)
	assert.Contains(t, formatted, expectedSha256_1_str+"  release-v1.0.0-darwin-arm64.tar.gz")

	// Parse checksum file back
	parsed, err := community.ParseChecksumFile(strings.NewReader(formatted), community.HashSHA256)
	require.NoError(t, err)
	assert.Len(t, parsed.Records, 2)
	assert.Equal(t, expectedSha256_1_str, parsed.Records[0].Hash)
	assert.Equal(t, community.HashSHA256, parsed.Records[0].Algorithm)

	// SHA512 test
	manifest512, err := community.GenerateChecksumManifest([]string{sample1}, community.HashSHA512)
	require.NoError(t, err)
	expectedSha512_1 := sha512.Sum512(content1)
	expectedSha512_1_str := hex.EncodeToString(expectedSha512_1[:])
	assert.Equal(t, expectedSha512_1_str, manifest512.Records[0].Hash)

	match512, err := community.VerifyFileAgainstChecksum(sample1, expectedSha512_1_str, community.HashSHA512)
	require.NoError(t, err)
	assert.True(t, match512)
}

// CRIT-1789717628518407000-12b7759d: Boundary & Error Handling
// Verifies handling of nil readers, unsupported algorithms, missing files, and corrupted checksum files.
func TestAttestation_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("nil_reader_compute_hash", func(t *testing.T) {
		_, err := community.ComputeHash(nil, community.HashSHA256)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "input reader cannot be nil")
	})

	t.Run("unsupported_hash_algorithm", func(t *testing.T) {
		_, err := community.ComputeHash(bytes.NewReader([]byte("test")), community.HashAlgorithm("md5"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported hash algorithm")
	})

	t.Run("empty_file_path_hash", func(t *testing.T) {
		_, err := community.ComputeFileHash("", community.HashSHA256)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file path cannot be empty")
	})

	t.Run("non_existent_file_hash", func(t *testing.T) {
		_, err := community.ComputeFileHash("/non/existent/path/for/zqk/test", community.HashSHA256)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot open file")
	})

	t.Run("empty_paths_manifest", func(t *testing.T) {
		_, err := community.GenerateChecksumManifest([]string{}, community.HashSHA256)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no file paths provided")
	})

	t.Run("nil_manifest_format", func(t *testing.T) {
		_, err := community.FormatChecksumFile(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "manifest cannot be nil")
	})

	t.Run("nil_reader_parse_checksum", func(t *testing.T) {
		_, err := community.ParseChecksumFile(nil, community.HashSHA256)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "input reader cannot be nil")
	})

	t.Run("malformed_checksum_entry", func(t *testing.T) {
		input := "justaverylongtokenwithnofilename\n"
		_, err := community.ParseChecksumFile(strings.NewReader(input), community.HashSHA256)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "malformed checksum entry")
	})

	t.Run("mismatched_checksum_verification", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "target.bin")
		require.NoError(t, fileutil.WriteStandardFile(tempFile, []byte("data")))
		match, err := community.VerifyFileAgainstChecksum(tempFile, "badhash0000000000000000000000000000000000000000000000000000000000", community.HashSHA256)
		require.NoError(t, err)
		assert.False(t, match)
	})
}

// CRIT-1789717628518408000-46dee27c: Integration & Conformance
// Verifies stream hashing, binary mode indicator support, comments, and empty lines.
func TestAttestation_IntegrationAndConformance(t *testing.T) {
	t.Run("binary_mode_indicator_and_comments", func(t *testing.T) {
		rawManifest := `
# Official release checksums
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 *empty.tar.gz

# Next record
cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e  asset.zip
`
		manifest, err := community.ParseChecksumFile(strings.NewReader(rawManifest), community.HashSHA256)
		require.NoError(t, err)
		require.Len(t, manifest.Records, 2)

		assert.Equal(t, "empty.tar.gz", manifest.Records[0].Filename)
		assert.Equal(t, community.HashSHA256, manifest.Records[0].Algorithm)

		assert.Equal(t, "asset.zip", manifest.Records[1].Filename)
		assert.Equal(t, community.HashSHA512, manifest.Records[1].Algorithm)
	})
}
