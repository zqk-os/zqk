package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Server provides an embedded local HTTP server for interactive web visual studio.
type Server struct {
	mu          sync.RWMutex
	httpServer  *http.Server
	listener    net.Listener
	projectRoot string
	indexer     *storage.PureGoIndexer
	logger      logging.Logger
	started     bool
}

// ServerOption configures the UI server.
type ServerOption func(*Server)

// WithLogger sets a custom logger on the UI server.
func WithLogger(logger logging.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithIndexer attaches an existing PureGoIndexer instance to the server.
func WithIndexer(indexer *storage.PureGoIndexer) ServerOption {
	return func(s *Server) {
		s.indexer = indexer
	}
}

// NewServer constructs an embedded Web Studio UI server.
func NewServer(projectRoot string, opts ...ServerOption) *Server {
	s := &Server{
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(""),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.indexer == nil {
		s.indexer = storage.NewPureGoIndexer()
		// Eagerly index the project root if available
		if projectRoot != "" {
			_, _ = s.indexer.IndexProjectDir(context.Background(), projectRoot)
		}
	}
	return s
}

// Start launches the HTTP server on addr (e.g. "127.0.0.1:0" for dynamic port or "127.0.0.1:8080").
func (s *Server) Start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("studio: server already started")
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("studio: failed to listen on %s: %w", addr, err)
	}
	s.listener = ln

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/objects", s.handleObjects)
	mux.HandleFunc("/api/objects/", s.handleObjectByID)

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.started = true

	goroutinelabels.NewGoroutine("studio_ui_http_serve", "serving studio UI HTTP traffic").
		AsControlPlane().
		StartSimple(func() {
			_ = s.httpServer.Serve(ln)
		})

	return nil
}

// Addr returns the resolved network address (host:port) that the server is listening on.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started || s.httpServer == nil {
		return nil
	}

	err := s.httpServer.Shutdown(ctx)
	s.started = false
	return err
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"projectRoot": s.projectRoot,
		"nodeCount":   s.indexer.NodeCount(),
	})
}

// GraphPayload defines the JSON response structure for graph visualization.
type GraphPayload struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode represents an entity in the visual graph.
type GraphNode struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

// GraphEdge represents a directed relationship in the visual graph.
type GraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	kinds := []string{
		"mission", "vision", "goal", "roadmap", "milestone",
		"backlog_item", "requirement", "criteria", "test_case", "priority_plan",
	}

	var nodes []GraphNode
	var edges []GraphEdge
	seenEdges := make(map[string]bool)

	for _, kind := range kinds {
		for _, node := range s.indexer.GetNodesByKind(kind) {
			nodes = append(nodes, GraphNode{
				ID:     node.ID,
				Kind:   node.Kind,
				Status: node.Status,
				Title:  node.Title,
			})

			for rel, targets := range node.References {
				for _, target := range targets {
					edgeKey := fmt.Sprintf("%s->%s:%s", node.ID, target, rel)
					if !seenEdges[edgeKey] {
						seenEdges[edgeKey] = true
						edges = append(edges, GraphEdge{
							Source:   node.ID,
							Target:   target,
							Relation: rel,
						})
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(GraphPayload{
		Nodes: nodes,
		Edges: edges,
	})
}

func (s *Server) handleObjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	kindFilter := r.URL.Query().Get("kind")
	var results []*storage.IndexedNode

	if kindFilter != "" {
		results = s.indexer.GetNodesByKind(kindFilter)
	} else {
		// Return sample across known kinds
		kinds := []string{"goal", "milestone", "backlog_item", "priority_plan"}
		for _, k := range kinds {
			results = append(results, s.indexer.GetNodesByKind(k)...)
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(results)
}

func (s *Server) handleObjectByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	id := strings.TrimPrefix(r.URL.Path, "/api/objects/")
	if id == "" {
		http.Error(w, `{"error": "missing object id"}`, http.StatusBadRequest)
		return
	}

	node, exists := s.indexer.GetNode(id)
	if !exists {
		http.Error(w, `{"error": "object not found"}`, http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(node)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(embeddedDashboardHTML))
}

const embeddedDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>ZQK Knowledge Kernel Studio</title>
  <style>
    :root {
      --bg: #0d1117;
      --card-bg: #161b22;
      --border: #30363d;
      --text: #c9d1d9;
      --text-muted: #8b949e;
      --accent: #58a6ff;
      --success: #3fb950;
      --warning: #d29922;
    }
    body {
      margin: 0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      background-color: var(--bg);
      color: var(--text);
    }
    header {
      padding: 16px 24px;
      background: var(--card-bg);
      border-bottom: 1px solid var(--border);
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
    h1 { margin: 0; font-size: 20px; font-weight: 600; color: #fff; }
    .status-badge {
      background: rgba(63, 185, 80, 0.15);
      color: var(--success);
      padding: 4px 10px;
      border-radius: 12px;
      font-size: 13px;
      font-weight: 500;
    }
    main {
      padding: 24px;
      display: grid;
      grid-template-columns: 1fr 340px;
      gap: 20px;
      height: calc(100vh - 120px);
    }
    .panel {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      overflow: hidden;
      display: flex;
      flex-direction: column;
    }
    .panel-header {
      padding: 12px 16px;
      border-bottom: 1px solid var(--border);
      font-size: 14px;
      font-weight: 600;
      color: #fff;
    }
    .graph-canvas {
      flex: 1;
      display: flex;
      align-items: center;
      justify-content: center;
      position: relative;
    }
    .node-list {
      flex: 1;
      overflow-y: auto;
      padding: 8px;
    }
    .node-card {
      padding: 10px 12px;
      margin-bottom: 8px;
      background: #21262d;
      border-radius: 6px;
      border-left: 3px solid var(--accent);
      cursor: pointer;
    }
    .node-id { font-weight: 600; font-size: 13px; color: #fff; }
    .node-kind { font-size: 11px; color: var(--text-muted); text-transform: uppercase; }
  </style>
</head>
<body>
  <header>
    <h1>⚡ ZQK Knowledge Kernel Visual Studio</h1>
    <div class="status-badge" id="live-indicator">Connected</div>
  </header>
  <main>
    <div class="panel">
      <div class="panel-header">Interactive Knowledge Graph DAG</div>
      <div class="graph-canvas" id="canvas-container">
        <svg id="dag-svg" width="100%" height="100%"></svg>
      </div>
    </div>
    <div class="panel">
      <div class="panel-header">Kernel Objects</div>
      <div class="node-list" id="nodes-container">
        <div style="padding: 16px; color: var(--text-muted);">Loading live DAG objects...</div>
      </div>
    </div>
  </main>
  <script>
    async function loadDAG() {
      try {
        const res = await fetch('/api/graph');
        const data = await res.json();
        const container = document.getElementById('nodes-container');
        if (!data.nodes || data.nodes.length === 0) {
          container.innerHTML = '<div style="padding: 16px; color: var(--text-muted);">No active objects in DAG.</div>';
          return;
        }
        container.innerHTML = data.nodes.map(function(n) {
          return '<div class="node-card">' +
            '<div class="node-kind">' + n.kind + ' &bull; ' + n.status + '</div>' +
            '<div class="node-id">' + n.id + '</div>' +
            '<div style="font-size: 12px; margin-top: 4px;">' + (n.title || '') + '</div>' +
          '</div>';
        }).join('');
      } catch (e) {
        console.error('Failed to load DAG', e);
      }
    }
    loadDAG();
  </script>
</body>
</html>
`
