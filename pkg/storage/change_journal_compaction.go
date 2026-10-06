package storage

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

var (
	changeJournalWindowsCompactedTotal atomic.Int64
	changeJournalCompactedEntriesTotal atomic.Int64
)

// GetChangeJournalCompactionStats returns lifetime counters for windows compacted and entries compacted.
func GetChangeJournalCompactionStats() (windows, entries int64) {
	return changeJournalWindowsCompactedTotal.Load(), changeJournalCompactedEntriesTotal.Load()
}

const (
	// CompactedChangeJournalFormatVersion is the format version for compacted journal artifacts
	CompactedChangeJournalFormatVersion = "1.0.0"
	// ChangeJournalEntryIDPrefix is the prefix for change journal entry IDs (CHA-)
	ChangeJournalEntryIDPrefix = "CHA-"
)

// CompactedJournalHeader holds metadata for a compacted change journal artifact
type CompactedJournalHeader struct {
	FormatVersion      string    `yaml:"format_version" json:"format_version"`
	Timestamp          time.Time `yaml:"timestamp" json:"timestamp"`
	ObjectCount        int       `yaml:"object_count" json:"object_count"`
	EntryIDRanges      []string  `yaml:"entry_id_ranges" json:"entry_id_ranges"`
	DictionarySize     int       `yaml:"dictionary_size" json:"dictionary_size"`
	Checksum           string    `yaml:"checksum" json:"checksum"`
	ReferenceTimestamp string    `yaml:"reference_timestamp,omitempty" json:"reference_timestamp,omitempty"`
	WindowStart        string    `yaml:"window_start,omitempty" json:"window_start,omitempty"`
	WindowEnd          string    `yaml:"window_end,omitempty" json:"window_end,omitempty"`
}

// CompactedChangeJournalArtifact is the on-disk format for one compacted window of change journal entries
type CompactedChangeJournalArtifact struct {
	Header     *CompactedJournalHeader `yaml:"header" json:"header"`
	Dictionary *SnapshotDictionary     `yaml:"dictionary" json:"dictionary"`
	Data       *SnapshotData           `yaml:"data" json:"data"`
}

// CompactionResult holds the result of compacting a window of change journal entries
type CompactionResult struct {
	ArtifactPath   string   // path to the written artifact
	EntryIDRanges  []string // compressed entry ID ranges (e.g. CHA-1..CHA-960)
	EntryCount     int
	WindowStart    time.Time
	WindowEnd      time.Time
	DictionarySize int
}

// EntryMapsFromEntries converts raw change_journal_entry objects into a normalized list of maps
// suitable for dictionary building and compression (id, object_ref, change_type, created_at, created_by, changed_paths).
func EntryMapsFromEntries(entries []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		m := make(map[string]any)
		if v := objects.GetString(e, objects.FieldKeyID); v != "" {
			m[objects.FieldKeyID] = v
		}
		if v := objects.GetString(e, objects.FieldKeyObjectRef); v != "" {
			m[objects.FieldKeyObjectRef] = v
		}
		if v := objects.GetString(e, objects.FieldKeyChangeType); v != "" {
			m[objects.FieldKeyChangeType] = v
		}
		if v := objects.GetString(e, objects.FieldKeyCreatedAt); v != "" {
			m[objects.FieldKeyCreatedAt] = v
		}
		if v := objects.GetString(e, objects.FieldKeyCreatedBy); v != "" {
			m[objects.FieldKeyCreatedBy] = v
		}
		if v, ok := e[objects.FieldKeyChangedPaths].([]any); ok {
			paths := make([]any, 0, len(v))
			for _, p := range v {
				if s, ok := p.(string); ok {
					paths = append(paths, s)
				}
			}
			m[objects.FieldKeyChangedPaths] = paths
		} else if v, ok := e[objects.FieldKeyChangedPaths].([]string); ok {
			m[objects.FieldKeyChangedPaths] = v
		}
		out = append(out, m)
	}
	return out
}

// CompressEntryIDs compresses consecutive change journal entry IDs into ranges (e.g. CHA-1..CHA-960).
// Same logic as audit compressEventIDs but for CHA- prefix.
func CompressEntryIDs(entries []map[string]any) []string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if id := objects.GetString(e, objects.FieldKeyID); id != "" && strings.HasPrefix(id, ChangeJournalEntryIDPrefix) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool {
		return compareChangeJournalID(ids[i], ids[j]) < 0
	})
	return compressIDSorted(ids)
}

func compareChangeJournalID(id1, id2 string) int {
	n1 := extractChangeJournalIDNumber(id1)
	n2 := extractChangeJournalIDNumber(id2)
	if n1 < n2 {
		return -1
	}
	if n1 > n2 {
		return 1
	}
	return 0
}

func extractChangeJournalIDNumber(id string) int {
	parts := strings.Split(id, "-")
	if len(parts) != 2 {
		return 0
	}
	n, _err_83870319 := strconv.Atoi(parts[1])
	if _err_83870319 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83870319).Log()
	}
	return n
}

func isConsecutiveChangeJournalID(id1, id2 string) bool {
	return extractChangeJournalIDNumber(id2) == extractChangeJournalIDNumber(id1)+1
}

func compressIDSorted(ids []string) []string {
	result := make([]string, 0)
	var rangeStart, rangeEnd string
	rangeCount := 0
	for i, id := range ids {
		if i == 0 {
			rangeStart = id
			rangeEnd = id
			rangeCount = 1
			continue
		}
		if isConsecutiveChangeJournalID(rangeEnd, id) {
			rangeEnd = id
			rangeCount++
		} else {
			when.When(func() bool { return rangeCount > 2 }).Then(func() {
				result = append(result, fmt.Sprintf("%s..%s", rangeStart, rangeEnd))
			}).OrElseWhen(func() bool { return rangeCount == 1 }).Then(func() {
				result = append(result, rangeStart)
			}).OrElse(func() {
				result = append(result, rangeStart, rangeEnd)
			}).Run()
			rangeStart = id
			rangeEnd = id
			rangeCount = 1
		}
	}
	when.When(func() bool { return rangeCount > 2 }).Then(func() {
		result = append(result, fmt.Sprintf("%s..%s", rangeStart, rangeEnd))
	}).OrElseWhen(func() bool { return rangeCount == 1 }).Then(func() {
		result = append(result, rangeStart)
	}).OrElse(func() {
		result = append(result, rangeStart, rangeEnd)
	}).Run()
	return result
}

// ExpandChangeJournalIDRanges expands entry ID ranges (e.g. CHA-1..CHA-960) into a flat list of IDs.
func ExpandChangeJournalIDRanges(ranges []string) []string {
	var out []string
	for _, r := range ranges {
		out = append(out, expandChangeJournalIDRange(r)...)
	}
	return out
}

func expandChangeJournalIDRange(rangeStr string) []string {
	if !strings.Contains(rangeStr, "..") {
		return []string{rangeStr}
	}
	parts := strings.Split(rangeStr, "..")
	if len(parts) != 2 {
		return []string{rangeStr}
	}
	startStr := strings.TrimPrefix(parts[0], ChangeJournalEntryIDPrefix)
	endStr := strings.TrimPrefix(parts[1], ChangeJournalEntryIDPrefix)
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start > end || (end-start+1) > 50000 {
		return []string{rangeStr}
	}
	ids := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		ids = append(ids, fmt.Sprintf("%s%d", ChangeJournalEntryIDPrefix, i))
	}
	return ids
}

// CompactChangeJournalWindow builds a dictionary over the given entries, compresses them with
// snapshot-style encoding, writes one artifact per window, and returns the result (path and entry ID ranges).
// It does not archive or delete the original entries; the caller may do that after a successful write.
func CompactChangeJournalWindow(
	entries []map[string]any,
	outputDir string,
	windowStart, windowEnd time.Time,
	logger logging.Logger,
) (*CompactionResult, error) {
	if len(entries) == 0 {
		return &CompactionResult{
			EntryCount:  0,
			WindowStart: windowStart,
			WindowEnd:   windowEnd,
		}, nil
	}
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	entryMaps := EntryMapsFromEntries(entries)
	db := NewDictionaryBuilder(logger)
	for _, m := range entryMaps {
		db.AnalyzeObject(m)
	}
	dict := db.BuildDictionary()
	refTime := db.referenceTimestamp
	compressed, err := CompressObjects(entryMaps, dict, refTime)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditCompressEntries).Wrap(err)
	}
	entryIDRanges := CompressEntryIDs(entries)
	artifact := &CompactedChangeJournalArtifact{
		Header: &CompactedJournalHeader{
			FormatVersion:      CompactedChangeJournalFormatVersion,
			Timestamp:          time.Now().UTC(),
			ObjectCount:        len(entries),
			EntryIDRanges:      entryIDRanges,
			DictionarySize:     len(dict.FieldNames) + len(dict.Values) + len(dict.Patterns),
			ReferenceTimestamp: refTime.Format(time.RFC3339),
			WindowStart:        windowStart.Format(time.RFC3339),
			WindowEnd:          windowEnd.Format(time.RFC3339),
		},
		Dictionary: dict,
		Data:       &SnapshotData{Objects: compressed},
	}
	checksum, err := calculateCompactedArtifactChecksum(artifact)
	if err != nil {
		return nil, errfmt.Newf("checksum").Wrap(err)
	}
	artifact.Header.Checksum = checksum
	name := fmt.Sprintf(ConstAuditCompactedCjournal, windowStart.Format("2006-01-02"))
	outputPath := filepath.Join(outputDir, name)
	if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("mkdir output").Wrap(err)
	}
	data, err := yaml.Marshal(artifact)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditMarshalArtifact).Wrap(err)
	}
	if err := fileutil.WriteFile(outputPath, data, paths.FilePerm600); err != nil {
		return nil, errfmt.Newf(ConstAuditWriteArtifact).Wrap(err)
	}

	changeJournalWindowsCompactedTotal.Add(1)
	changeJournalCompactedEntriesTotal.Add(int64(len(entries)))

	return &CompactionResult{
		ArtifactPath:   outputPath,
		EntryIDRanges:  entryIDRanges,
		EntryCount:     len(entries),
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		DictionarySize: artifact.Header.DictionarySize,
	}, nil
}

func calculateCompactedArtifactChecksum(a *CompactedChangeJournalArtifact) (string, error) {
	dataJSON, err := json.Marshal(a.Data)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(dataJSON)
	return fmt.Sprintf("%x", hash), nil
}

// ReadCompactedChangeJournalArtifact reads a compacted change journal artifact from disk.
func ReadCompactedChangeJournalArtifact(path string) (*CompactedChangeJournalArtifact, error) {
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, errfmt.Newf("read file").Wrap(err)
	}
	var a CompactedChangeJournalArtifact
	if err := yaml.Unmarshal(data, &a); err != nil {
		return nil, errfmt.Newf("unmarshal").Wrap(err)
	}
	if a.Header == nil || a.Dictionary == nil || a.Data == nil {
		return nil, errfmt.Errorf(ConstAuditInvalidArtifactMissingHeaderDictionaryOrData)
	}
	return &a, nil
}

// ExpandCompactedChangeJournalArtifact expands the compressed objects in the artifact back to entry maps.
func ExpandCompactedChangeJournalArtifact(a *CompactedChangeJournalArtifact) ([]map[string]any, error) {
	refTime, _err_83876028 := time.Parse(time.RFC3339, a.Header.ReferenceTimestamp)
	if _err_83876028 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_83876028).Log()
	}
	return ExpandObjects(a.Data.Objects, a.Dictionary, refTime)
}
