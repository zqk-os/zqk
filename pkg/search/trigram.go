package search

import (
	"bytes"
	"encoding/gob"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// PackTrigram converts 3 bytes into a single uint32 for fast hashing and comparison.
func PackTrigram(b0, b1, b2 byte) uint32 {
	return (uint32(b0) << 16) | (uint32(b1) << 8) | uint32(b2)
}

func toLowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// ExtractTrigrams extracts all unique 3-byte sequences from a byte slice.
func ExtractTrigrams(data []byte, caseInsensitive bool) map[uint32]struct{} {
	trigrams := make(map[uint32]struct{})
	n := len(data)
	if n < 3 {
		return trigrams
	}

	if caseInsensitive {
		for i := 0; i <= n-3; i++ {
			b0 := toLowerByte(data[i])
			b1 := toLowerByte(data[i+1])
			b2 := toLowerByte(data[i+2])
			trigrams[PackTrigram(b0, b1, b2)] = struct{}{}
		}
	} else {
		for i := 0; i <= n-3; i++ {
			trigrams[PackTrigram(data[i], data[i+1], data[i+2])] = struct{}{}
		}
	}
	return trigrams
}

// ExtractQueryTrigrams extracts trigrams from a string query.
func ExtractQueryTrigrams(query string, caseInsensitive bool) []uint32 {
	if caseInsensitive {
		query = strings.ToLower(query)
	}
	data := []byte(query)
	n := len(data)
	if n < 3 {
		return nil
	}

	seen := make(map[uint32]struct{})
	var result []uint32
	for i := 0; i <= n-3; i++ {
		tg := PackTrigram(data[i], data[i+1], data[i+2])
		if _, ok := seen[tg]; !ok {
			seen[tg] = struct{}{}
			result = append(result, tg)
		}
	}
	return result
}

// TrigramIndex stores an inverted index mapping trigrams to candidate document IDs.
type TrigramIndex struct {
	mu       sync.RWMutex
	files    []string
	postings map[uint32][]int32
}

// NewTrigramIndex constructs an empty TrigramIndex.
func NewTrigramIndex() *TrigramIndex {
	return &TrigramIndex{
		postings: make(map[uint32][]int32),
	}
}

// BuildIndex builds a trigram index over a collection of file paths.
func BuildIndex(files []string) (*TrigramIndex, error) {
	idx := NewTrigramIndex()
	idx.files = make([]string, len(files))
	copy(idx.files, files)

	type fileTrigrams struct {
		docID int32
		tgs   map[uint32]struct{}
	}

	results := make(chan fileTrigrams, 64)
	var wg sync.WaitGroup

	// Worker pool reading and extracting trigrams in parallel
	numWorkers := 8
	fileChan := make(chan int32, 128)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("search.trigram_worker", "extract trigrams from candidate files").
			StartSimple(func() {
				defer wg.Done()
				for docID := range fileChan {
					path := files[docID]
					data, err := fileutil.ReadFile(path)
					if err != nil || IsBinary(data) {
						continue
					}
					// Always index in case-insensitive lowercase form for universal lookup
					tgs := ExtractTrigrams(data, true)
					results <- fileTrigrams{docID: docID, tgs: tgs}
				}
			})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("search.trigram_wait", "wait for trigram workers").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})

	goroutinelabels.NewGoroutine("search.trigram_feed", "feed files to trigram workers").
		StartSimple(func() {
			for i := range files {
				fileChan <- int32(i)
			}
			close(fileChan)
			select {
			case <-waitDone:
			case <-time.After(60 * time.Second):
			}
			close(results)
		})

	for ft := range results {
		for tg := range ft.tgs {
			idx.postings[tg] = append(idx.postings[tg], ft.docID)
		}
	}

	return idx, nil
}

// FilterCandidates returns candidate file paths that match all query trigrams.
// If query has < 3 chars, returns all files in index.
func (idx *TrigramIndex) FilterCandidates(query string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	queryTgs := ExtractQueryTrigrams(query, true)
	if len(queryTgs) == 0 {
		// Cannot filter with trigrams: return all indexed files
		res := make([]string, len(idx.files))
		copy(res, idx.files)
		return res
	}

	// Intersect posting lists
	var candidateSet map[int32]struct{}
	for i, tg := range queryTgs {
		postings, ok := idx.postings[tg]
		if !ok || len(postings) == 0 {
			// A required trigram is absent anywhere in the index: 0 matches
			return nil
		}

		if i == 0 {
			candidateSet = make(map[int32]struct{}, len(postings))
			for _, docID := range postings {
				candidateSet[docID] = struct{}{}
			}
		} else {
			// Retain only docIDs present in this posting list
			currentSet := make(map[int32]struct{}, len(postings))
			for _, docID := range postings {
				currentSet[docID] = struct{}{}
			}
			for docID := range candidateSet {
				if _, found := currentSet[docID]; !found {
					delete(candidateSet, docID)
				}
			}
			if len(candidateSet) == 0 {
				return nil
			}
		}
	}

	var candidates []string
	for docID := range candidateSet {
		if int(docID) < len(idx.files) {
			candidates = append(candidates, idx.files[docID])
		}
	}
	return candidates
}

// IsWordBoundary returns true if position is adjacent to non-alphanumeric runes.
func IsWordBoundary(line []byte, start, end int) bool {
	if start > 0 {
		prev := rune(line[start-1])
		if unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' {
			return false
		}
	}
	if end < len(line) {
		next := rune(line[end])
		if unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_' {
			return false
		}
	}
	return true
}

// LineSearch scans lines of a file for matches with surrounding context.
func LineSearch(filePath string, data []byte, matcher func(line []byte) (int, int, bool), contextLines int) []Match {
	var matches []Match
	lines := bytes.Split(data, []byte("\n"))
	numLines := len(lines)

	for lineIdx, line := range lines {
		start, _, ok := matcher(line)
		if !ok {
			continue
		}

		lineNum := lineIdx + 1
		colNum := start + 1

		var before []string
		if contextLines > 0 {
			from := lineIdx - contextLines
			if from < 0 {
				from = 0
			}
			for i := from; i < lineIdx; i++ {
				before = append(before, string(lines[i]))
			}
		}

		var after []string
		if contextLines > 0 {
			to := lineIdx + contextLines + 1
			if to > numLines {
				to = numLines
			}
			for i := lineIdx + 1; i < to; i++ {
				after = append(after, string(lines[i]))
			}
		}

		matches = append(matches, Match{
			File:          filePath,
			Line:          lineNum,
			Column:        colNum,
			EndLine:       lineNum,
			LineContent:   string(line),
			ContextBefore: before,
			ContextAfter:  after,
		})
	}

	return matches
}

// SerializedTrigramIndex is the on-disk format for the cached trigram index.
type SerializedTrigramIndex struct {
	Files    []string
	Postings map[uint32][]int32
}

// SaveToFile serializes the trigram index to disk using encoding/gob.
func (idx *TrigramIndex) SaveToFile(path string) error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	payload := SerializedTrigramIndex{
		Files:    idx.files,
		Postings: idx.postings,
	}
	if err := enc.Encode(&payload); err != nil {
		return err
	}
	return fileutil.WriteDurableFile(path, buf.Bytes(), 0o644)
}

// LoadIndexFromFile deserializes a cached trigram index from disk.
func LoadIndexFromFile(path string) (*TrigramIndex, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var payload SerializedTrigramIndex
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}

	return &TrigramIndex{
		files:    payload.Files,
		postings: payload.Postings,
	}, nil
}
