package authcred

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// SeatingCredentialsDir is the relative path under project root for seat secrets.
// Plaintext is local-only (gitignored); CAS stores fingerprints only.
// TRACK: move to sealed vault when available.
const SeatingCredentialsDir = "seating/credentials" //nolint:gosec

// SeatCredentialPath returns .zqk/seating/credentials/<account_id>.
func SeatCredentialPath(projectRoot, accountID string) string {
	safe := strings.ReplaceAll(strings.TrimSpace(accountID), string(fileutil.PathSeparator), "_")
	return filepath.Join(projectRoot, paths.ProjectDataDir, SeatingCredentialsDir, safe)
}

// WriteSeatCredential stores the issued secret for orchestrate/sync-loop injection (0600).
func WriteSeatCredential(projectRoot, accountID, secret string) error {
	path := SeatCredentialPath(projectRoot, accountID)
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		return errfmt.Newf("create seating credentials dir").Wrap(err)
	}
	if err := fileutil.WriteSecureFile(path, []byte(strings.TrimSpace(secret)+"\n")); err != nil {
		return errfmt.Newf("write seating credential").Wrap(err)
	}
	return nil
}

// LoadSeatCredential returns the seating secret for accountID, or empty if missing.
func LoadSeatCredential(projectRoot, accountID string) (string, error) {
	path := SeatCredentialPath(projectRoot, accountID)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return "", nil
		}
		return "", errfmt.Newf("read seating credential").Wrap(err)
	}
	return strings.TrimSpace(string(data)), nil
}

// APIKeyForSeat prefers the issued seating secret; falls back to ACC-* id (transitional).
func APIKeyForSeat(projectRoot, accountID string) string {
	if secret, err := LoadSeatCredential(projectRoot, accountID); err == nil && secret != "" {
		return secret
	}
	return strings.TrimSpace(accountID)
}

// WithSeatAPIKeyEnv copies a bounded subset of parent env, replaces ZQK_API_KEY with seatKey,
// and sets ZQK_PROJECT_ROOT when non-empty. projectRootEnv must be the seated kernel root,
// never an ATK git worktree (POL-AGENT-KERNEL-ROOT-BINDING-001).
func WithSeatAPIKeyEnv(parent []string, seatKey, projectRootEnv string) []string {
	keyName := zqkenv.APIKey()
	out := make([]string, 0, len(parent)+2)
	for _, e := range parent {
		if strings.HasPrefix(e, keyName.Name()+"=") {
			continue
		}
		if projectRootEnv != "" && strings.HasPrefix(e, zqkenv.ProjectRoot().Name()+"=") {
			continue
		}
		// Bound environment to prevent dumping parent secrets into untrusted agent seat
		if strings.HasPrefix(e, zqkenv.OSPath().Name()+"=") || strings.HasPrefix(e, zqkenv.OSHome().Name()+"=") || strings.HasPrefix(e, "USER=") || zqkenv.IsProductPrefixed(e) {
			out = append(out, e)
		}
	}
	if seatKey != "" {
		out = append(out, keyName.Name()+"="+seatKey)
	}
	if projectRootEnv != "" {
		out = append(out, zqkenv.ProjectRoot().Name()+"="+projectRootEnv)
	}
	return out
}
