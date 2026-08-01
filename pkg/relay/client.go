package relay

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/lanceman/zqk/pkg/license"
)

var (
	ErrUnauthorized = errors.New("unauthorized: invalid or missing license")
	ErrConnection   = errors.New("failed to connect to sovereign relay")
)

// MessageHandler is a callback invoked when the relay receives a webhook from the outside world.
type MessageHandler func(ctx context.Context, payload []byte) error

// Client represents the local daemon's connection to the public Sovereign Relay (e.g., relay.zqkos.com).
type Client interface {
	// Connect establishes a persistent, secure tunnel to the remote relay.
	// It authenticates using the provided cryptographic JWT license.
	Connect(ctx context.Context, jwtToken string) error

	// Listen blocks and routes incoming messages from the tunnel to the handler.
	Listen(ctx context.Context, handler MessageHandler) error

	// Disconnect safely tears down the tunnel.
	Disconnect(ctx context.Context) error
}

// clientImpl is the standard implementation of the relay Client.
type clientImpl struct {
	remoteURL string
	validator *license.Validator
	connected bool
}

// NewClient creates a new Sovereign Relay client.
func NewClient(remoteURL string, validator *license.Validator) Client {
	return &clientImpl{
		remoteURL: remoteURL,
		validator: validator,
	}
}

// Connect verifies the license offline before attempting to dial out to the relay server.
func (c *clientImpl) Connect(ctx context.Context, jwtToken string) error {
	// 1. Offline Cryptographic Verification (Toll Booth Enforcement)
	// We require the "relay_access" feature flag in the user's Stripe subscription.
	_, err := c.validator.RequireFeature(jwtToken, "relay_access")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}

	// 2. Establish Tunnel (Mocked for now)
	// In production, this would dial a wss:// websocket to c.remoteURL
	log.Printf("[Relay] Authenticated via JWT. Establishing tunnel to %s", c.remoteURL)
	c.connected = true

	return nil
}

func (c *clientImpl) Listen(ctx context.Context, handler MessageHandler) error {
	if !c.connected {
		return fmt.Errorf("%w: must connect first", ErrConnection)
	}

	// Mock listening loop. In production, this reads from the websocket.
	<-ctx.Done()
	return ctx.Err()
}

func (c *clientImpl) Disconnect(ctx context.Context) error {
	c.connected = false
	log.Printf("[Relay] Disconnected from %s", c.remoteURL)
	return nil
}
