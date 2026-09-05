package handslapper

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
)

// Electrocute generates a standardized punitive error message for any agent constraint violations.
// It tailors the message based on the active security context's persona.
func Electrocute(ctx context.Context, violationType string, reason string) error {
	persona := "UNKNOWN_ENTITY"
	if ctx != nil {
		if secCtx := pkgctx.GetSecurityContext(ctx); secCtx != nil {
			persona = secCtx.AccountID
			if len(secCtx.Roles) > 0 {
				persona = fmt.Sprintf("%s (Role: %s)", persona, secCtx.Roles[0])
			}
		}
	}

	msg := fmt.Sprintf("⚡️ BZZZT! AGENT ELECTROCUTED: %s\nTarget Persona: [%s]\nUser Mandate Violation: %s\n", violationType, persona, reason)
	return errfmt.Errorf("%s", msg)
}
