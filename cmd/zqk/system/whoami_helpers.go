package system

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
)

// WhoamiAccountInfo holds account information
type WhoamiAccountInfo struct {
	AccountID      string
	Roles          []string
	Permissions    []string
	PersonaID      string
	Lane           string
	AccountDetails map[string]any
}

// getProjectRootForWhoami gets the project root for whoami command
func getProjectRootForWhoami(ctx *cli.Context) (string, error) {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return "", errfmt.Errorf("not a ZQK project (no project root found)")
	}
	return projectRoot, nil
}

// parseMCPRoles parses roles from MCP environment variable
func parseMCPRoles(rolesStr string) []string {
	if rolesStr == emptyValue {
		return nil
	}
	roles := strings.Split(rolesStr, ",")
	for i, role := range roles {
		roles[i] = strings.TrimSpace(role)
	}
	return roles
}

// parseMCPPermissions parses permissions from MCP environment variable
func parseMCPPermissions(permsStr string) []string {
	if permsStr == emptyValue {
		return nil
	}
	permissions := strings.Split(permsStr, ",")
	for i, perm := range permissions {
		permissions[i] = strings.TrimSpace(perm)
	}
	return permissions
}

// loadAccountDetailsFromStorage loads account details from storage
func loadAccountDetailsFromStorage(cmd *cobra.Command, projectRoot string, accountID string) map[string]any {
	factory, err := storage.NewStorageFactory(cmd.Context(), projectRoot)
	if err != nil {
		return nil
	}
	storageProvider := factory.GetStorage()
	if storageProvider != nil {
		defer func() { _ = storageProvider.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	accountObj, err := storageProvider.Read(cmd.Context(), secCtx, accountID)
	if err != nil || accountObj == nil {
		return nil
	}

	return accountObj
}

// loadWhoamiFromSecurityContext uses the AuthMiddleware-resolved seat.
func loadWhoamiFromSecurityContext(cmd *cobra.Command, projectRoot string) *WhoamiAccountInfo {
	sec := pkgctx.GetSecurityContext(cmd.Context())
	if sec == nil || strings.TrimSpace(sec.AccountID) == emptyValue {
		return loadMCPAccountInfo(cmd, projectRoot)
	}
	info := &WhoamiAccountInfo{
		AccountID:   sec.AccountID,
		Roles:       append([]string(nil), sec.Roles...),
		Permissions: append([]string(nil), sec.Permissions...),
		PersonaID:   sec.PersonaID,
		Lane:        authcred.ClassifyLane(sec),
	}
	info.AccountDetails = loadAccountDetailsFromStorage(cmd, projectRoot, sec.AccountID)
	authcred.WriteIdentityStatus(projectRoot, authcred.SnapshotFromSecurityContext(sec, info.AccountDetails))
	return info
}

// loadMCPAccountInfo loads account information from MCP environment variables
func loadMCPAccountInfo(cmd *cobra.Command, projectRoot string) *WhoamiAccountInfo {
	mcpAccountID := zqkenv.MCPAccountID().Get()
	if mcpAccountID == emptyValue {
		return nil
	}

	info := &WhoamiAccountInfo{
		AccountID: mcpAccountID,
	}

	// Get roles from environment
	if mcpRoles := zqkenv.MCPRoles().Get(); mcpRoles != emptyValue {
		info.Roles = parseMCPRoles(mcpRoles)
	}

	// Get permissions from environment
	if mcpPerms := zqkenv.MCPPermissions().Get(); mcpPerms != emptyValue {
		info.Permissions = parseMCPPermissions(mcpPerms)
	}

	// Try to load account details from storage
	info.AccountDetails = loadAccountDetailsFromStorage(cmd, projectRoot, mcpAccountID)
	if info.AccountDetails != nil {
		if p, ok := info.AccountDetails[objects.FieldKeyPersonaRef].(string); ok {
			info.PersonaID = p
		}
	}

	return info
}

// buildWhoamiOutputData builds the output data structure
func buildWhoamiOutputData(accountInfo *WhoamiAccountInfo) map[string]any {
	outputData := map[string]any{
		objects.FieldKeyAccountID:   accountInfo.AccountID,
		objects.FieldKeyRoles:       accountInfo.Roles,
		objects.FieldKeyPermissions: accountInfo.Permissions,
		objects.FieldKeyPersonaRef:  accountInfo.PersonaID,
		"lane":                      accountInfo.Lane,
		"grant_source":              "roles",
	}

	// Add account details if available
	addAccountDetailsToOutput(outputData, accountInfo.AccountDetails)

	// Add context information
	addContextToOutput(outputData)

	return outputData
}

// addAccountDetailsToOutput adds account details to output data
func addAccountDetailsToOutput(outputData map[string]any, accountDetails map[string]any) {
	if accountDetails == nil {
		return
	}

	if displayName, ok := accountDetails[objects.FieldKeyDisplayName].(string); ok {
		outputData[objects.FieldKeyDisplayName] = displayName
	}
	if email, ok := accountDetails[objects.FieldKeyEmail].(string); ok && email != emptyValue {
		outputData[objects.FieldKeyEmail] = email
	}
	if username, ok := accountDetails[objects.FieldKeyUsername].(string); ok {
		outputData[objects.FieldKeyUsername] = username
	}
	if title, ok := accountDetails[objects.FieldKeyTitle].(string); ok {
		outputData[objects.FieldKeyTitle] = title
	}
}

// addContextToOutput adds context information to output data
func addContextToOutput(outputData map[string]any) {
	source := "cli"
	if zqkenv.MCPAccountID().Get() != emptyValue {
		source = "mcp"
	}

	outputData[objects.FieldKeyContext] = map[string]any{
		objects.FieldKeySource: source,
	}
}

// formatWhoamiTableAccount formats account ID for table output
func formatWhoamiTableAccount(data map[string]any) string {
	accountID, ok := data[objects.FieldKeyAccountID].(string)
	if !ok {
		return ""
	}
	return fmt.Sprintf("Account ID: %s\n", accountID)
}

func formatWhoamiTableLane(data map[string]any) string {
	lane, ok := data["lane"].(string)
	if !ok || lane == emptyValue {
		return ""
	}
	return fmt.Sprintf("Lane: %s (grants from roles; ACC is the audit identity)\n", lane)
}

func formatWhoamiTablePersona(data map[string]any) string {
	persona, ok := data[objects.FieldKeyPersonaRef].(string)
	if !ok || persona == emptyValue {
		return ""
	}
	return fmt.Sprintf("Persona: %s\n", persona)
}

// formatWhoamiTableName formats display name or title for table output
func formatWhoamiTableName(data map[string]any) string {
	if displayName, ok := data[objects.FieldKeyDisplayName].(string); ok && displayName != emptyValue {
		return fmt.Sprintf("Display Name: %s\n", displayName)
	}
	if title, ok := data[objects.FieldKeyTitle].(string); ok && title != emptyValue {
		return fmt.Sprintf("Title: %s\n", title)
	}
	return ""
}

// formatWhoamiTableUserInfo formats username and email for table output
func formatWhoamiTableUserInfo(data map[string]any) string {
	var result string
	if username, ok := data[objects.FieldKeyUsername].(string); ok && username != emptyValue {
		result += fmt.Sprintf("Username: %s\n", username)
	}
	if email, ok := data[objects.FieldKeyEmail].(string); ok && email != emptyValue {
		result += fmt.Sprintf("Email: %s\n", email)
	}
	return result
}

// formatWhoamiTableRoles formats roles for table output
func formatWhoamiTableRoles(data map[string]any) string {
	roles, ok := data[objects.FieldKeyRoles].([]string)
	if !ok || len(roles) == 0 {
		return ""
	}
	return fmt.Sprintf("Roles: %s\n", strings.Join(roles, ", "))
}

// formatWhoamiTablePermissions formats permissions for table output
func formatWhoamiTablePermissions(data map[string]any) string {
	permissions, ok := data[objects.FieldKeyPermissions].([]string)
	if !ok || len(permissions) == 0 {
		return ""
	}
	return fmt.Sprintf("Permissions: %s\n", strings.Join(permissions, ", "))
}

// formatWhoamiTableContext formats context for table output
func formatWhoamiTableContext(data map[string]any) string {
	contextInfo, ok := data[objects.FieldKeyContext].(map[string]any)
	if !ok {
		return ""
	}

	source, ok := contextInfo[objects.FieldKeySource].(string)
	if !ok {
		return ""
	}

	sourceLabel := "CLI"
	if source == systemProfileMCP {
		sourceLabel = "MCP Server"
	}

	return fmt.Sprintf("\nContext: %s\n", sourceLabel)
}
