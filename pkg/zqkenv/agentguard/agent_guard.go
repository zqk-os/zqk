package agentguard

import (
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func init() {
	zqkenv.EnforceForegroundGoTestGuard()
}
