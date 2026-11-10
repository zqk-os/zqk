// Package coverage provides high-performance multi-shard Go coverprofile parsing,
// atomic statement deduplication, and verified package rollup aggregation for CI matrix pipelines.
package coverage

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Supported Go coverage modes.
const (
	ModeSet    = "set"
	ModeCount  = "count"
	ModeAtomic = "atomic"
)

// Scanner buffer limits for large coverage profiles.
const (
	InitialBufferSize = 64 * 1024   // 64 KiB
	MaxBufferSize     = 1024 * 1024 // 1 MiB
)

// Domain errors for coverprofile parsing and multi-shard aggregation.
var (
	ErrEmptyShardSet             = errors.New("coverage: at least one coverage shard is required")
	ErrInvalidCoverageMode       = errors.New("coverage: invalid coverage mode in header")
	ErrInconsistentCoverageMode  = errors.New("coverage: inconsistent coverage mode across shards")
	ErrMalformedCoverageLine     = errors.New("coverage: malformed coverage profile line")
	ErrInvalidStatementCount     = errors.New("coverage: statement count must be positive")
	ErrNegativeExecutionCount    = errors.New("coverage: execution count cannot be negative")
	ErrConflictingStatementCount = errors.New("coverage: conflicting statement counts for identical block")
	ErrInvalidProfilePath        = errors.New("coverage: invalid or escaping profile path")
	ErrNilProfile                = errors.New("coverage: profile cannot be nil")
)

// ProfileBlock represents a single atomic code block from a Go coverprofile.
type ProfileBlock struct {
	Path      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	NumStmt   int
	Count     int
}

// Key returns the canonical immutable identifier for this code block (file:startLine.startCol,endLine.endCol).
func (b *ProfileBlock) Key() string {
	return fmt.Sprintf("%s:%d.%d,%d.%d", b.Path, b.StartLine, b.StartCol, b.EndLine, b.EndCol)
}

// Format returns the standard Go coverprofile line for this block.
func (b *ProfileBlock) Format() string {
	return fmt.Sprintf("%s:%d.%d,%d.%d %d %d", b.Path, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, b.Count)
}

// Validate verifies block structural invariants.
func (b *ProfileBlock) Validate(allowEscape bool) error {
	if b == nil {
		return ErrNilProfile
	}
	clean := strings.TrimSpace(b.Path)
	if clean == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidProfilePath)
	}
	if !allowEscape {
		if strings.HasPrefix(clean, "/") || strings.Contains(clean, `\`) || strings.Contains(clean, "..") {
			return fmt.Errorf("%w: path escapes or contains traversal: %s", ErrInvalidProfilePath, clean)
		}
	}
	if b.StartLine < 1 || b.StartCol < 1 || b.EndLine < b.StartLine || (b.EndLine == b.StartLine && b.EndCol < b.StartCol) {
		return fmt.Errorf("%w: invalid line/col coordinates (%d.%d,%d.%d)", ErrMalformedCoverageLine, b.StartLine, b.StartCol, b.EndLine, b.EndCol)
	}
	if b.NumStmt < 0 {
		return fmt.Errorf("%w: numStmt=%d", ErrInvalidStatementCount, b.NumStmt)
	}
	if b.Count < 0 {
		return fmt.Errorf("%w: count=%d", ErrNegativeExecutionCount, b.Count)
	}
	return nil
}

// MergerConfig defines optional configuration for multi-shard profile merging.
type MergerConfig struct {
	EnforceMode string
	AllowEscape bool
}

// Merger orchestrates multi-shard profile parsing and atomic deduplication.
type Merger struct {
	config MergerConfig
}

// NewMerger returns a configured profile merger.
func NewMerger(cfg ...MergerConfig) *Merger {
	var c MergerConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return &Merger{config: c}
}

// ParseLine parses a single Go coverprofile line into a ProfileBlock.
func (m *Merger) ParseLine(line string) (*ProfileBlock, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "mode:") {
		return nil, nil
	}

	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, fmt.Errorf("%w: expected 3 fields, got %d in line %q", ErrMalformedCoverageLine, len(parts), line)
	}

	// Last field: execution count
	count, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid execution count %q: %v", ErrMalformedCoverageLine, parts[len(parts)-1], err)
	}

	// Second to last field: statement count
	numStmt, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return nil, fmt.Errorf("%w: invalid statement count %q: %v", ErrMalformedCoverageLine, parts[len(parts)-2], err)
	}

	// Range specification: path:startLine.startCol,endLine.endCol
	rangeSpec := strings.Join(parts[:len(parts)-2], " ")
	colonIdx := strings.LastIndex(rangeSpec, ":")
	if colonIdx == -1 {
		return nil, fmt.Errorf("%w: missing colon in range spec %q", ErrMalformedCoverageLine, rangeSpec)
	}

	filePath := rangeSpec[:colonIdx]
	coords := rangeSpec[colonIdx+1:]

	commaIdx := strings.Index(coords, ",")
	if commaIdx == -1 {
		return nil, fmt.Errorf("%w: missing comma in coords %q", ErrMalformedCoverageLine, coords)
	}

	startParts := strings.Split(coords[:commaIdx], ".")
	endParts := strings.Split(coords[commaIdx+1:], ".")
	if len(startParts) != 2 || len(endParts) != 2 {
		return nil, fmt.Errorf("%w: coordinates must be line.col, got %q", ErrMalformedCoverageLine, coords)
	}

	sLine, err1 := strconv.Atoi(startParts[0])
	sCol, err2 := strconv.Atoi(startParts[1])
	eLine, err3 := strconv.Atoi(endParts[0])
	eCol, err4 := strconv.Atoi(endParts[1])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return nil, fmt.Errorf("%w: non-integer coordinate values in %q", ErrMalformedCoverageLine, coords)
	}

	block := &ProfileBlock{
		Path:      filePath,
		StartLine: sLine,
		StartCol:  sCol,
		EndLine:   eLine,
		EndCol:    eCol,
		NumStmt:   numStmt,
		Count:     count,
	}

	if err := block.Validate(m.config.AllowEscape); err != nil {
		return nil, err
	}
	return block, nil
}

// ParseProfile parses an entire coverprofile reader into its mode and blocks.
func (m *Merger) ParseProfile(r io.Reader) (string, []*ProfileBlock, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, InitialBufferSize), MaxBufferSize)

	var detectedMode string
	var blocks []*ProfileBlock

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "mode:") {
			parts := strings.Fields(line)
			if len(parts) != 2 {
				return "", nil, fmt.Errorf("%w: invalid mode declaration %q", ErrInvalidCoverageMode, line)
			}
			mode := strings.ToLower(strings.TrimSpace(parts[1]))
			switch mode {
			case ModeSet, ModeCount, ModeAtomic:
				detectedMode = mode
			default:
				return "", nil, fmt.Errorf("%w: unrecognized mode %q", ErrInvalidCoverageMode, mode)
			}
			continue
		}

		block, err := m.ParseLine(line)
		if err != nil {
			return "", nil, err
		}
		if block != nil {
			blocks = append(blocks, block)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("coverage: scan error: %w", err)
	}
	return detectedMode, blocks, nil
}

// MergedProfile contains the unified statement coverage blocks across all shards.
type MergedProfile struct {
	Mode   string
	blocks map[string]*ProfileBlock
}

// NewMergedProfile initializes an empty merged profile container.
func NewMergedProfile(mode string) *MergedProfile {
	return &MergedProfile{
		Mode:   mode,
		blocks: make(map[string]*ProfileBlock),
	}
}

// AddBlock inserts or aggregates a profile block according to the profile mode.
func (p *MergedProfile) AddBlock(b *ProfileBlock) error {
	if b == nil {
		return ErrNilProfile
	}
	key := b.Key()
	existing, exists := p.blocks[key]
	if !exists {
		// Clone block
		cloned := *b
		p.blocks[key] = &cloned
		return nil
	}

	// Verify statement count consistency
	if existing.NumStmt != b.NumStmt {
		return fmt.Errorf("%w: block %s declared %d statements previously, shard declared %d",
			ErrConflictingStatementCount, key, existing.NumStmt, b.NumStmt)
	}

	// Aggregate counts based on mode
	switch p.Mode {
	case ModeSet:
		if b.Count > 0 {
			existing.Count = 1
		}
	case ModeCount, ModeAtomic:
		existing.Count += b.Count
	default:
		existing.Count += b.Count
	}
	return nil
}

// BlockCount returns the number of distinct deduplicated code blocks.
func (p *MergedProfile) BlockCount() int {
	return len(p.blocks)
}

// TotalStatements calculates total statements and covered statements across all deduplicated blocks.
func (p *MergedProfile) TotalStatements() (total int, covered int) {
	for _, b := range p.blocks {
		total += b.NumStmt
		if b.Count > 0 {
			covered += b.NumStmt
		}
	}
	return total, covered
}

// SortedBlocks returns all blocks sorted canonically by Path, StartLine, and StartCol.
func (p *MergedProfile) SortedBlocks() []*ProfileBlock {
	result := make([]*ProfileBlock, 0, len(p.blocks))
	for _, b := range p.blocks {
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].StartLine != result[j].StartLine {
			return result[i].StartLine < result[j].StartLine
		}
		return result[i].StartCol < result[j].StartCol
	})
	return result
}

// Format returns the standard Go coverprofile byte representation.
func (p *MergedProfile) Format() []byte {
	var buf bytes.Buffer
	mode := p.Mode
	if mode == "" {
		mode = ModeAtomic
	}
	buf.WriteString(fmt.Sprintf("mode: %s\n", mode))
	for _, b := range p.SortedBlocks() {
		buf.WriteString(b.Format())
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// Rollup generates the verified package rollup report from the merged profile.
func (p *MergedProfile) Rollup(threshold float64) (*community.CoverageRollupReport, error) {
	data := p.Format()
	report, err := community.ParseCoverProfileBytes(data)
	if err != nil {
		return nil, fmt.Errorf("coverage: failed to synthesize rollup from merged profile: %w", err)
	}
	if threshold > 0 {
		report.ThresholdPct = threshold
		report.MeetsThreshold = report.CoveragePct >= threshold
	}
	return report, nil
}

// MergeShards combines multiple raw coverprofile byte slices into a single deduplicated MergedProfile.
func (m *Merger) MergeShards(shards ...[]byte) (*MergedProfile, error) {
	if len(shards) == 0 {
		return nil, ErrEmptyShardSet
	}

	var activeMode string
	if m.config.EnforceMode != "" {
		activeMode = m.config.EnforceMode
	}

	var parsedBlocks []*ProfileBlock

	for idx, shardData := range shards {
		if len(bytes.TrimSpace(shardData)) == 0 {
			continue
		}
		mode, blocks, err := m.ParseProfile(bytes.NewReader(shardData))
		if err != nil {
			return nil, fmt.Errorf("shard %d: %w", idx, err)
		}
		if mode != "" {
			if activeMode == "" {
				activeMode = mode
			} else if activeMode != mode {
				return nil, fmt.Errorf("%w: expected mode %q, shard %d declared %q",
					ErrInconsistentCoverageMode, activeMode, idx, mode)
			}
		}
		parsedBlocks = append(parsedBlocks, blocks...)
	}

	if activeMode == "" {
		activeMode = ModeAtomic
	}

	merged := NewMergedProfile(activeMode)
	for _, b := range parsedBlocks {
		if err := merged.AddBlock(b); err != nil {
			return nil, err
		}
	}

	return merged, nil
}

// MergeFiles reads and merges multiple coverprofile file paths from disk.
func (m *Merger) MergeFiles(paths ...string) (*MergedProfile, error) {
	if len(paths) == 0 {
		return nil, ErrEmptyShardSet
	}

	var shards [][]byte
	for _, p := range paths {
		if !fileutil.Exists(p) {
			return nil, fmt.Errorf("coverage: shard profile missing: %s", p)
		}
		data, err := fileutil.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("coverage: read %s: %w", p, err)
		}
		shards = append(shards, data)
	}
	return m.MergeShards(shards...)
}

// MergeGlob finds and merges coverprofiles matching a glob pattern (e.g. "coverage-*.out").
func (m *Merger) MergeGlob(pattern string) (*MergedProfile, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("coverage: invalid glob pattern %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: no files matched pattern %q", ErrEmptyShardSet, pattern)
	}
	sort.Strings(matches)
	return m.MergeFiles(matches...)
}
