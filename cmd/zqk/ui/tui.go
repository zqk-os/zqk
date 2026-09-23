package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"golang.org/x/term"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RunTUI starts the interactive full-screen mission control session.
func RunTUI(ctx context.Context, projectRoot string, initialTab string, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext) error {
	stdinFd := int(os.Stdin.Fd())
	stdoutFd := int(os.Stdout.Fd())

	// If stdout or stdin is not a terminal, render a single-shot view
	if !term.IsTerminal(stdinFd) || !term.IsTerminal(stdoutFd) {
		m := NewUIModel(projectRoot, initialTab)
		m.Width, m.Height = 100, 30
		m.RefreshMutations()
		m.RefreshObjects()
		if sp != nil && sec != nil {
			m.RefreshSwarm(ctx, sp, sec)
			m.RefreshScheduler(ctx, sp, sec)
		}
		fmt.Print(Render(m))
		return nil
	}

	// Put terminal into raw mode
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(stdinFd, oldState)
	}()

	// Switch to alternate screen buffer, clear screen, and hide cursor
	_, _ = os.Stdout.WriteString("\033[?1049h\033[2J\033[H\033[?25l")
	defer func() {
		// Show cursor and restore normal screen buffer
		_, _ = os.Stdout.WriteString("\033[?25h\033[?1049l\r\n")
	}()

	m := NewUIModel(projectRoot, initialTab)
	w, h, err := term.GetSize(stdoutFd)
	if err == nil {
		m.Width, m.Height = w, h
	} else {
		m.Width, m.Height = 80, 24
	}

	// Initial data fetch
	m.RefreshMutations()
	m.RefreshObjects()
	if sp != nil && sec != nil {
		m.RefreshSwarm(ctx, sp, sec)
		m.RefreshScheduler(ctx, sp, sec)
	}

	// Render initial frame
	writeScreen(Render(m))

	// Setup reactive filesystem watcher
	watcher, _ := fsnotify.NewWatcher()
	if watcher != nil {
		defer watcher.Close()
		for _, kind := range []string{"change_journal_entry", "audit_event", "agent_instruction", "process_lifecycle"} {
			sDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, kind)
			_ = fileutil.MkdirAll(sDir, paths.DirPerm755)
			_ = watcher.Add(sDir)
		}
	}

	// Key event channel
	keyCh := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 16)
		for {
			n, rErr := os.Stdin.Read(buf)
			if rErr != nil {
				return
			}
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				keyCh <- cp
			}
		}
	}()

	// OS signals
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	refreshTicker := time.NewTicker(1 * time.Second)
	defer refreshTicker.Stop()

	var fsEvents <-chan fsnotify.Event
	if watcher != nil {
		fsEvents = watcher.Events
	}

	renderScreen := func() {
		curW, curH, sErr := term.GetSize(stdoutFd)
		if sErr == nil && (curW != m.Width || curH != m.Height) {
			m.Width, m.Height = curW, curH
			// Clear on size change
			_, _ = os.Stdout.WriteString("\033[2J")
		}
		writeScreen(Render(m))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sigCh:
			return nil
		case rawKeys := <-keyCh:
			if shouldExit := handleInput(m, rawKeys); shouldExit {
				return nil
			}
			renderScreen()
		case <-refreshTicker.C:
			m.RefreshMutations()
			if sp != nil && sec != nil {
				m.RefreshSwarm(ctx, sp, sec)
				m.RefreshScheduler(ctx, sp, sec)
			}
			renderScreen()
		case ev, ok := <-fsEvents:
			if ok && (ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create)) {
				m.RefreshMutations()
				renderScreen()
			}
		}
	}
}

// writeScreen handles rendering in terminal raw mode.
// In raw mode, lone \n causes the cursor to step down without returning to column 0
// (the "staircase effect" cascading diagonally down and to the right).
// writeScreen replaces every \n with \r\n and clears the line remainder (\033[K).
func writeScreen(s string) {
	lines := strings.Split(s, "\n")
	var buf strings.Builder
	buf.WriteString("\033[H")
	for i, line := range lines {
		buf.WriteString(line)
		buf.WriteString("\033[K")
		if i < len(lines)-1 {
			buf.WriteString("\r\n")
		}
	}
	_, _ = os.Stdout.WriteString(buf.String())
}

func handleInput(m *UIModel, key []byte) bool {
	if len(key) == 0 {
		return false
	}

	// Check exit keys
	if key[0] == 'q' || key[0] == 'Q' || key[0] == 3 { // Ctrl+C = 3
		return true
	}

	// Single key presses
	if len(key) == 1 {
		switch key[0] {
		case 27: // Esc
			return true
		case '\t': // Tab: cycle tabs
			m.ActiveTab = (m.ActiveTab + 1) % TotalTabs
			m.ScrollOffset = 0
			m.AutoScroll = true
		case '1':
			m.ActiveTab = TabSeismograph
			m.ScrollOffset = 0
			m.AutoScroll = true
		case '2':
			m.ActiveTab = TabSwarm
		case '3':
			m.ActiveTab = TabObjects
		case '4':
			m.ActiveTab = TabScheduler
		case ' ': // Space: toggle auto-scroll
			m.AutoScroll = !m.AutoScroll
			if m.AutoScroll {
				m.ScrollOffset = 0
			}
		case 'r', 'R': // Force refresh
			m.RefreshMutations()
			m.RefreshObjects()
		case 'k', 'K': // Scroll up
			m.AutoScroll = false
			m.ScrollOffset++
		case 'j', 'J': // Scroll down
			if m.ScrollOffset > 0 {
				m.ScrollOffset--
			}
			if m.ScrollOffset == 0 {
				m.AutoScroll = true
			}
		}
		return false
	}

	// ANSI Escape sequences
	if len(key) >= 3 && key[0] == 27 && key[1] == '[' {
		switch key[2] {
		case 'A': // Arrow Up
			m.AutoScroll = false
			m.ScrollOffset++
		case 'B': // Arrow Down
			if m.ScrollOffset > 0 {
				m.ScrollOffset--
			}
			if m.ScrollOffset == 0 {
				m.AutoScroll = true
			}
		case '5': // Page Up
			m.AutoScroll = false
			m.ScrollOffset += 10
		case '6': // Page Down
			m.ScrollOffset -= 10
			if m.ScrollOffset <= 0 {
				m.ScrollOffset = 0
				m.AutoScroll = true
			}
		case 'Z': // Shift-Tab
			m.ActiveTab = (m.ActiveTab - 1 + TotalTabs) % TotalTabs
			m.ScrollOffset = 0
			m.AutoScroll = true
		}
	}

	return false
}
