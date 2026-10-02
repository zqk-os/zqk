package system

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/tde"
)

func registerTDEScriptAction(actionName, logPrefix string) {
	tde.RegisterAction(actionName, func(ctx context.Context, env tde.Envelope) error {
		scriptBytes, err := base64.StdEncoding.DecodeString(env.PayloadB64)
		if err != nil {
			return fmt.Errorf("failed to decode %s payload: %w", actionName, err)
		}
		script := string(scriptBytes)
		logger := logging.GetLoggerFromContext(ctx)
		logging.FluentEvent(logger).Info(logPrefix).Script(script).Log()

		cmd := execwrap.CommandContext(ctx, "bash", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
}

func init() {
	for _, action := range []string{"mubert-generate", "mubert-streaming", "ffmpeg"} {
		registerTDEScriptAction(action, fmt.Sprintf("TDE %s Executing", action))
	}
}
