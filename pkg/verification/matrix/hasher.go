package matrix

import (
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ComputeFileHash computes the SHA-256 hex string for the file at the given path.
func ComputeFileHash(path string) (string, error) {
	f, err := fileutil.OpenRead(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ComputeBytesHash computes the SHA-256 hex string for an in-memory byte slice.
func ComputeBytesHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
