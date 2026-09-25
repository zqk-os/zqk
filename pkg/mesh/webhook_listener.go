package mesh

import (
	"context"
	"net/http"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/transport"
)

// WebhookPayload represents a standard callback payload from an external media generator
// or a Tool Pod Mesh worker.
type WebhookPayload struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
	Error  string `json:"error,omitempty"`
}

// WebhookHandler returns an http.HandlerFunc that processes incoming webhooks
// from Tool Pods or external APIs and routes them to the Knowledge Kernel.
func WebhookHandler(store storage.ObjectStorageProvider) http.HandlerFunc {
	opts := transport.Options{
		HandlerName: "WebhookHandler",
	}

	return transport.NewHTTPHandler(opts, func(req transport.Request[WebhookPayload]) (transport.Response[map[string]any], error) {
		payload := req.Payload

		// A valid completion payload must have either a URL (from external generator)
		// or a "success" status (from Tool Pods).
		isValidCompletion := (payload.Status == objects.ObjectStatusCompleted && payload.URL != "") || payload.Status == "success"

		if isValidCompletion {
			if store != nil {
				updates := map[string]any{
					objects.FieldKeyStatus: objects.ObjectStatusCompleted,
				}
				if payload.URL != "" {
					updates["artifact_url"] = payload.URL
				}
				reqCtx := req.Context
				if reqCtx == nil {
					reqCtx = context.Background()
				}
				if err := store.Update(reqCtx, nil, payload.JobID, updates); err != nil {
					return transport.Response[map[string]any]{
						StatusCode: http.StatusInternalServerError,
					}, err
				}
			}
			return transport.Response[map[string]any]{
				StatusCode: http.StatusOK,
			}, nil
		}

		return transport.Response[map[string]any]{
			StatusCode: http.StatusUnprocessableEntity,
		}, nil
	})
}
