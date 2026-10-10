package matrix

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LiteralLocation denotes the specific file and line where a literal was discovered.
type LiteralLocation struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// LiteralRecord tracks global occurrences of a string literal across the codebase.
type LiteralRecord struct {
	Literal   string            `json:"literal"`
	Count     int               `json:"count"`
	Locations []LiteralLocation `json:"locations"`
	FirstSeen time.Time         `json:"first_seen"`
	LastSeen  time.Time         `json:"last_seen"`
}

// LiteralInventory maintains an alphabetical repository-wide inventory of all extracted string literals.
type LiteralInventory struct {
	mu        sync.RWMutex
	storePath string
	Literals  map[string]*LiteralRecord `json:"literals"`
}

// NewLiteralInventory creates or loads the global literal inventory from the given path.
func NewLiteralInventory(storePath string) (*LiteralInventory, error) {
	inv := &LiteralInventory{
		storePath: storePath,
		Literals:  make(map[string]*LiteralRecord),
	}
	if err := inv.load(); err != nil && !fileutil.IsNotExist(err) {
		return nil, err
	}
	return inv, nil
}

func (inv *LiteralInventory) load() error {
	inv.mu.Lock()
	defer inv.mu.Unlock()

	data, err := fileutil.ReadFile(inv.storePath)
	if err != nil {
		return err
	}
	var stored struct {
		Literals map[string]*LiteralRecord `json:"literals"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.Literals != nil {
		inv.Literals = stored.Literals
	}
	return nil
}

// Save persists the inventory atomically in alphabetical order.
func (inv *LiteralInventory) Save() error {
	inv.mu.RLock()
	defer inv.mu.RUnlock()

	if inv.storePath == "" {
		return nil
	}

	if err := fileutil.MkdirAll(filepath.Dir(inv.storePath), fileutil.StandardDirPerm); err != nil {
		return err
	}

	payload := struct {
		UpdatedAt time.Time                 `json:"updated_at"`
		Total     int                       `json:"total"`
		Literals  map[string]*LiteralRecord `json:"literals"`
	}{
		UpdatedAt: time.Now().UTC(),
		Total:     len(inv.Literals),
		Literals:  inv.Literals,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return fileutil.WriteSecureFile(inv.storePath, data)
}

// Record indexes an observed string literal and returns the updated count and whether it violates deduplication.
func (inv *LiteralInventory) Record(literal string, filePath string, line int) (int, bool) {
	trimmed := strings.TrimSpace(literal)
	if trimmed == "" {
		return 0, false
	}

	inv.mu.Lock()
	defer inv.mu.Unlock()

	rec, exists := inv.Literals[trimmed]
	now := time.Now().UTC()
	if !exists {
		rec = &LiteralRecord{
			Literal:   trimmed,
			Count:     1,
			Locations: []LiteralLocation{{Path: filePath, Line: line}},
			FirstSeen: now,
			LastSeen:  now,
		}
		inv.Literals[trimmed] = rec
		return 1, false
	}

	// Check if this exact location was already recorded
	duplicateLocation := false
	for _, loc := range rec.Locations {
		if loc.Path == filePath && loc.Line == line {
			duplicateLocation = true
			break
		}
	}
	if !duplicateLocation {
		rec.Count++
		rec.Locations = append(rec.Locations, LiteralLocation{Path: filePath, Line: line})
		rec.LastSeen = now
	}

	// Policy: Max 1 unique occurrence tolerated globally. >= 2 is a deduplication violation.
	isViolation := rec.Count >= 2
	return rec.Count, isViolation
}

// FindDuplicates returns all string literals that appear >= 2 times across the codebase.
func (inv *LiteralInventory) FindDuplicates() []*LiteralRecord {
	inv.mu.RLock()
	defer inv.mu.RUnlock()

	var dups []*LiteralRecord
	for _, rec := range inv.Literals {
		if rec.Count >= 2 {
			dups = append(dups, rec)
		}
	}

	sort.Slice(dups, func(i, j int) bool {
		if dups[i].Count == dups[j].Count {
			return dups[i].Literal < dups[j].Literal
		}
		return dups[i].Count > dups[j].Count
	})

	return dups
}

// TotalCount returns the count of unique string literals recorded.
func (inv *LiteralInventory) TotalCount() int {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	return len(inv.Literals)
}

// DuplicateCount returns the count of literals violating the deduplication threshold.
func (inv *LiteralInventory) DuplicateCount() int {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	count := 0
	for _, rec := range inv.Literals {
		if rec.Count >= 2 {
			count++
		}
	}
	return count
}
