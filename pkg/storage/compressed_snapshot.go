package storage

import (
	"sort"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

const (
	// CompressedSnapshotFormatVersion is the current format version
	CompressedSnapshotFormatVersion = "1.1.0"
	// CompressedSnapshotFileExtension is the file extension for compressed snapshots
	CompressedSnapshotFileExtension = ".csnap"
)

// CompressedSnapshot represents a compressed snapshot with header, dictionary, and data
type CompressedSnapshot struct {
	Header     *SnapshotHeader     `yaml:"header" json:"header"`
	Dictionary *SnapshotDictionary `yaml:"dictionary" json:"dictionary"`
	Data       *SnapshotData       `yaml:"data" json:"data"`
}

// SnapshotHeader contains metadata about the compressed snapshot
type SnapshotHeader struct {
	FormatVersion   string         `yaml:"format_version" json:"format_version"`
	Timestamp       time.Time      `yaml:"timestamp" json:"timestamp"`
	ObjectCount     int            `yaml:"object_count" json:"object_count"`
	DictionarySize  int            `yaml:"dictionary_size" json:"dictionary_size"`
	CompressionAlgo string         `yaml:"compression_algo" json:"compression_algo"`
	ExpansionRules  ExpansionRules `yaml:"expansion_rules" json:"expansion_rules"`
	Checksum        string         `yaml:"checksum" json:"checksum"`
	OriginalSize    int64          `yaml:"original_size,omitempty" json:"original_size,omitempty"`
	CompressedSize  int64          `yaml:"compressed_size,omitempty" json:"compressed_size,omitempty"`
}

// SnapshotDictionary maps strings to integer IDs for compression
type SnapshotDictionary struct {
	FieldNames map[int]string `yaml:"field_names" json:"field_names"`               // ID -> field name
	Values     map[int]string `yaml:"values" json:"values"`                         // ID -> value
	Patterns   map[int]string `yaml:"patterns,omitempty" json:"patterns,omitempty"` // ID -> pattern template
}

// Reverse dictionaries for fast lookup during compression
type reverseDictionary struct {
	fieldNames         map[string]int
	values             map[string]int
	patterns           map[string]int
	referenceTimestamp time.Time
}

// SnapshotData contains the compressed object data
type SnapshotData struct {
	Objects []CompressedObject `yaml:"objects" json:"objects"`
}

// CompressedObject represents a single object in compressed format
type CompressedObject map[string]any

// ExpansionRules define how to reconstruct data from compressed format
type ExpansionRules struct {
	Version               string                `yaml:"version" json:"version"`
	ReferenceTimestamp    string                `yaml:"reference_timestamp,omitempty" json:"reference_timestamp,omitempty"`
	CompressionStrategies []CompressionStrategy `yaml:"compression_strategies,omitempty" json:"compression_strategies,omitempty"`
}

// CompressionStrategy describes a compression technique used
type CompressionStrategy struct {
	Type    string   `yaml:"type" json:"type"`         // dictionary_reference, pattern_matching, delta_encoding
	ApplyTo []string `yaml:"apply_to" json:"apply_to"` // field_names, common_values, timestamps, etc.
}

// DictionaryBuilder builds frequency-based dictionaries from objects
type DictionaryBuilder struct {
	fieldNameCounts    map[string]int
	valueCounts        map[string]int
	patternCounts      map[string]int
	referenceTimestamp time.Time
	logger             logging.Logger
}

// NewDictionaryBuilder creates a new dictionary builder
func NewDictionaryBuilder(logger logging.Logger) *DictionaryBuilder {
	return &DictionaryBuilder{
		fieldNameCounts: make(map[string]int),
		valueCounts:     make(map[string]int),
		patternCounts:   make(map[string]int),
		logger:          logger,
	}
}

// AnalyzeObject analyzes an object and counts field names and values
func (db *DictionaryBuilder) AnalyzeObject(obj map[string]any) {
	db.analyzeValue(obj, "")
}

// analyzeValue recursively analyzes values in an object
func (db *DictionaryBuilder) analyzeValue(value any, fieldPath string) {
	switch v := value.(type) {
	case map[string]any:
		for key, val := range v {
			db.fieldNameCounts[key]++
			db.analyzeValue(val, key)
		}
	case []any:
		for _, elem := range v {
			db.analyzeValue(elem, fieldPath)
		}
	case string:
		// Logic to identify if this is a timestamp field
		if isTimestampField(fieldPath) {
			if ts, err := time.Parse(time.RFC3339, v); err == nil {
				if db.referenceTimestamp.IsZero() || ts.Before(db.referenceTimestamp) {
					db.referenceTimestamp = ts
				}
				// Don't add to value dictionary if we'll use delta encoding
				return
			}
		}

		db.valueCounts[v]++

		if pattern := extractPattern(v); pattern != emptyValue {
			db.patternCounts[pattern]++
		}
	}
}

func isTimestampField(fieldPath string) bool {
	return strings.HasSuffix(fieldPath, "_at") || fieldPath == "timestamp"
}

// extractPattern extracts a pattern from a string (e.g., "BLI-001" -> "BLI-{n}")
func extractPattern(s string) string {
	parts := strings.Split(s, "-")
	if len(parts) == 2 && isNumeric(parts[1]) {
		return parts[0] + "-{n}"
	}
	return ""
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	if s == emptyValue {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// BuildDictionary creates dictionaries sorted by frequency
func (db *DictionaryBuilder) BuildDictionary() *SnapshotDictionary {
	dict := &SnapshotDictionary{
		FieldNames: make(map[int]string),
		Values:     make(map[int]string),
		Patterns:   make(map[int]string),
	}

	fieldNamePairs := make([]pair, 0, len(db.fieldNameCounts))
	for name, count := range db.fieldNameCounts {
		fieldNamePairs = append(fieldNamePairs, pair{key: name, count: count})
	}
	sort.Sort(byCount(fieldNamePairs))
	for i, p := range fieldNamePairs {
		dict.FieldNames[i] = p.key
	}

	// Scientific threshold: only include values if they save significant space
	// Assuming reference overhead is 5 bytes ("v1234") and dict overhead is 20 bytes.
	valuePairs := make([]pair, 0, len(db.valueCounts))
	for value, count := range db.valueCounts {
		if count < 2 {
			continue
		}

		saved := (len(value) - 5) * count
		overhead := len(value) + 20
		if saved > overhead {
			valuePairs = append(valuePairs, pair{key: value, count: count})
		}
	}
	sort.Sort(byCount(valuePairs))
	for i, p := range valuePairs {
		dict.Values[i] = p.key
	}

	patternPairs := make([]pair, 0, len(db.patternCounts))
	for pattern, count := range db.patternCounts {
		if count > 10 { // Only very frequent patterns
			patternPairs = append(patternPairs, pair{key: pattern, count: count})
		}
	}
	sort.Sort(byCount(patternPairs))
	for i, p := range patternPairs {
		dict.Patterns[i] = p.key
	}

	return dict
}

type pair struct {
	key   string
	count int
}

type byCount []pair

func (b byCount) Len() int           { return len(b) }
func (b byCount) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byCount) Less(i, j int) bool { return b[i].count > b[j].count }

func buildReverseDictionary(dict *SnapshotDictionary, refTime time.Time) *reverseDictionary {
	rev := &reverseDictionary{
		fieldNames:         make(map[string]int),
		values:             make(map[string]int),
		patterns:           make(map[string]int),
		referenceTimestamp: refTime,
	}

	for id, name := range dict.FieldNames {
		rev.fieldNames[name] = id
	}
	for id, value := range dict.Values {
		rev.values[value] = id
	}
	for id, pattern := range dict.Patterns {
		rev.patterns[pattern] = id
	}

	return rev
}

// CompressObjects compresses a list of objects using the dictionary

func CreateCompressedSnapshot(objects []map[string]any, timestamp time.Time, logger logging.Logger) (*CompressedSnapshot, error) {
	builder := NewDictionaryBuilder(logger)
	for _, obj := range objects {
		builder.AnalyzeObject(obj)
	}
	dict := builder.BuildDictionary()
	refTime := builder.referenceTimestamp

	compressed, err := CompressObjects(objects, dict, refTime)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToCompressObjects).Wrap(err)
	}

	expansionRules := ExpansionRules{
		Version:            CompressedSnapshotFormatVersion,
		ReferenceTimestamp: refTime.Format(time.RFC3339),
		CompressionStrategies: []CompressionStrategy{
			{
				Type:    ConstMiscDictionaryReference,
				ApplyTo: []string{"field_names", ConstMiscRepeatingStrings},
			},
			{
				Type:    ConstMiscDeltaEncoding,
				ApplyTo: []string{"timestamps"},
			},
		},
	}

	header := &SnapshotHeader{
		FormatVersion:   CompressedSnapshotFormatVersion,
		Timestamp:       timestamp,
		ObjectCount:     len(objects),
		DictionarySize:  len(dict.FieldNames) + len(dict.Values) + len(dict.Patterns),
		CompressionAlgo: ConstMiscDictionaryDelta,
		ExpansionRules:  expansionRules,
	}

	// Calculate original size (uncompressed YAML) for metrics
	var originalSize int64
	for _, obj := range objects {
		if data, err := yaml.Marshal(obj); err == nil {
			originalSize += int64(len(data))
		}
	}
	header.OriginalSize = originalSize

	cs := &CompressedSnapshot{
		Header:     header,
		Dictionary: dict,
		Data: &SnapshotData{
			Objects: compressed,
		},
	}

	// Calculate compressed size (YAML of the whole csnap)
	if data, err := yaml.Marshal(cs); err == nil {
		header.CompressedSize = int64(len(data))
	}

	checksum, err := CalculateChecksum(cs)
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToCalculateChecksum).Wrap(err)
	}
	cs.Header.Checksum = checksum

	return cs, nil
}

// Expand decompresses a compressed snapshot back to objects
func (cs *CompressedSnapshot) Expand() ([]map[string]any, error) {
	refTime, _err_82796227 := time.Parse(time.RFC3339, cs.Header.ExpansionRules.ReferenceTimestamp)
	if _err_82796227 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_82796227).Log()
	}
	return ExpandObjects(cs.Data.Objects, cs.Dictionary, refTime)
}
