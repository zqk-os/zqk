package agentguard

import (
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func init() {
	zqkenv.EnforceForegroundGoTestGuard()
}
