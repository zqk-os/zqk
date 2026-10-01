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

	"github.com/zqk-os/zqk/pkg/acronyms"
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
	dashboard   *DashboardBuilder
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

// WithCustomAssetsDir configures a custom directory for live loading studio assets without recompilation.
func WithCustomAssetsDir(dir string) ServerOption {
	return func(s *Server) {
		if s.dashboard != nil {
			s.dashboard.WithCustomAssetsDir(dir)
		}
	}
}

// NewServer constructs an embedded Web Studio UI server.
func NewServer(projectRoot string, opts ...ServerOption) *Server {
	s := &Server{
		projectRoot: projectRoot,
		logger:      logging.GetLoggerFromProfile(""),
		dashboard:   NewDashboardBuilder(projectRoot),
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

// Dashboard returns the underlying DashboardBuilder for programmatic customization.
func (s *Server) Dashboard() *DashboardBuilder {
	return s.dashboard
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
	mux.HandleFunc("/assets/", s.handleAsset)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/objects", s.handleObjects)
	mux.HandleFunc("/api/objects/", s.handleObjectByID)
	mux.HandleFunc("/api/inbox", s.handleInbox)
	mux.HandleFunc("/api/inbox/ack", s.handleInboxAck)
	mux.HandleFunc("/api/inbox/respond", s.handleInboxRespond)
	mux.HandleFunc("/api/acronyms", s.handleAcronyms)
	mux.HandleFunc("/api/explain", s.handleExplain)

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

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	setNoCacheHeaders(w)
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
	ID              string              `json:"id"`
	Kind            string              `json:"kind"`
	Status          string              `json:"status"`
	Title           string              `json:"title"`
	CreatedAt       string              `json:"createdAt,omitempty"`
	UpdatedAt       string              `json:"updatedAt,omitempty"`
	StartDate       string              `json:"startDate,omitempty"`
	TargetDate      string              `json:"targetDate,omitempty"`
	DueDate         string              `json:"dueDate,omitempty"`
	EstimatedEffort string              `json:"estimatedEffort,omitempty"`
	References      map[string][]string `json:"references,omitempty"`
	WorkstreamRefs  []string            `json:"workstreamRefs,omitempty"`
}

// GraphEdge represents a directed relationship in the visual graph.
type GraphEdge struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Relation   string `json:"relation"`
	Structural bool   `json:"structural"`
}

func isStructuralRelation(rel string) bool {
	switch rel {
	case "workstream_ref", "workstream_refs", "from_workstream_ref", "to_workstream_ref",
		"goal_ref", "goal_refs",
		"priority_plan_ref", "plan_ref", "plan_refs", "roadmap_ref",
		"milestone_ref", "milestone_refs",
		"backlog_item_ref", "backlog_item_refs", "parent_ref", "parent_task_ref",
		"requirement_ref", "requirement_refs",
		"criteria_ref", "criteria_refs",
		"test_case_ref", "test_case_refs",
		"depends_on", "blocked_by", "blocking_ref":
		return true
	default:
		return false
	}
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	kinds := []string{
		"mission", "vision", "workstream", "goal", "roadmap", "milestone",
		"priority_plan", "backlog_item", "requirement", "criteria", "test_case",
		"decision", "agent_task",
	}

	nodes := make([]GraphNode, 0)
	edges := make([]GraphEdge, 0)
	seenEdges := make(map[string]bool)

	for _, kind := range kinds {
		for _, node := range s.indexer.GetNodesByKind(kind) {
			var ca, ua string
			if !node.CreatedAt.IsZero() {
				ca = node.CreatedAt.UTC().Format(time.RFC3339)
			}
			if !node.UpdatedAt.IsZero() {
				ua = node.UpdatedAt.UTC().Format(time.RFC3339)
			}
			var wsRefs []string
			for _, k := range []string{"workstream_refs", "workstream_ref", "from_workstream_ref", "to_workstream_ref"} {
				if refs, ok := node.References[k]; ok {
					for _, r := range refs {
						if r != "" {
							wsRefs = append(wsRefs, r)
						}
					}
				}
			}
			var sd, td, dd, eff string
			if node.Attributes != nil {
				if v, ok := node.Attributes["start_date"].(string); ok {
					sd = v
				}
				if v, ok := node.Attributes["target_date"].(string); ok {
					td = v
				}
				if v, ok := node.Attributes["due_date"].(string); ok {
					dd = v
				}
				if v, ok := node.Attributes["estimated_effort"].(string); ok {
					eff = v
				}
			}
			nodes = append(nodes, GraphNode{
				ID:              node.ID,
				Kind:            node.Kind,
				Status:          node.Status,
				Title:           node.Title,
				CreatedAt:       ca,
				UpdatedAt:       ua,
				StartDate:       sd,
				TargetDate:      td,
				DueDate:         dd,
				EstimatedEffort: eff,
				References:      node.References,
				WorkstreamRefs:  wsRefs,
			})

			for rel, targets := range node.References {
				structural := isStructuralRelation(rel)
				for _, target := range targets {
					edgeKey := fmt.Sprintf("%s->%s:%s", node.ID, target, rel)
					if !seenEdges[edgeKey] {
						seenEdges[edgeKey] = true
						edges = append(edges, GraphEdge{
							Source:     node.ID,
							Target:     target,
							Relation:   rel,
							Structural: structural,
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
	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	kindFilter := r.URL.Query().Get("kind")
	results := make([]*storage.IndexedNode, 0)

	if kindFilter != "" {
		results = s.indexer.GetNodesByKind(kindFilter)
	} else {
		kinds := []string{
			"mission", "vision", "workstream", "goal", "roadmap", "milestone",
			"priority_plan", "backlog_item", "requirement", "criteria", "test_case",
			"decision", "agent_task",
		}
		for _, k := range kinds {
			results = append(results, s.indexer.GetNodesByKind(k)...)
		}
	}

	if results == nil {
		results = make([]*storage.IndexedNode, 0)
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(results)
}

func (s *Server) handleObjectByID(w http.ResponseWriter, r *http.Request) {
	setNoCacheHeaders(w)
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
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if s.dashboard != nil {
		_, _ = w.Write([]byte(s.dashboard.RenderHTML()))
		return
	}
	_, _ = w.Write([]byte(NewDashboardBuilder(s.projectRoot).RenderHTML()))
}

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	setNoCacheHeaders(w)
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	builder := s.dashboard
	if builder == nil {
		builder = NewDashboardBuilder(s.projectRoot)
	}

	switch name {
	case "style.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(builder.resolveCSS()))
	case "app.js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(builder.resolveJS()))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAcronyms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	all := acronyms.ListAll()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"scheme_id": acronyms.KernelAcronymsSchemeID,
		"acronyms":  all,
		"count":     len(all),
	})
}

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		s.handleAcronyms(w, r)
		return
	}
	item, found := acronyms.Lookup(q)
	if !found {
		suggestions := acronyms.FindClosest(q)
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":       fmt.Sprintf("unknown acronym %q", q),
			"suggestions": suggestions,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(item)
}
