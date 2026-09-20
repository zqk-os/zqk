// Package macos adapts host-level message delivery on Darwin.
//
// ClipboardPasteAdapter pastes via pbcopy and System Events AppleScript
// keystrokes. It is a macOS vendor/host membrane, not kernel-native delivery,
// and fails closed on non-darwin GOOS.
package macos
