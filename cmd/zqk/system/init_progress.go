package system

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func (p *initProgress) withInteractiveLock(fn func()) {
	if p == nil || !p.interactive {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	fn()
}

func (p *initProgress) reportValidationProgress(stage, message string) {
	if p != nil && p.ctx != nil {
		if fn := pkgctx.GetValidationProgress(p.ctx); fn != nil {
			fn(stage, message)
		}
	}
}

func (p *initProgress) printInteractive(format string, a ...any) {
	p.withInteractiveLock(func() {
		fmt.Fprintf(p.out, format, a...)
	})
}

func (p *initProgress) Header(projectName string, mode InitMode) {
	p.printInteractive("🚀 Initializing %s Kernel — Project: %s (Mode: %s)\n\n", brand.ProductName(), projectName, mode)
}

func (p *initProgress) Step(step, total int, stageName string) {
	p.reportValidationProgress(stageName, fmt.Sprintf("[%d/%d] %s", step, total, stageName))
	p.withInteractiveLock(func() {
		p.currentStage = stageName
		p.stageStart = time.Now()
		fmt.Fprintf(p.out, "  [%d/%d] %s\n", step, total, stageName)
	})
}

func (p *initProgress) SubStep(message string) {
	stage := ""
	if p != nil {
		stage = p.currentStage
	}
	p.reportValidationProgress(stage, message)
	p.printInteractive("      ↳ %s\n", message)
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
	p.withInteractiveLock(func() {
		fmt.Fprintf(p.out, "\n✅ %s Kernel initialized successfully in %s\n\n", brand.ProductName(), projectRoot)
		fmt.Fprintln(p.out, "🚀 Next Step: Hand off to your AI Assistant")
		fmt.Fprintln(p.out, "   Copy and paste this prompt into your AI coding assistant (Cursor, Claude, Windsurf, Cline, Gemini):")
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "   \"You are the primary %s coordinating agent for this project (%s).\n", brand.ProductName(), projectRoot)
		fmt.Fprintf(p.out, "    Equip the %s expert skill and TPM persona, and help me configure\n", brand.ProductName())
		fmt.Fprintf(p.out, "    the %s system for our greenfield project.\"\n\n", brand.ProductName())
		fmt.Fprintln(p.out, "--------------------------------------------------------------------------------")
		fmt.Fprintln(p.out, "Optional Controls:")
		fmt.Fprintf(p.out, "  • Launch Visual Web Studio:  $ %s ui -w  (http://127.0.0.1:8080)\n", brand.ExecutableName())
		fmt.Fprintf(p.out, "  • Execute Autonomously:       $ %s do\n", brand.ExecutableName())

		docsIndexPath := filepath.Join(projectRoot, "docs", "INDEX.md")
		gettingStartedPath := filepath.Join(projectRoot, "ZQK_GETTING_STARTED.md")
		if _, err := os.Stat(docsIndexPath); err == nil {
			fmt.Fprintln(p.out, "  • Docs & Architecture:       docs/INDEX.md")
		} else if _, err := os.Stat(gettingStartedPath); err == nil {
			fmt.Fprintln(p.out, "  • Getting Started Guide:     ZQK_GETTING_STARTED.md")
		} else {
			fmt.Fprintln(p.out, "  • Online Documentation:      https://docs.zqk.dev")
		}
	})
}
