package ambient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/ambient"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
)

func newIngestCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientIngestCommandBuilder()
	cmd.RunE = runAmbientIngest
	return cmd
}

func newAmbientIngestServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func runAmbientIngest(cmd *cobra.Command, _ []string) error {
	var flags clipkg.FlagBag
	port := flags.Int(cmd, "port")
	if err := flags.Err(); err != nil {
		return err
	}

	eventLogger := logging.GetLoggerFromContext(cmd.Context())
	hub := ambient.NewEventHub()

	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		var payload struct {
			Type      string `json:"type"`
			Payload   any    `json:"payload"`
			Timestamp string `json:"timestamp,omitempty"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		eventType := ambient.EventType(payload.Type)
		if eventType == "" {
			eventType = ambient.EventType("unknown")
		}

		ts := time.Now()
		if payload.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339, payload.Timestamp); err == nil {
				ts = parsed
			}
		}

		event := ambient.Event{
			Type:      eventType,
			Payload:   payload.Payload,
			Timestamp: ts,
		}

		if err := hub.Publish(cmd.Context(), event); err != nil {
			eventLogger.Logger().Error("Failed to publish ambient event", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintln(w, `{"status":"accepted"}`)
	})

	// Loopback-only by default (REQ-CEF-SEC-001 / CRIT-CEF-SEC-001B).
	// Set timeouts to prevent unbounded resource consumption (REQ-CEF-R2-SEC-HTTP-TIMEOUTS / CRIT-CEF-R2-SEC-HTTP-TIMEOUTS-A).
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	eventLogger.Logger().Info("Starting ambient event ingest API", logging.String("addr", addr))
	srv := newAmbientIngestServer(addr, mux)
	return srv.ListenAndServe()
}
