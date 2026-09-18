package search

import (
	"context"
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func createTestWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Sample Go file
	goCode := `package sample

import "fmt"

type Server struct {
	Port int
}

type Greeter interface {
	Greet(name string) string
}

const DefaultGreeting = "Hello, World!"
var ActiveCount = 42

func NewServer(port int) *Server {
	return &Server{Port: port}
}

func (s *Server) Start() error {
	fmt.Println("Server starting...")
	return nil
}

func (s *Server) Stop() {
	fmt.Println("Server stopped.")
}
`
	if err := fileutil.WriteFile(filepath.Join(dir, "server.go"), []byte(goCode), 0o644); err != nil {
		t.Fatalf("failed writing server.go: %v", err)
	}

	// Sample text file
	readme := `# Sample Project
This is a test project demonstrating native in-process code search.
Trigram indexing speeds up searches across millions of characters.
`
	if err := fileutil.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("failed writing README.md: %v", err)
	}

	// Sample binary file (with null bytes)
	binaryData := []byte{0x7f, 'E', 'L', 'F', 0x00, 0x01, 0x02, 0x00}
	if err := fileutil.WriteFile(filepath.Join(dir, "binary.bin"), binaryData, 0o644); err != nil {
		t.Fatalf("failed writing binary.bin: %v", err)
	}

	return dir
}

func TestTextSearch(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	res, err := engine.Search(context.Background(), SearchOptions{
		Query: "indexing",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("expected matches for 'indexing', got 0")
	}
	if res.Matches[0].File != "README.md" {
		t.Errorf("expected README.md match, got %s", res.Matches[0].File)
	}
}

func TestTextSearchCaseInsensitive(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	res, err := engine.Search(context.Background(), SearchOptions{
		Query:           "sAmPlE",
		CaseInsensitive: true,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("expected matches for case-insensitive 'sAmPlE'")
	}
}

func TestRegexSearch(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	// Valid regex
	res, err := engine.Search(context.Background(), SearchOptions{
		Query: `func\s+\(s\s+\*Server\)\s+\w+`,
		Regex: true,
	})
	if err != nil {
		t.Fatalf("Regex search failed: %v", err)
	}
	if len(res.Matches) != 2 {
		t.Errorf("expected 2 method matches for Server, got %d", len(res.Matches))
	}

	// Invalid regex should fail closed gracefully
	_, err = engine.Search(context.Background(), SearchOptions{
		Query: `[unclosed-regex`,
		Regex: true,
	})
	if err == nil {
		t.Fatalf("expected error for unclosed regex, got nil")
	}
}

func TestTrigramIndex(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	err := engine.BuildTrigramIndex(SearchOptions{})
	if err != nil {
		t.Fatalf("BuildTrigramIndex failed: %v", err)
	}

	res, err := engine.Search(context.Background(), SearchOptions{
		Query:    "demonstrating",
		UseIndex: true,
	})
	if err != nil {
		t.Fatalf("Search with index failed: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("expected matches for 'demonstrating' via trigram index")
	}
}

func TestASTSearch(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	// Search for all functions
	res, err := engine.Search(context.Background(), SearchOptions{
		Mode:    ModeAST,
		ASTKind: "func",
	})
	if err != nil {
		t.Fatalf("AST func search failed: %v", err)
	}
	if len(res.Matches) < 3 { // NewServer, Start, Stop
		t.Errorf("expected >= 3 functions/methods, got %d", len(res.Matches))
	}

	// Search for methods on Server receiver
	resRecv, err := engine.Search(context.Background(), SearchOptions{
		Mode:        ModeAST,
		ASTReceiver: "Server",
	})
	if err != nil {
		t.Fatalf("AST receiver search failed: %v", err)
	}
	if len(resRecv.Matches) != 2 {
		t.Errorf("expected 2 methods on Server receiver, got %d", len(resRecv.Matches))
	}

	// Search for structs
	resStruct, err := engine.Search(context.Background(), SearchOptions{
		Mode:    ModeAST,
		ASTKind: "struct",
		Query:   "Server",
	})
	if err != nil {
		t.Fatalf("AST struct search failed: %v", err)
	}
	if len(resStruct.Matches) != 1 {
		t.Errorf("expected 1 struct match for Server, got %d", len(resStruct.Matches))
	}
	if resStruct.Matches[0].SymbolKind != "struct" {
		t.Errorf("expected SymbolKind 'struct', got %s", resStruct.Matches[0].SymbolKind)
	}

	// Search for interfaces
	resIface, err := engine.Search(context.Background(), SearchOptions{
		Mode:    ModeAST,
		ASTKind: "interface",
	})
	if err != nil {
		t.Fatalf("AST interface search failed: %v", err)
	}
	if len(resIface.Matches) != 1 || resIface.Matches[0].SymbolName != "Greeter" {
		t.Errorf("expected 1 interface match for Greeter, got %v", resIface.Matches)
	}
}

func TestBinaryFileSkipped(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	res, err := engine.Search(context.Background(), SearchOptions{
		Query: "ELF",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	for _, m := range res.Matches {
		if filepath.Base(m.File) == "binary.bin" {
			t.Errorf("binary file binary.bin should not have been matched")
		}
	}
}

func TestTokenBudgetCap(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	// Set tiny token budget
	res, err := engine.Search(context.Background(), SearchOptions{
		Query:     "e",
		MaxTokens: 15,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if !res.Truncated {
		t.Errorf("expected results to be truncated under tiny token budget")
	}
	if res.TruncateReason != "max_tokens" {
		t.Errorf("expected TruncateReason 'max_tokens', got %s", res.TruncateReason)
	}
}

func TestMaxMatchesCap(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	res, err := engine.Search(context.Background(), SearchOptions{
		Query:      "e",
		MaxMatches: 2,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(res.Matches) > 2 {
		t.Errorf("expected at most 2 matches, got %d", len(res.Matches))
	}
	if !res.Truncated {
		t.Errorf("expected results to be truncated by MaxMatches")
	}
	if res.TruncateReason != "max_matches" {
		t.Errorf("expected TruncateReason 'max_matches', got %s", res.TruncateReason)
	}
}

func TestZqkProcessDirCollected(t *testing.T) {
	ws := t.TempDir()
	procDir := filepath.Join(ws, ".zqk", "process", "backlog_items")
	if err := fileutil.EnsureDir(procDir); err != nil {
		t.Fatal(err)
	}
	bliFile := filepath.Join(procDir, "BLI-1.yaml")
	if err := fileutil.WriteFile(bliFile, []byte("id: BLI-1\ntitle: Test Item\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cacheDir := filepath.Join(ws, ".zqk", "cache")
	if err := fileutil.EnsureDir(cacheDir); err != nil {
		t.Fatal(err)
	}
	cacheFile := filepath.Join(cacheDir, "cache.json")
	if err := fileutil.WriteFile(cacheFile, []byte("cached data"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := CollectFiles(ws, SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	foundBLI := false
	foundCache := false
	for _, f := range files {
		if filepath.Base(f) == "BLI-1.yaml" {
			foundBLI = true
		}
		if filepath.Base(f) == "cache.json" {
			foundCache = true
		}
	}
	if !foundBLI {
		t.Errorf("expected BLI-1.yaml to be collected, got %v", files)
	}
	if foundCache {
		t.Errorf("expected cache.json to be ignored, got %v", files)
	}
}

func TestTrigramIndexDiskCache(t *testing.T) {
	ws := createTestWorkspace(t)
	cacheFile := filepath.Join(ws, "cache", "trigram.idx")
	if err := fileutil.EnsureDir(filepath.Dir(cacheFile)); err != nil {
		t.Fatalf("failed ensuring dir: %v", err)
	}

	files, err := CollectFiles(ws, SearchOptions{})
	if err != nil {
		t.Fatalf("failed collecting files: %v", err)
	}

	idx, err := BuildIndex(files)
	if err != nil {
		t.Fatalf("failed building index: %v", err)
	}

	if err := idx.SaveToFile(cacheFile); err != nil {
		t.Fatalf("failed saving index: %v", err)
	}

	loaded, err := LoadIndexFromFile(cacheFile)
	if err != nil {
		t.Fatalf("failed loading index: %v", err)
	}

	candidates := loaded.FilterCandidates("demonstrating")
	if len(candidates) == 0 {
		t.Fatalf("expected candidates for 'demonstrating' from loaded index")
	}
}

func TestSearchUnder15ms(t *testing.T) {
	ws := createTestWorkspace(t)
	engine := NewEngine(ws)

	if err := engine.BuildTrigramIndex(SearchOptions{}); err != nil {
		t.Fatalf("failed building index: %v", err)
	}

	res, err := engine.Search(context.Background(), SearchOptions{
		Query:    "demonstrating",
		UseIndex: true,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(res.Matches) == 0 {
		t.Fatalf("expected matches, got 0")
	}
	if res.DurationMs > 15.0 {
		t.Errorf("expected search duration <= 15ms, got %.2fms", res.DurationMs)
	}
}
