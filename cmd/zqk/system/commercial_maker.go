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

func init() {
	tde.RegisterAction("mubert-generate", func(ctx context.Context, env tde.Envelope) error {
		scriptBytes, err := base64.StdEncoding.DecodeString(env.PayloadB64)
		if err != nil {
			return fmt.Errorf("failed to decode mubert-generate payload: %w", err)
		}
		script := string(scriptBytes)
		logger := logging.GetLoggerFromContext(ctx)
		logging.FluentEvent(logger).Info("TDE mubert-generate Executing").Script(script).Log()

		cmd := execwrap.CommandContext(ctx, "bash", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})

	tde.RegisterAction("mubert-streaming", func(ctx context.Context, env tde.Envelope) error {
		scriptBytes, err := base64.StdEncoding.DecodeString(env.PayloadB64)
		if err != nil {
			return fmt.Errorf("failed to decode mubert-streaming payload: %w", err)
		}
		script := string(scriptBytes)
		logger := logging.GetLoggerFromContext(ctx)
		logging.FluentEvent(logger).Info("TDE mubert-streaming Executing").Script(script).Log()

		cmd := execwrap.CommandContext(ctx, "bash", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})

	tde.RegisterAction("ffmpeg", func(ctx context.Context, env tde.Envelope) error {
		scriptBytes, err := base64.StdEncoding.DecodeString(env.PayloadB64)
		if err != nil {
			return fmt.Errorf("failed to decode ffmpeg payload: %w", err)
		}
		script := string(scriptBytes)
		logger := logging.GetLoggerFromContext(ctx)
		logging.FluentEvent(logger).Info("TDE ffmpeg Executing").Script(script).Log()

		cmd := execwrap.CommandContext(ctx, "bash", "-c", script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	})
}
