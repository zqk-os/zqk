package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// EmitContextHUD loudly broadcasts provenance if an action is executing across context boundaries.
func EmitContextHUD(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any, action string, targetID string) {
	if obj == nil {
		return
	}

	namespaceID, _ := obj[objects.FieldKeyNamespaceID].(string)
	if namespaceID == "" || namespaceID == validation.DefaultNamespaceKernel {
		return // No HUD needed for local context
	}

	actor := "Human"
	if secCtx != nil && secCtx.AccountID != "" {
		actor = secCtx.AccountID
		if strings.HasPrefix(actor, "agent:") || strings.HasPrefix(actor, "service:") {
			actor = fmt.Sprintf("Agent '%s'", actor)
		} else {
			actor = fmt.Sprintf("User '%s'", actor)
		}
	}

	hud := color.New(color.FgHiYellow, color.Bold).Sprintf("[Mesh Context: %s] %s is %s object %s...", namespaceID, actor, action, targetID)

	if ctx != nil {
		writer := logging.GetCommandOutputWriter(ctx)
		if writer != nil {
			_, _ = io.WriteString(writer, hud+"\n")
			return
		}
	}

	// Fallback if no context/logger provided
	_, _ = os.Stderr.WriteString(hud + "\n")
}
