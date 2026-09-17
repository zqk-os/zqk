package monitor

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

// OmniTicker renders agent status to the terminal using ANSI formatting.
type OmniTicker struct{}

func NewOmniTicker() *OmniTicker {
	return &OmniTicker{}
}

func (t *OmniTicker) Render(activeAgents []string) {
	logger := logging.GetLogger()
	logger.LogInfo("\033[H\033[2J") // Clear screen (simplified)
	logger.LogInfo("=== Mission Control ===")
	logger.LogInfo(fmt.Sprintf("Time: %s", time.Now().Format(time.RFC3339)))
	logger.LogInfo("Active Personas:")
	for _, a := range activeAgents {
		logger.LogInfo(fmt.Sprintf("  • %s", a))
	}
}
