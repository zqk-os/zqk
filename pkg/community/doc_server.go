package community

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DocServerConfig defines options for the embedded documentation server.
type DocServerConfig struct {
	Addr         string `json:"addr"`
	DocRoot      string `json:"doc_root"`
	EnableSearch bool   `json:"enable_search"`
}

// DocServer is an in-tree embedded documentation server for offline browsing.
type DocServer struct {
	cfg       DocServerConfig
	server    *http.Server
	listener  net.Listener
	mu        sync.RWMutex
	actualURL string
	running   bool
}

// DocEntry represents metadata about a documentation file.
type DocEntry struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Size        int64  `json:"size"`
	RelativeURL string `json:"relative_url"`
}

// NewDocServer initializes a new DocServer instance.
func NewDocServer(cfg DocServerConfig) (*DocServer, error) {
	cleanRoot := filepath.Clean(cfg.DocRoot)
	if !fileutil.Exists(cleanRoot) {
		return nil, fmt.Errorf("doc root does not exist: %s", cleanRoot)
	}

	cfg.DocRoot = cleanRoot
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}

	ds := &DocServer{
		cfg: cfg,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", ds.handleHealthz)
	mux.HandleFunc("/api/specs", ds.handleSpecs)
	mux.HandleFunc("/api/search", ds.handleSearch)
	mux.HandleFunc("/docs/", ds.handleDoc)
	mux.HandleFunc("/", ds.handleIndex)

	ds.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return ds, nil
}

// Start launches the documentation server on the configured address.
func (s *DocServer) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("doc server is already running")
	}

	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", s.cfg.Addr, err)
	}

	s.listener = ln
	s.actualURL = fmt.Sprintf("http://%s", ln.Addr().String())
	s.running = true
	s.mu.Unlock()

	goroutinelabels.NewGoroutine("community.doc_server", "serving embedded HTTP documentation").StartSimple(func() {
		_ = s.server.Serve(ln)
	})

	return nil
}

// Stop gracefully shuts down the server.
func (s *DocServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// URL returns the base URL of the running server.
func (s *DocServer) URL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.actualURL
}

func (s *DocServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *DocServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	docs, err := s.scanDocs()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to scan documents: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html><html><head><title>ZQK Offline Documentation</title>")
	sb.WriteString("<style>body{font-family:sans-serif;margin:40px;line-height:1.6}ul{padding-left:20px}li{margin-bottom:8px}a{color:#0969da;text-decoration:none}a:hover{text-decoration:underline}</style>")
	sb.WriteString("</head><body><h1>ZQK Offline Documentation & Spec Browser</h1>")
	sb.WriteString("<p>Locally embedded documentation and architecture specifications.</p><ul>")

	for _, d := range docs {
		sb.WriteString(fmt.Sprintf("<li><a href=\"%s\">%s</a> <small>(%d bytes)</small></li>",
			html.EscapeString(d.RelativeURL),
			html.EscapeString(d.Title),
			d.Size,
		))
	}
	sb.WriteString("</ul></body></html>")
	_, _ = w.Write([]byte(sb.String()))
}

func (s *DocServer) handleDoc(w http.ResponseWriter, r *http.Request) {
	relPath := strings.TrimPrefix(r.URL.Path, "/docs/")
	cleanRel := filepath.Clean("/" + relPath)
	targetPath := filepath.Join(s.cfg.DocRoot, cleanRel)

	// Path traversal protection
	relToRoot, err := filepath.Rel(s.cfg.DocRoot, targetPath)
	if err != nil || strings.HasPrefix(relToRoot, "..") {
		http.Error(w, "Forbidden: path traversal rejected", http.StatusForbidden)
		return
	}

	if !fileutil.Exists(targetPath) {
		http.NotFound(w, r)
		return
	}

	info, err := fileutil.Stat(targetPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	content, err := fileutil.ReadFile(targetPath)
	if err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html><html><head><title>")
	sb.WriteString(html.EscapeString(filepath.Base(targetPath)))
	sb.WriteString("</title><style>body{font-family:sans-serif;margin:40px;line-height:1.6}pre{background:#f6f8fa;padding:16px;border-radius:6px;overflow-x:auto}</style></head><body>")
	sb.WriteString("<p><a href=\"/\">&larr; Back to Index</a></p><pre>")
	sb.WriteString(html.EscapeString(string(content)))
	sb.WriteString("</pre></body></html>")

	_, _ = w.Write([]byte(sb.String()))
}

func (s *DocServer) handleSpecs(w http.ResponseWriter, r *http.Request) {
	docs, err := s.scanDocs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"total": len(docs),
		"docs":  docs,
	})
}

func (s *DocServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	docs, err := s.scanDocs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	queryLower := strings.ToLower(q)
	var matched []DocEntry

	for _, d := range docs {
		targetPath := filepath.Join(s.cfg.DocRoot, d.Path)
		content, err := fileutil.ReadFile(targetPath)
		if err == nil {
			if strings.Contains(strings.ToLower(string(content)), queryLower) ||
				strings.Contains(strings.ToLower(d.Title), queryLower) {
				matched = append(matched, d)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"query":   q,
		"matches": len(matched),
		"results": matched,
	})
}

func (s *DocServer) scanDocs() ([]DocEntry, error) {
	var docs []DocEntry
	entries, err := fileutil.ReadDir(s.cfg.DocRoot)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".html") {
			fullPath := filepath.Join(s.cfg.DocRoot, name)
			info, err := fileutil.Stat(fullPath)
			if err != nil {
				continue
			}

			title := strings.TrimSuffix(name, filepath.Ext(name))
			title = strings.ReplaceAll(title, "_", " ")
			title = strings.ReplaceAll(title, "-", " ")

			docs = append(docs, DocEntry{
				Path:        name,
				Title:       title,
				Size:        info.Size(),
				RelativeURL: fmt.Sprintf("/docs/%s", name),
			})
		}
	}

	return docs, nil
}
