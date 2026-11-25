// Plane: core
// Cold episodic pages — durable store for facts evicted from the hot window.
//
// TRACK: https://github.com/zqk-os/zqk/pull/1215 — remove when: cold episodic MCP ships.
package sessionpaging

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// ColdSubdir holds cold episodic pages under project state.
	ColdSubdir = "session_cold"
)

var (
	// ErrEmptyEpisodeID is returned when episode id is missing.
	ErrEmptyEpisodeID = errors.New("sessionpaging: episode id is required")
	// ErrInvalidEpisodeID is returned when episode id has unsafe path characters.
	ErrInvalidEpisodeID = errors.New("sessionpaging: episode id must be a single path segment")
	// ErrEpisodeNotFound is returned when a cold episode is missing.
	ErrEpisodeNotFound = errors.New("sessionpaging: cold episode not found")
)

// ColdStore persists evicted episodic facts under .zqk/state/session_cold.
type ColdStore struct {
	projectRoot string
}

// NewColdStore returns a cold episodic store rooted at projectRoot.
func NewColdStore(projectRoot string) (*ColdStore, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, ErrEmptyProjectRoot
	}
	return &ColdStore{projectRoot: root}, nil
}

// Put writes a cold episode for sessionID.
func (c *ColdStore) Put(sessionID, episodeID string, payload []byte) error {
	if c == nil {
		return ErrEmptyProjectRoot
	}
	if err := validateEpisodeIDs(sessionID, episodeID); err != nil {
		return err
	}
	path, err := c.episodePath(sessionID, episodeID)
	if err != nil {
		return err
	}
	return fileutil.WriteDurableSecureFile(path, payload)
}

// Recall reads a cold episode. Missing episodes return ErrEpisodeNotFound.
func (c *ColdStore) Recall(sessionID, episodeID string) ([]byte, error) {
	if c == nil {
		return nil, ErrEmptyProjectRoot
	}
	if err := validateEpisodeIDs(sessionID, episodeID); err != nil {
		return nil, err
	}
	path, err := c.episodePath(sessionID, episodeID)
	if err != nil {
		return nil, err
	}
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrEpisodeNotFound
		}
		return nil, err
	}
	return data, nil
}

// EvictHotToCold moves a hot page into cold storage and removes the hot page.
// Returns the cold payload on success.
func EvictHotToCold(hot *Store, cold *ColdStore, sessionID, pageID string) ([]byte, error) {
	if hot == nil || cold == nil {
		return nil, ErrEmptyProjectRoot
	}
	payload, err := hot.Get(sessionID, pageID)
	if err != nil {
		return nil, err
	}
	if err := cold.Put(sessionID, pageID, payload); err != nil {
		return nil, err
	}
	path, err := hot.pagePath(sessionID, pageID)
	if err != nil {
		return payload, nil
	}
	_ = os.Remove(path)
	return payload, nil
}

func validateEpisodeIDs(sessionID, episodeID string) error {
	sessionID = strings.TrimSpace(sessionID)
	episodeID = strings.TrimSpace(episodeID)
	if sessionID == "" {
		return ErrEmptySessionID
	}
	if episodeID == "" {
		return ErrEmptyEpisodeID
	}
	if strings.Contains(episodeID, "/") || strings.Contains(episodeID, `\`) || episodeID == "." || episodeID == ".." {
		return ErrInvalidEpisodeID
	}
	for _, r := range episodeID {
		if unicode.IsSpace(r) {
			return ErrInvalidEpisodeID
		}
	}
	return nil
}

func (c *ColdStore) episodePath(sessionID, episodeID string) (string, error) {
	sum := sha256.Sum256([]byte(episodeID))
	bucket := hex.EncodeToString(sum[:])[:pageBucketLen]
	dir := filepath.Join(paths.StateDirPath(c.projectRoot), ColdSubdir, sanitizeSegment(sessionID), bucket)
	if err := fileutil.EnsureDir(dir); err != nil {
		return "", err
	}
	return filepath.Join(dir, episodeID+".episode"), nil
}
