// Package macos adapts host-level message delivery on Darwin.
//
// ClipboardPasteAdapter pastes via pbcopy and System Events AppleScript
// keystrokes into a named target app (default Cursor / ZQK_IDE_PASTE_APP /
// renderDetails["target_app"]). It refuses to emit keystrokes when a terminal
// (or any non-target process) is frontmost — blind frontmost paste mistargets
// shells. Host membrane only; fails closed on non-darwin GOOS.
package macos
