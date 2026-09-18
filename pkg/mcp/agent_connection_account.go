// Package mcp: agent connection account creation (BLI-OBS-003).
// When an unregistered agent connects, MCP notifies observer; a registered callback
// can create an account object and assign roles so the agent is self-bootstrapped.

package mcp

import (
	"context"
	"regexp"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
	accountEnum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/accounts"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
)

const (
	maxUsernameLen   = 64
	defaultAgentRole = "developer"
)

var nonUsernameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// accountCreatedNotificationMessage returns the message sent to the new agent after account creation (BLI-860).
func accountCreatedNotificationMessage(accountID string) string {
	return "Account created: " + accountID + ", please re-initialize"
}

// RegisterAgentConnectionAccountCreation registers a connection-event callback that
// creates an account object for new agents (BLI-OBS-003). Call once when the MCP server
// has storage available (e.g. from cmd/zqk/mcp runServe after SetStorageProvider).
func RegisterAgentConnectionAccountCreation(server *Server) {
	if server == nil {
		return
	}
	observer.RegisterConnectionEventCallback(func(ctx context.Context, info observer.AgentConnectionInfo) {
		handleAgentConnectionCreateAccount(ctx, server, info)
	})
}

func handleAgentConnectionCreateAccount(ctx context.Context, s *Server, info observer.AgentConnectionInfo) {
	if s.storageProvider == nil {
		return
	}
	secCtx, _ := s.secCtx.(*pkgctx.SecurityContext)
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	username := sanitizeUsername(info.ClientID)
	if username == emptyValue {
		username = "agent"
	}
	accountID := "account:" + username

	builder := bldr_instance_v1.NewAccountInstanceBuilder(objects.DefaultSchemaVersion)
	displayName := info.ClientName
	if displayName == emptyValue {
		displayName = info.ClientID
	}
	builder.ID(accountID).
		Status(accountEnum.StatusActive).
		Username(username).
		DisplayName(displayName).
		Roles([]string{defaultAgentRole})
	instanceMap, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))
		logging.Fluent(logger).Warn("Agent connection: failed to build account instance").
			String("client_id", info.ClientID).
			WithError(err).
			Log()
		return
	}

	err = s.storageProvider.Create(ctx, secCtx, instanceMap)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return
		}
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))
		logging.Fluent(logger).Warn("Agent connection: failed to create account").
			String("account_id", accountID).
			String("client_id", info.ClientID).
			WithError(err).
			Log()
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileMCP))
	logging.Fluent(logger).Info("Agent connection: created account for new agent").
		String("account_id", accountID).
		String("client_id", info.ClientID).
		Log()

	// BLI-860: Notify the new agent so they can re-initialize with the account and get proper permissions.
	notificationMsg := accountCreatedNotificationMessage(accountID)
	_ = s.SendMessageToClientByID(info.ClientID, notificationMsg, "account_created", "high")
}

func sanitizeUsername(clientID string) string {
	s := nonUsernameChars.ReplaceAllString(clientID, "_")
	s = strings.Trim(s, "_")
	for len(s) > 1 {
		if s != strings.TrimPrefix(s, "__") {
			s = strings.ReplaceAll(s, "__", "_")
			continue
		}
		break
	}
	// Treat only dashes/underscores as empty (e.g. "---" or "___")
	s = strings.Trim(s, "-")
	if len(s) > maxUsernameLen {
		s = s[:maxUsernameLen]
	}
	return s
}
