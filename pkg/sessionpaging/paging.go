// Package sessionpaging provides community-local hot virtual-context pages.
//
// Stranger / open-core sessions keep bounded local context under the project
// state tree without importing Studio-only paging modules (github.com/zqk-os/zqke,
// virtualcontext, studio/paging). Cold episodic MMU remains v0.2+; v0.1 continuity
// also reconstructs from kernel objects via `zqk workflow whats-next`.
//
// TRACK: community-session-paging (open-core) — remove when: cold episodic store ships.
package sessionpaging

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// PagesSubdir is the state subdirectory for hot session pages.
	PagesSubdir = "session_pages"
	// DefaultMaxHotPages bounds in-session hot pages (evict oldest first).
	DefaultMaxHotPages = 32
	// pageBucketLen keeps ≤100 top-level entries under a session dir (hex prefix).
	pageBucketLen = 2
)

// ForbiddenStudioModuleFragments must never appear as required import paths for
// community session paging. Used by gates and package self-checks.
var ForbiddenStudioModuleFragments = []string{
	"github.com/zqk-os/zqke",
	"virtualcontext",
	"studio/paging",
}

var (
	// ErrEmptyProjectRoot is returned when project root is missing.
	ErrEmptyProjectRoot = errors.New("sessionpaging: project root is required")
	// ErrEmptySessionID is returned when session id is missing.
	ErrEmptySessionID = errors.New("sessionpaging: session id is required")
	// ErrEmptyPageID is returned when page id is missing.
	ErrEmptyPageID = errors.New("sessionpaging: page id is required")
	// ErrInvalidPageID is returned when page id has unsafe path characters.
	ErrInvalidPageID = errors.New("sessionpaging: page id must be a single path segment")
	// ErrPageNotFound is returned when a page is missing.
	ErrPageNotFound = errors.New("sessionpaging: page not found")
	// ErrStudioPagingDep is returned when a studio-only paging module path is requested.
	ErrStudioPagingDep = errors.New("sessionpaging: studio-only paging dependency refused")
)

// Store keeps hot context pages on local disk under .zqk/state/session_pages.
type Store struct {
	projectRoot string
	maxHotPages int
}

// NewStore returns a community-local page store rooted at projectRoot.
func NewStore(projectRoot string) (*Store, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, ErrEmptyProjectRoot
	}
	return &Store{projectRoot: root, maxHotPages: DefaultMaxHotPages}, nil
}

// WithMaxHotPages sets the hot-page eviction bound (minimum 1).
func (s *Store) WithMaxHotPages(n int) *Store {
	if s == nil {
		return nil
	}
	if n < 1 {
		n = 1
	}
	s.maxHotPages = n
	return s
}

// RefuseStudioModulePath fail-closes when path looks like a Studio-only pager import.
func RefuseStudioModulePath(modulePath string) error {
	p := strings.ToLower(strings.TrimSpace(modulePath))
	if p == "" {
		return nil
	}
	for _, frag := range ForbiddenStudioModuleFragments {
		if strings.Contains(p, strings.ToLower(frag)) {
			return fmt.Errorf("%w: %s", ErrStudioPagingDep, modulePath)
		}
	}
	return nil
}

// Put writes a hot page for sessionID and enforces the hot-page bound.
func (s *Store) Put(sessionID, pageID string, payload []byte) error {
	if s == nil {
		return ErrEmptyProjectRoot
	}
	if err := validateIDs(sessionID, pageID); err != nil {
		return err
	}
	path, err := s.pagePath(sessionID, pageID)
	if err != nil {
		return err
	}
	if err := fileutil.WriteDurableSecureFile(path, payload); err != nil {
		return err
	}
	return s.evictOldest(sessionID)
}

// Get reads a hot page. Missing pages return ErrPageNotFound.
func (s *Store) Get(sessionID, pageID string) ([]byte, error) {
	if s == nil {
		return nil, ErrEmptyProjectRoot
	}
	if err := validateIDs(sessionID, pageID); err != nil {
		return nil, err
	}
	path, err := s.pagePath(sessionID, pageID)
	if err != nil {
		return nil, err
	}
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrPageNotFound
		}
		return nil, err
	}
	return data, nil
}

// ListPageIDs returns page ids for a session (unsorted).
func (s *Store) ListPageIDs(sessionID string) ([]string, error) {
	if s == nil {
		return nil, ErrEmptyProjectRoot
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrEmptySessionID
	}
	root := s.sessionDir(sessionID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		bucket := filepath.Join(root, ent.Name())
		files, err := os.ReadDir(bucket)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if strings.HasSuffix(name, ".page") {
				ids = append(ids, strings.TrimSuffix(name, ".page"))
			}
		}
	}
	return ids, nil
}

func validateIDs(sessionID, pageID string) error {
	sessionID = strings.TrimSpace(sessionID)
	pageID = strings.TrimSpace(pageID)
	if sessionID == "" {
		return ErrEmptySessionID
	}
	if pageID == "" {
		return ErrEmptyPageID
	}
	if strings.Contains(pageID, "/") || strings.Contains(pageID, `\`) || pageID == "." || pageID == ".." {
		return ErrInvalidPageID
	}
	for _, r := range pageID {
		if unicode.IsSpace(r) {
			return ErrInvalidPageID
		}
	}
	return nil
}

func (s *Store) sessionDir(sessionID string) string {
	safe := sanitizeSegment(sessionID)
	return filepath.Join(paths.StateDirPath(s.projectRoot), PagesSubdir, safe)
}

func (s *Store) pagePath(sessionID, pageID string) (string, error) {
	sum := sha256.Sum256([]byte(pageID))
	bucket := hex.EncodeToString(sum[:])[:pageBucketLen]
	dir := filepath.Join(s.sessionDir(sessionID), bucket)
	if err := fileutil.EnsureDir(dir); err != nil {
		return "", err
	}
	return filepath.Join(dir, pageID+".page"), nil
}

func sanitizeSegment(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "session"
	}
	return out
}

type pageMeta struct {
	id   string
	path string
	mod  time.Time
}

func (s *Store) evictOldest(sessionID string) error {
	root := s.sessionDir(sessionID)
	var pages []pageMeta
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".page") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		pages = append(pages, pageMeta{
			id:   strings.TrimSuffix(name, ".page"),
			path: path,
			mod:  info.ModTime(),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(pages) <= s.maxHotPages {
		return nil
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].mod.Before(pages[j].mod)
	})
	overflow := len(pages) - s.maxHotPages
	for i := 0; i < overflow; i++ {
		if err := os.Remove(pages[i].path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
