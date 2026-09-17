package billing

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lanceman/zqk/pkg/license"
)

// Blocklist defines the interface for revoking tokens.
type Blocklist interface {
	AddToBlocklist(subID string)
}

// StripeHandler handles incoming Stripe webhooks for the ZQK Toll Booth.
type StripeHandler struct {
	privateKey ed25519.PrivateKey
	blocklist  Blocklist
}

// NewStripeHandler creates a new Stripe webhook handler.
func NewStripeHandler(privateKey ed25519.PrivateKey, blocklist Blocklist) *StripeHandler {
	return &StripeHandler{
		privateKey: privateKey,
		blocklist:  blocklist,
	}
}

// StripePayload represents the minimal webhook payload we expect.
type StripePayload struct {
	Type           string `json:"type"`
	CustomerID     string `json:"customer_id"`
	SubscriptionID string `json:"subscription_id"`
}

// ServeHTTP processes the Stripe webhook.
func (h *StripeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload StripePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	switch payload.Type {
	case "customer.subscription.created":
		if payload.CustomerID == "" {
			http.Error(w, "missing customer_id", http.StatusBadRequest)
			return
		}

		claims := license.Claims{
			Features: []string{"relay_access"},
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   payload.CustomerID,
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(365 * 24 * time.Hour)),
			},
		}

		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		signedToken, err := token.SignedString(h.privateKey)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": signedToken})

	case "customer.subscription.deleted":
		if payload.CustomerID == "" {
			http.Error(w, "missing customer_id", http.StatusBadRequest)
			return
		}

		h.blocklist.AddToBlocklist(payload.CustomerID)
		w.WriteHeader(http.StatusOK)

	default:
		// Ignore other event types
		w.WriteHeader(http.StatusOK)
	}
}
