// BLI-STARTER-COMMUNITY-040 / PRI-STARTER-COMMUNITY-040 coverage elevation
package search

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const extraGoSrc = `package sample

type Server struct { Port int }
type Greeter interface { Greet(name string) string }
const DefaultGreeting = "Hello"
var ActiveCount = 42
func NewServer(port int) *Server { return &Server{Port: port} }
func (s *Server) Start() error { return nil }
`

func TestExtraSearchWalkerAndModes(t *testing.T) {
	if EstimateTokens("") != 0 || EstimateTokens("ab") != 1 {
		t.Fatal("tokens")
	}
	if EstimateMatchTokens(Match{File: "a.go", LineContent: "x", ContextBefore: []string{"b"}, ContextAfter: []string{"a"}}) <= 0 {
		t.Fatal("match tokens")
	}

	ws := t.TempDir()
	goPath := filepath.Join(ws, "a.go")
	if err := fileutil.WriteFile(goPath, []byte(extraGoSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(ws, "vendor"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(ws, "vendor", "skip.go"), []byte("package skip\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(ws, "zqk"), []byte("bin"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(ws, "idx.idx"), []byte("idx"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(ws, "notes.md"), []byte("HelloWorld indexing\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(ws)
	if err := eng.BuildTrigramIndex(SearchOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.LoadOrBuildIndex(SearchOptions{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := eng.Search(ctx, SearchOptions{Query: "Hello", CaseInsensitive: true, WordMatch: true, MaxMatches: 1, MaxTokens: 8, ContextLines: 1, UseIndex: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "Hello.*", Regex: true, CaseInsensitive: true, WordMatch: true, FileExtensions: []string{".go", "md"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "[", Regex: true}); err == nil {
		t.Fatal("invalid regex")
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "Start", Mode: ModeAST, ASTKind: "func", ASTReceiver: "Server", ContextLines: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "Server", Mode: ModeAST, ASTKind: "type"}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "ActiveCount", Mode: ModeAST, ASTKind: "var"}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "DefaultGreeting", Mode: ModeAST, ASTKind: "const"}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Search(ctx, SearchOptions{Query: "", Mode: ModeAST, ASTKind: "any"}); err != nil {
		t.Fatal(err)
	}

	src := []byte(extraGoSrc)
	got, err := ASTSearchFile(goPath, src, SearchOptions{Query: "Start", ASTKind: "func", ASTReceiver: "*Server", ContextLines: 1})
	if err != nil || len(got) == 0 {
		t.Fatalf("ASTSearchFile method = %v %v", got, err)
	}
	got, err = ASTSearchFile(goPath, src, SearchOptions{Query: "Greeter", ASTKind: "interface"})
	if err != nil || len(got) == 0 {
		t.Fatalf("interface = %v %v", got, err)
	}
	got, err = ASTSearchFile(goPath, src, SearchOptions{Query: "(?i)server", Regex: true, ASTKind: "struct"})
	if err != nil || len(got) == 0 {
		t.Fatalf("regex struct = %v %v", got, err)
	}
	if _, err := ASTSearchFile(filepath.Join(ws, "missing.go"), nil, SearchOptions{}); err == nil {
		t.Fatal("missing ast file")
	}

	hits := LineSearch("a.go", []byte("alpha\nHello World\nomega\n"), func(line []byte) (int, int, bool) {
		i := bytes.Index(line, []byte("Hello"))
		if i < 0 {
			return 0, 0, false
		}
		return i, i + 5, true
	}, 1)
	if len(hits) != 1 || hits[0].Column != 1 || len(hits[0].ContextBefore) == 0 {
		t.Fatalf("LineSearch = %#v", hits)
	}
	if !IsWordBoundary([]byte(" Hello "), 1, 6) || IsWordBoundary([]byte("xHello"), 1, 6) {
		t.Fatal("word boundary")
	}

	idx, err := BuildIndex([]string{goPath})
	if err != nil || idx == nil {
		t.Fatal(err)
	}
	_ = idx.FilterCandidates("Hello")
	cache := filepath.Join(ws, "trigram.idx")
	if err := idx.SaveToFile(cache); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndexFromFile(cache); err != nil {
		t.Fatal(err)
	}

	if IsBinary([]byte{1}) || !IsBinary([]byte{0}) || !IsBinaryFile(filepath.Join(ws, "missing")) {
		t.Fatal("binary")
	}
	if !MatchExtension("a.go", nil) || MatchExtension("a.bin", nil) || !MatchExtension("a.go", []string{"go"}) {
		t.Fatal("ext")
	}
	if !ShouldSkipDir("vendor", false, nil) || ShouldSkipDir(".agent", false, nil) || !ShouldSkipDir("cache", false, nil) {
		t.Fatal("skip")
	}
	_ = PackTrigram(1, 2, 3)
	_ = ExtractTrigrams([]byte("abcd"), true)
	_ = ExtractQueryTrigrams("abcd", false)
	files, err := CollectFiles(ws, SearchOptions{FileExtensions: []string{".go"}})
	if err != nil || len(files) == 0 {
		t.Fatalf("collect dir = %v %v", files, err)
	}

	long := strings.Repeat("HelloWorld ", 80)
	if err := fileutil.WriteFile(filepath.Join(ws, "long.md"), []byte(long+"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	res, err := eng.Search(ctx, SearchOptions{Query: "HelloWorld", MaxMatches: 1, FileExtensions: []string{".md"}})
	if err != nil || res == nil {
		t.Fatal(err)
	}
}

func TestExtraSearchCanceled(t *testing.T) {
	ws := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(ws, "a.go"), []byte(extraGoSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = NewEngine(ws).Search(ctx, SearchOptions{Query: "Hello"})
}

func TestExtraSearchIndexDiskAndWalkerGaps(t *testing.T) {
	ws := t.TempDir()
	goPath := filepath.Join(ws, "a.go")
	if err := fileutil.WriteFile(goPath, []byte(extraGoSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(ws, "a.bin")
	if err := fileutil.WriteFile(bin, []byte{0, 1, 2, 3}, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if !IsBinaryFile(bin) || IsBinaryFile(goPath) {
		t.Fatal("IsBinaryFile")
	}
	files, err := CollectFiles(goPath, SearchOptions{})
	if err != nil || len(files) != 1 {
		t.Fatalf("collect file = %v %v", files, err)
	}
	files, err = CollectFiles(bin, SearchOptions{})
	if err != nil || len(files) != 0 {
		t.Fatalf("collect binary = %v %v", files, err)
	}
	if _, err := CollectFiles(filepath.Join(ws, "missing-dir"), SearchOptions{}); err == nil {
		t.Fatal("missing collect")
	}
	_ = ExtractTrigrams([]byte("ab"), false)
	_ = ExtractQueryTrigrams("ab", true)
	if !ShouldSkipDir(".git", true, nil) || !ShouldSkipDir("node_modules", true, nil) || !ShouldSkipDir("bin", false, nil) {
		t.Fatal("always skip")
	}
	if ShouldSkipDir(".zqk", false, nil) || ShouldSkipDir(".agent", false, nil) {
		t.Fatal("whitelist")
	}
	if !ShouldSkipDir(".hidden", false, nil) || ShouldSkipDir(".hidden", true, nil) {
		t.Fatal("hidden")
	}
	if !ShouldSkipDir("tmp", false, []string{"tmp"}) || ShouldSkipDir("src", true, nil) {
		t.Fatal("custom/src")
	}

	eng := NewEngine(ws)
	if err := eng.BuildTrigramIndex(SearchOptions{Path: "a.go"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.BuildTrigramIndex(SearchOptions{Path: ws}); err != nil {
		t.Fatal(err)
	}
	eng2 := NewEngine(ws)
	if _, err := eng2.LoadOrBuildIndex(SearchOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng2.LoadOrBuildIndex(SearchOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndexFromFile(filepath.Join(ws, "missing.idx")); err == nil {
		t.Fatal("missing idx")
	}
	garbage := filepath.Join(ws, "bad.idx")
	if err := fileutil.WriteFile(garbage, []byte("not gob"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadIndexFromFile(garbage); err == nil {
		t.Fatal("bad gob")
	}

	idx, err := BuildIndex([]string{goPath})
	if err != nil {
		t.Fatal(err)
	}
	_ = idx.FilterCandidates("")
	_ = idx.FilterCandidates("ab")
	_ = idx.FilterCandidates("Hello")
	_ = idx.FilterCandidates("zzzzzzzz")
	if !IsWordBoundary([]byte("Hello"), 0, 5) || IsWordBoundary([]byte("Hello_"), 0, 5) {
		t.Fatal("boundary ends")
	}

	got, err := ASTSearchFile("bad.go", []byte("not go {"), SearchOptions{})
	if err != nil || got != nil {
		t.Fatalf("parse skip = %v %v", got, err)
	}
	if _, err := ASTSearchFile(goPath, []byte(extraGoSrc), SearchOptions{Query: "[", Regex: true}); err == nil {
		t.Fatal("bad ast regex")
	}
	got, err = ASTSearchFile(goPath, nil, SearchOptions{Query: "server", CaseInsensitive: true, ASTKind: "type"})
	if err != nil || len(got) == 0 {
		t.Fatalf("case ast = %v %v", got, err)
	}

	many := filepath.Join(ws, "many")
	if err := fileutil.MkdirAll(many, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 52; i++ {
		p := filepath.Join(many, fmt.Sprintf("f%02d.md", i))
		if err := fileutil.WriteFile(p, []byte("HelloWorld token\n"+strings.Repeat("x", 520)+"\n"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	eng3 := NewEngine(many)
	if _, err := eng3.Search(ctx, SearchOptions{Query: "HelloWorld", UseIndex: true, MaxMatches: 0, MaxTokens: -1, Path: many, WordMatch: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEngine(ws).Search(ctx, SearchOptions{Query: "Start", Mode: ModeAST, Path: "a.go"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewEngine(ws).Search(ctx, SearchOptions{Query: "Hello", Path: goPath, FileExtensions: []string{".go"}}); err != nil {
		t.Fatal(err)
	}
}
