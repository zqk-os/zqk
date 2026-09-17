package observer

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50
)

// Query is a scoped live AST lookup. It never walks the whole repo:
// name searches stay in pkg/ and cmd/, and a path pin walks only that file or dir.
type Query struct {
	Root  string
	Path  string
	Name  string
	Kind  string
	Limit int
}

var skipDirNames = map[string]struct{}{
	".git":         {},
	".zqk":         {},
	"vendor":       {},
	"node_modules": {},
	"testdata":     {},
	"dist":         {},
	"bin":          {},
}

// Search extracts matching Go entities from Root. Name or Path is required.
// File paths on returned entities are slash-separated and relative to Root.
func Search(ctx context.Context, q Query) ([]Entity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	q.Root = strings.TrimSpace(q.Root)
	q.Path = strings.TrimSpace(q.Path)
	q.Name = strings.TrimSpace(q.Name)
	q.Kind = strings.TrimSpace(q.Kind)
	if q.Root == "" {
		q.Root = "."
	}
	if q.Name == "" && q.Path == "" {
		return nil, fmt.Errorf("observer search requires name or path")
	}
	if q.Limit <= 0 {
		q.Limit = defaultSearchLimit
	}
	if q.Limit > maxSearchLimit {
		q.Limit = maxSearchLimit
	}

	absRoot, err := filepath.Abs(q.Root)
	if err != nil {
		return nil, err
	}
	files, err := collectQueryFiles(ctx, absRoot, q.Path)
	if err != nil {
		return nil, err
	}

	needle := []byte(strings.ToLower(q.Name))
	kind := strings.ToLower(q.Kind)
	ex := GoExtractor{}
	out := make([]Entity, 0, q.Limit)
	for _, absFile := range files {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		rel, err := filepath.Rel(absRoot, absFile)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		content, err := fileutil.ReadFile(absFile)
		if err != nil {
			continue
		}
		if len(needle) > 0 && !bytes.Contains(bytes.ToLower(content), needle) {
			continue
		}
		ents, err := ex.ExtractFile(ctx, rel, content)
		if err != nil {
			continue
		}
		for _, ent := range ents {
			if !entityMatches(ent, q.Name, kind) {
				continue
			}
			ent.File = rel
			out = append(out, ent)
			if len(out) >= q.Limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// FormatHit is one prompt/tool line. Keep it compact so local doers can scan it.
func FormatHit(e Entity) string {
	sig := strings.TrimSpace(e.Signature)
	if sig == "" {
		sig = e.Name
	}
	return fmt.Sprintf("- `%s` (%s) `%s:%d` — `%s`", e.Name, e.Kind, e.File, e.Line, sig)
}

func entityMatches(e Entity, name, kind string) bool {
	if kind != "" && !strings.EqualFold(e.Kind, kind) {
		return false
	}
	if name == "" {
		return true
	}
	n := strings.ToLower(name)
	return strings.Contains(strings.ToLower(e.Name), n) ||
		strings.Contains(strings.ToLower(e.Receiver), n) ||
		strings.Contains(strings.ToLower(e.File), n)
}

func collectQueryFiles(ctx context.Context, absRoot, relPath string) ([]string, error) {
	if relPath != "" {
		target, err := paths.ProjectPath(absRoot, relPath)
		if err != nil {
			return nil, err
		}
		info, err := fileutil.Stat(target)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if strings.EqualFold(filepath.Ext(target), goFileSuffix) {
				return []string{target}, nil
			}
			return nil, fmt.Errorf("observer path is not a Go file: %s", relPath)
		}
		return collectGoFiles(ctx, target)
	}
	var files []string
	for _, sub := range []string{"pkg", "cmd"} {
		dir := filepath.Join(absRoot, sub)
		info, err := fileutil.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		found, err := collectGoFiles(ctx, dir)
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	return files, nil
}

func collectGoFiles(ctx context.Context, dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			name := d.Name()
			if _, skip := skipDirNames[name]; skip {
				return filepath.SkipDir
			}
			if name != "." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), goFileSuffix) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

var observerStopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "that": {},
	"this": {}, "into": {}, "onto": {}, "add": {}, "fix": {}, "make": {},
	objects.FieldKeyUse: {}, "via": {}, "task": {}, "item": {}, objects.FieldKeyCode: {}, "file": {},
	"when": {}, "then": {}, "than": {}, "over": {}, "after": {}, "before": {},
	"must": {}, "should": {}, "will": {}, "have": {}, "been": {}, "does": {},
	"check": {}, "ensure": {}, "update": {}, "create": {}, "implement": {},
	"support": {}, "handle": {}, "process": {}, "enable": {}, "disable": {},
}

// SearchTokens pulls likely symbol/package words from a task title.
func SearchTokens(title string) []string {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	var buf strings.Builder
	flush := func() {
		tok := strings.ToLower(strings.TrimSpace(buf.String()))
		buf.Reset()
		if len(tok) < 4 {
			return
		}
		if _, stop := observerStopWords[tok]; stop {
			return
		}
		if _, ok := seen[tok]; ok {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			buf.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// InferSourcePath returns the first pkg/ or cmd/ path fragment in text.
func InferSourcePath(text string) string {
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, "`\"'")
		if strings.HasPrefix(field, "pkg/") || strings.HasPrefix(field, "cmd/") {
			return strings.TrimRight(field, ".,;:")
		}
	}
	return ""
}
