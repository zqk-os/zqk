package ticker

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/orchestration"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ActiveAgent reports the current status of an agent persona.
type ActiveAgent struct {
	Persona    string
	Workstream string
}

// ActivityTicker tracks and surfaces active agents.
type ActivityTicker struct {
	mu     sync.RWMutex
	active map[string]ActiveAgent
}

func NewActivityTicker() *ActivityTicker {
	return &ActivityTicker{
		active: make(map[string]ActiveAgent),
	}
}

// Tick records agent activity.
func (t *ActivityTicker) Tick(persona, workstream string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active[persona] = ActiveAgent{Persona: persona, Workstream: workstream}
}

// GetStatus returns the current active agent hive status.
func (t *ActivityTicker) GetStatus() []ActiveAgent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	status := make([]ActiveAgent, 0, len(t.active))
	for _, agent := range t.active {
		status = append(status, agent)
	}
	return status
}

// Display surfaces the current status to a persistent log file.
func (t *ActivityTicker) Display(nextUpdate time.Duration) {
	status := t.GetStatus()

	if len(status) == 0 {
		return
	}

	// Open or create the log file in .zqk/logs using resolved project root
	root := paths.ResolveProjectRoot(".")
	logPath := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, orchestration.LogKeyHiveActivity)

	f, err := fileutil.OpenFile(logPath, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		logging.GetLogger().LogError(fmt.Sprintf(orchestration.ErrMsgWriteTickerLog, err), err)
		return
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, orchestration.LogFmtHiveAgentsActive, len(status)); err != nil {
		logging.GetLogger().LogError(fmt.Sprintf(orchestration.ErrMsgWriteTickerLog, err), err)
	}
	for _, agent := range status {
		if _, err := fmt.Fprintf(f, "   - %s: %s\n", agent.Persona, agent.Workstream); err != nil {
			logging.GetLogger().LogError(fmt.Sprintf(orchestration.ErrMsgWriteAgentActivity, err), err)
		}
	}
	if _, err := fmt.Fprintf(f, orchestration.LogFmtNextUpdate, nextUpdate.String()); err != nil {
		logging.GetLogger().LogError(fmt.Sprintf(orchestration.ErrMsgWriteNextUpdate, err), err)
	}
}

// RunHeartbeat periodically displays agent activity.
func (t *ActivityTicker) RunHeartbeat(ctx context.Context, interval time.Duration) {
	// Initial display on start
	t.Display(interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.Display(interval)
		case <-ctx.Done():
			return
		}
	}
}
