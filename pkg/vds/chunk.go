package vds

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Chunk is one independently verifiable unit of work.
type Chunk struct {
	ChunkID           string   `yaml:"chunk_id" json:"chunk_id"`
	Stage             string   `yaml:"stage" json:"stage"`
	Claim             string   `yaml:"claim" json:"claim"`
	WorkObjectRef     string   `yaml:"work_object_ref" json:"work_object_ref,omitempty"`
	RubricRef         string   `yaml:"rubric_ref" json:"rubric_ref"`
	DSLChecks         []string `yaml:"dsl_checks" json:"dsl_checks"`
	EvidenceRefs      []string `yaml:"evidence_refs" json:"evidence_refs"`
	GateIntent        string   `yaml:"gate_intent" json:"gate_intent,omitempty"`
	GateDesign        string   `yaml:"gate_design" json:"gate_design,omitempty"`
	GateImplement     string   `yaml:"gate_implement" json:"gate_implement,omitempty"`
	GateIntegrate     string   `yaml:"gate_integrate" json:"gate_integrate,omitempty"`
	GateOperate       string   `yaml:"gate_operate" json:"gate_operate,omitempty"`
	IndependentVerify string   `yaml:"independent_verify" json:"independent_verify"`
	WaiverRef         string   `yaml:"waiver_ref" json:"waiver_ref,omitempty"`
	Notes             string   `yaml:"notes" json:"notes,omitempty"`
}

// ChunkFile is the on-disk YAML document agents edit.
type ChunkFile struct {
	Schema string  `yaml:"schema" json:"schema,omitempty"`
	Chunks []Chunk `yaml:"chunks" json:"chunks"`
}

// LoadChunks reads a chunk YAML file.
func LoadChunks(path string) ([]Chunk, error) {
	f, err := loadChunkFile(path)
	if err != nil {
		return nil, err
	}
	return f.Chunks, nil
}

func loadChunkFile(path string) (*ChunkFile, error) {
	b, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, errfmt.Errorf("vds: read chunks %s: %w", path, err)
	}
	var f ChunkFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, errfmt.Errorf("vds: parse chunks: %w", err)
	}
	if len(f.Chunks) == 0 {
		return nil, errfmt.Errorf("vds: no chunks in %s", path)
	}
	if strings.TrimSpace(f.Schema) == "" {
		f.Schema = "zqk_vds_chunks_v1"
	}
	return &f, nil
}

// SaveChunks writes a chunk file (preserves schema; stable field order via struct tags).
func SaveChunks(path string, chunks []Chunk) error {
	if len(chunks) == 0 {
		return errfmt.Errorf("vds: refuse to save empty chunks to %s", path)
	}
	f := ChunkFile{Schema: "zqk_vds_chunks_v1", Chunks: chunks}
	b, err := yaml.Marshal(&f)
	if err != nil {
		return errfmt.Errorf("vds: marshal chunks: %w", err)
	}
	if err := fileutil.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errfmt.Errorf("vds: mkdir chunks dir: %w", err)
	}
	if err := fileutil.WriteFile(path, b, 0o644); err != nil {
		return errfmt.Errorf("vds: write chunks %s: %w", path, err)
	}
	return nil
}

// ApplyIndependentVerifyYes sets independent_verify=yes on named chunks and saves the file.
// Prefers a surgical text edit so comments and hand formatting are preserved; falls back to
// full YAML marshal if the surgical pass cannot locate a chunk's independent_verify line.
// Returns how many chunks were updated.
func ApplyIndependentVerifyYes(path string, chunkIDs []string) (int, error) {
	f, err := loadChunkFile(path)
	if err != nil {
		return 0, err
	}
	want := make(map[string]struct{}, len(chunkIDs))
	for _, id := range chunkIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			want[id] = struct{}{}
		}
	}
	if len(want) == 0 {
		return 0, nil
	}
	toSet := make(map[string]struct{})
	for i := range f.Chunks {
		id := f.Chunks[i].ChunkID
		if _, ok := want[id]; !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(f.Chunks[i].IndependentVerify), "yes") {
			continue
		}
		toSet[id] = struct{}{}
	}
	if len(toSet) == 0 {
		return 0, nil
	}

	raw, err := fileutil.ReadFile(path)
	if err != nil {
		return 0, errfmt.Errorf("vds: read chunks %s: %w", path, err)
	}
	wantN := len(toSet)
	updated, n, ok := surgicalSetIndependentVerifyYes(string(raw), toSet)
	if ok && n == wantN {
		if err := fileutil.WriteFile(path, []byte(updated), 0o644); err != nil {
			return 0, errfmt.Errorf("vds: write chunks %s: %w", path, err)
		}
		return n, nil
	}

	// Fallback: full rewrite (may drop comments).
	for i := range f.Chunks {
		if _, ok := toSet[f.Chunks[i].ChunkID]; ok {
			f.Chunks[i].IndependentVerify = "yes"
		}
	}
	schema := strings.TrimSpace(f.Schema)
	if schema == "" {
		schema = "zqk_vds_chunks_v1"
	}
	out := ChunkFile{Schema: schema, Chunks: f.Chunks}
	b, err := yaml.Marshal(&out)
	if err != nil {
		return 0, errfmt.Errorf("vds: marshal chunks: %w", err)
	}
	if err := fileutil.WriteFile(path, b, 0o644); err != nil {
		return 0, errfmt.Errorf("vds: write chunks %s: %w", path, err)
	}
	return len(toSet), nil
}

// surgicalSetIndependentVerifyYes rewrites independent_verify lines under matching chunk_id blocks.
func surgicalSetIndependentVerifyYes(src string, chunkIDs map[string]struct{}) (string, int, bool) {
	lines := strings.Split(src, "\n")
	n := 0
	currentID := ""
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		// YAML list items look like "- chunk_id: …"
		keyLine := strings.TrimSpace(strings.TrimPrefix(trim, "-"))
		if strings.HasPrefix(keyLine, "chunk_id:") {
			rest := strings.TrimSpace(strings.TrimPrefix(keyLine, "chunk_id:"))
			rest = strings.Trim(rest, `"'`)
			currentID = rest
			continue
		}
		if currentID == "" {
			continue
		}
		if _, want := chunkIDs[currentID]; !want {
			continue
		}
		if !strings.HasPrefix(trim, "independent_verify:") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + "independent_verify: yes"
		n++
		delete(chunkIDs, currentID)
		currentID = ""
	}
	if len(chunkIDs) != 0 {
		return src, n, false
	}
	return strings.Join(lines, "\n"), n, true
}

// ResolveChunksPath picks --file or default under project root.
func ResolveChunksPath(projectRoot, fileFlag string) string {
	f := strings.TrimSpace(fileFlag)
	if f == "" {
		return filepath.Join(projectRoot, DefaultChunksRel)
	}
	if filepath.IsAbs(f) {
		return f
	}
	return filepath.Join(projectRoot, f)
}

// NormalizeDSLChecks accepts semicolon-separated strings or list elements.
func NormalizeDSLChecks(in []string) []string {
	var out []string
	for _, raw := range in {
		for part := range strings.SplitSeq(raw, ";") {
			p := strings.TrimSpace(part)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// IsDoneValue reports yes/na style completion cells.
func IsDoneValue(v string, done []string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return false
	}
	if len(done) == 0 {
		return v == "yes" || v == "na"
	}
	for _, d := range done {
		if v == strings.ToLower(strings.TrimSpace(d)) {
			return true
		}
	}
	return false
}

// KnownStage reports whether stage is in the rigid spine.
func KnownStage(stage string) bool {
	s := strings.TrimSpace(stage)
	for _, id := range StageIDs {
		if s == id {
			return true
		}
	}
	return false
}
