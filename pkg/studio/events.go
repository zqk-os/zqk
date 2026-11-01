package studio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// StartWithPortFallback attempts to start the server on ports from startPort to endPort inclusive.
// Returns the actual port selected on success.
func (s *Server) StartWithPortFallback(host string, startPort, endPort int) (int, error) {
	if startPort > endPort {
		return 0, fmt.Errorf("studio: invalid port range %d-%d", startPort, endPort)
	}
	for p := startPort; p <= endPort; p++ {
		addr := fmt.Sprintf("%s:%d", host, p)
		if err := s.Start(addr); err == nil {
			return p, nil
		}
	}
	return 0, fmt.Errorf("studio: all ports %d-%d are already in use", startPort, endPort)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	initialPayload := map[string]any{
		"event":     "connected",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"nodeCount": s.indexer.NodeCount(),
	}
	data, err := json.Marshal(initialPayload)
	if err != nil {
		http.Error(w, "Failed to encode initial payload", http.StatusInternalServerError)
		return
	}
	if _, err := fmt.Fprintf(w, "event: init\ndata: %s\n\n", data); err != nil {
		return
	}
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pulse := map[string]any{
				"event":     "pulse",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
				"nodeCount": s.indexer.NodeCount(),
			}
			pulseData, err := json.Marshal(pulse)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", pulseData); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
