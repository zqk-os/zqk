package community

import (
	"bufio"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// HashAlgorithm represents supported cryptographic hashing algorithms for release assets.
type HashAlgorithm string

const (
	// HashSHA256 represents SHA-256 digest algorithm.
	HashSHA256 HashAlgorithm = "sha256"
	// HashSHA512 represents SHA-512 digest algorithm.
	HashSHA512 HashAlgorithm = "sha512"
)

// ChecksumRecord represents a single file checksum record in an attestation manifest.
type ChecksumRecord struct {
	Algorithm HashAlgorithm `json:"algorithm"`
	Hash      string        `json:"hash"`
	Filename  string        `json:"filename"`
}

// AttestationManifest represents an aggregation of verified release asset checksums.
type AttestationManifest struct {
	Records []ChecksumRecord `json:"records"`
}

// ComputeHash computes the hexadecimal hash of the reader content using the given algorithm.
func ComputeHash(r io.Reader, algo HashAlgorithm) (string, error) {
	if r == nil {
		return "", fmt.Errorf("input reader cannot be nil")
	}

	var h hash.Hash
	switch algo {
	case HashSHA256:
		h = sha256.New()
	case HashSHA512:
		h = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm %q (supported: sha256, sha512)", algo)
	}

	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("failed to read stream for hashing: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeFileHash computes the cryptographic hash for a file at path.
func ComputeFileHash(path string, algo HashAlgorithm) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("file path cannot be empty")
	}
	f, err := fileutil.OpenRead(path)
	if err != nil {
		return "", fmt.Errorf("cannot open file %q: %w", path, err)
	}
	defer f.Close()

	return ComputeHash(f, algo)
}

// GenerateChecksumManifest generates an attestation manifest for a list of file paths.
func GenerateChecksumManifest(paths []string, algo HashAlgorithm) (*AttestationManifest, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("no file paths provided")
	}

	manifest := &AttestationManifest{
		Records: make([]ChecksumRecord, 0, len(paths)),
	}

	for _, p := range paths {
		h, err := ComputeFileHash(p, algo)
		if err != nil {
			return nil, err
		}
		manifest.Records = append(manifest.Records, ChecksumRecord{
			Algorithm: algo,
			Hash:      h,
			Filename:  filepath.Base(p),
		})
	}

	return manifest, nil
}

// FormatChecksumFile formats records in standard sha256sum / sha512sum output format (<hash>  <filename>).
func FormatChecksumFile(manifest *AttestationManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("manifest cannot be nil")
	}
	var sb strings.Builder
	for _, rec := range manifest.Records {
		sb.WriteString(fmt.Sprintf("%s  %s\n", rec.Hash, rec.Filename))
	}
	return sb.String(), nil
}

// ParseChecksumFile parses a standard sha256sum or sha512sum formatted checksum file.
func ParseChecksumFile(r io.Reader, defaultAlgo HashAlgorithm) (*AttestationManifest, error) {
	if r == nil {
		return nil, fmt.Errorf("input reader cannot be nil")
	}
	scanner := bufio.NewScanner(r)
	manifest := &AttestationManifest{}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("malformed checksum entry on line %d: %q", lineNum, line)
		}

		hashVal := strings.ToLower(fields[0])
		filename := strings.TrimPrefix(fields[1], "*") // Remove binary mode indicator if present

		algo := defaultAlgo
		if len(hashVal) == 64 {
			algo = HashSHA256
		} else if len(hashVal) == 128 {
			algo = HashSHA512
		}

		manifest.Records = append(manifest.Records, ChecksumRecord{
			Algorithm: algo,
			Hash:      hashVal,
			Filename:  filename,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading checksum stream: %w", err)
	}

	return manifest, nil
}

// VerifyFileAgainstChecksum verifies if a target file matches expected hash.
func VerifyFileAgainstChecksum(filePath, expectedHash string, algo HashAlgorithm) (bool, error) {
	actual, err := ComputeFileHash(filePath, algo)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(actual, strings.TrimSpace(expectedHash)), nil
}
