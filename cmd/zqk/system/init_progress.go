package system

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// initProgress provides in-process interactive stage updates and periodic heartbeats
// during project initialization so users know long-running steps are actively progressing.
// It integrates directly with pkgctx.GetValidationProgress to report into the existing
// async progress coordinator without spawning any external processes.
type initProgress struct {
	ctx           context.Context
	out           io.Writer
	interactive   bool
	currentStage  string
	stageStart    time.Time
	mu            sync.Mutex
	stopHeartbeat chan struct{}
	stopped       bool
}

func newInitProgress(ctx context.Context, out io.Writer, interactive bool) *initProgress {
	if ctx == nil {
		ctx = context.Background()
	}
	p := &initProgress{
		ctx:           ctx,
		out:           out,
		interactive:   interactive,
		stopHeartbeat: make(chan struct{}),
	}
	if interactive {
		goroutinelabels.NewGoroutine("init_progress_heartbeat", "emit stage heartbeat during project initialization").StartSimple(p.runHeartbeat)
	}
	return p
}

func (p *initProgress) Header(projectName string, mode InitMode) {
	if p == nil || !p.interactive {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.out, "🚀 Initializing %s Kernel — Project: %s (Mode: %s)\n\n", brand.ProductName(), projectName, mode)
}

func (p *initProgress) Step(step, total int, stageName string) {
	if p == nil {
		return
	}
	if p.ctx != nil {
		if fn := pkgctx.GetValidationProgress(p.ctx); fn != nil {
			fn(stageName, fmt.Sprintf("[%d/%d] %s", step, total, stageName))
		}
	}
	if !p.interactive {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentStage = stageName
	p.stageStart = time.Now()
	fmt.Fprintf(p.out, "  [%d/%d] %s\n", step, total, stageName)
}

func (p *initProgress) SubStep(message string) {
	if p == nil {
		return
	}
	if p.ctx != nil {
		if fn := pkgctx.GetValidationProgress(p.ctx); fn != nil {
			fn(p.currentStage, message)
		}
	}
	if !p.interactive {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.out, "      ↳ %s\n", message)
}

func (p *initProgress) runHeartbeat() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.stopHeartbeat:
			return
		case <-ticker.C:
			p.mu.Lock()
			if p.currentStage != "" && time.Since(p.stageStart) >= 3*time.Second {
				elapsed := int(time.Since(p.stageStart).Seconds())
				fmt.Fprintf(p.out, "      ⏳ Working on %s (%ds elapsed)...\n", strings.ToLower(p.currentStage), elapsed)
			}
			p.mu.Unlock()
		}
	}
}

func (p *initProgress) Done() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		close(p.stopHeartbeat)
	}
	p.mu.Unlock()
}

func (p *initProgress) Summary(projectRoot string) {
	if p == nil || !p.interactive {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.out, "\n✅ %s Kernel initialized successfully in %s\n\n", brand.ProductName(), projectRoot)
	fmt.Fprintln(p.out, "Next steps:")
	fmt.Fprintf(p.out, "  1. Discover mission and next tasks:\n     $ %s workflow whats-next\n\n", brand.ExecutableName())
	fmt.Fprintf(p.out, "  2. Onboard and pair with your IDE / AI agent mesh:\n     $ %s system agent-onboard\n\n", brand.ExecutableName())
	fmt.Fprintf(p.out, "  3. Verify kernel compliance and system health:\n     $ %s system check\n\n", brand.ExecutableName())
	fmt.Fprintln(p.out, "Docs & Architecture: docs/INDEX.md")
}
