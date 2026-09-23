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

	// Non-interactive fallback: render a single-shot frame
	if !term.IsTerminal(stdinFd) || !term.IsTerminal(stdoutFd) {
		m := NewUIModel(projectRoot, initialTab)
		m.Width, m.Height = 100, 30
		m.RefreshMutations()
		m.RefreshAuditEvents()
		m.RefreshObjects()
		m.RefreshQA(ctx, sp, sec)
		if sp != nil && sec != nil {
			m.RefreshSwarm(ctx, sp, sec)
			m.RefreshPM(ctx, sp, sec)
			m.RefreshMetrics(ctx, sp, sec)
			m.RefreshScheduler(ctx, sp, sec)
		}
		fmt.Print(Render(m))
		return nil
	}

	// Put terminal into raw mode to capture individual keypresses
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return err
	}
	defer func() {
		_ = term.Restore(stdinFd, oldState)
	}()

	// Switch to alternate screen buffer, clear screen, and hide cursor
	_, _ = os.Stdout.WriteString(AnsiAltBufferEnter + AnsiClearScreen + AnsiHomeCursor + AnsiHideCursor)
	defer func() {
		// Restore cursor visibility, exit alternate buffer, and return cursor
		_, _ = os.Stdout.WriteString(AnsiShowCursor + AnsiAltBufferExit + CRLF)
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
	m.RefreshAuditEvents()
	m.RefreshObjects()
	m.RefreshQA(ctx, sp, sec)
	if sp != nil && sec != nil {
		m.RefreshSwarm(ctx, sp, sec)
		m.RefreshPM(ctx, sp, sec)
		m.RefreshMetrics(ctx, sp, sec)
		m.RefreshScheduler(ctx, sp, sec)
	}

	// Render initial frame
	writeScreen(Render(m))

	// Setup reactive filesystem watcher across tracked stream directories
	watcher, _ := fsnotify.NewWatcher()
	if watcher != nil {
		defer watcher.Close()
		for _, kind := range []string{"change_journal_entry", "audit_event", "agent_instruction", "process_lifecycle"} {
			sDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, kind)
			_ = fileutil.MkdirAll(sDir, paths.DirPerm755)
			_ = watcher.Add(sDir)
		}
	}

	// Non-blocking keyboard input loop
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

	// OS signals for clean interruption
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
			_, _ = os.Stdout.WriteString(AnsiClearScreen)
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
			m.RefreshAuditEvents()
			m.RefreshQA(ctx, sp, sec)
			m.RefreshHealth()
			if sp != nil && sec != nil {
				m.RefreshSwarm(ctx, sp, sec)
				m.RefreshPM(ctx, sp, sec)
				m.RefreshMetrics(ctx, sp, sec)
				m.RefreshScheduler(ctx, sp, sec)
			}
			renderScreen()
		case ev, ok := <-fsEvents:
			if ok && (ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create)) {
				m.RefreshMutations()
				m.RefreshAuditEvents()
				renderScreen()
			}
		}
	}
}

// writeScreen handles rendering in terminal raw mode.
// In raw mode, standard '\n' only performs line-feed without resetting column position
// (the "staircase effect" cascading diagonally down and to the right).
// writeScreen clears each line remainder, appends an explicit CRLF, and finally clears
// from cursor to the bottom of the screen (AnsiClearToBottom) to erase any old lines.
func writeScreen(s string) {
	lines := strings.Split(s, "\n")
	var buf strings.Builder
	buf.WriteString(AnsiHomeCursor)
	for i, line := range lines {
		buf.WriteString(line)
		buf.WriteString(AnsiClearToEOL)
		if i < len(lines)-1 {
			buf.WriteString(CRLF)
		}
	}
	buf.WriteString(AnsiClearToBottom)
	_, _ = os.Stdout.WriteString(buf.String())
}

func handleInput(m *UIModel, key []byte) bool {
	if len(key) == 0 {
		return false
	}

	// Modal Overlay Dismissal: if modal is open, Esc, Backspace, or 'q' closes the modal without exiting TUI
	if m.DetailModal != nil {
		if (len(key) == 1 && (key[0] == KeyEsc || key[0] == KeyBackspace || key[0] == 'q' || key[0] == 'Q')) ||
			(len(key) >= 3 && key[0] == CSIPrefixEsc && key[1] == CSIPrefixBracket) {
			m.DetailModal = nil
			return false
		}
		if key[0] == KeyCtrlC {
			return true
		}
		return false
	}

	// Exit commands when modal is not open
	if key[0] == 'q' || key[0] == 'Q' || key[0] == KeyCtrlC || (len(key) == 1 && key[0] == KeyEsc) {
		return true
	}

	// Single keypress handling
	if len(key) == 1 {
		switch key[0] {
		case KeyTab: // Cycle tabs forward
			m.ActiveTab = (m.ActiveTab + 1) % TotalTabs
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '1':
			m.ActiveTab = TabState
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '2':
			m.ActiveTab = TabAudit
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '3':
			m.ActiveTab = TabSwarm
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '4':
			m.ActiveTab = TabPM
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '5':
			m.ActiveTab = TabMetrics
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '6':
			m.ActiveTab = TabScheduler
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '7':
			m.ActiveTab = TabQA
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case '8':
			m.ActiveTab = TabHealth
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		case KeyEnter, '\n': // Step into record details (drill-down modal)
			m.OpenSelectedItemDetail()
		case KeySpace: // Toggle auto-scroll
			m.AutoScroll = !m.AutoScroll
			if m.AutoScroll {
				m.ScrollOffset = 0
			}
		case 'r', 'R': // Force refresh
			m.RefreshMutations()
			m.RefreshAuditEvents()
			m.RefreshObjects()
			m.RefreshQA(context.Background(), nil, nil)
			m.RefreshHealth()
		case 'c', 'C', 'a', 'A', 'w', 'W', 'd', 'D', 'p', 'P', 'm', 'M', 's', 'S', 'b', 'B': // Action Center triggers (Tab 8)
			if m.ActiveTab == TabHealth {
				m.TriggerActionCenter(string(key[0]))
			}
		case 'k', 'K': // Cursor up / Scroll up
			m.AutoScroll = false
			m.ScrollOffset++
			if m.SelectedIndex > 0 {
				m.SelectedIndex--
			}
		case 'j', 'J': // Cursor down / Scroll down
			if m.ScrollOffset > 0 {
				m.ScrollOffset--
			}
			if m.ScrollOffset == 0 {
				m.AutoScroll = true
			}
			maxRows := m.GetCurrentRowCount()
			if maxRows > 0 && m.SelectedIndex < maxRows-1 {
				m.SelectedIndex++
			}
		}
		return false
	}

	// ANSI multi-byte escape sequences
	if len(key) >= 3 && key[0] == CSIPrefixEsc && key[1] == CSIPrefixBracket {
		switch key[2] {
		case SeqCodeArrowUp:
			m.AutoScroll = false
			m.ScrollOffset++
			if m.SelectedIndex > 0 {
				m.SelectedIndex--
			}
		case SeqCodeArrowDown:
			if m.ScrollOffset > 0 {
				m.ScrollOffset--
			}
			if m.ScrollOffset == 0 {
				m.AutoScroll = true
			}
			maxRows := m.GetCurrentRowCount()
			if maxRows > 0 && m.SelectedIndex < maxRows-1 {
				m.SelectedIndex++
			}
		case SeqCodePageUp:
			m.AutoScroll = false
			m.ScrollOffset += 10
			m.SelectedIndex -= 10
			if m.SelectedIndex < 0 {
				m.SelectedIndex = 0
			}
		case SeqCodePageDown:
			m.ScrollOffset -= 10
			if m.ScrollOffset <= 0 {
				m.ScrollOffset = 0
				m.AutoScroll = true
			}
			maxRows := m.GetCurrentRowCount()
			m.SelectedIndex += 10
			if maxRows > 0 && m.SelectedIndex >= maxRows {
				m.SelectedIndex = maxRows - 1
			}
		case SeqCodeShiftTab: // Shift+Tab: cycle tabs backward
			m.ActiveTab = (m.ActiveTab - 1 + TotalTabs) % TotalTabs
			m.ScrollOffset = 0
			m.SelectedIndex = 0
			m.AutoScroll = true
		}
	}

	return false
}
