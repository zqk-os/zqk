package ui

// Canonical ANSI Escape Sequences and Terminal Control Codes.
// Centralizing these constants ensures that low-level terminal operations
// are self-documenting and prevent magical byte strings across the TUI codebase.
const (
	// Alternate Screen Buffer Controls
	// \033[?1049h: switches the terminal emulator to a private alternate screen buffer,
	// preserving the user's existing terminal scrollback and command history upon exit.
	AnsiAltBufferEnter = "\033[?1049h"

	// \033[?1049l: restores the standard terminal buffer and scrollback history.
	AnsiAltBufferExit = "\033[?1049l"

	// Screen & Cursor Position Controls
	// \033[2J: erases the entire active display buffer.
	AnsiClearScreen = "\033[2J"

	// \033[H: moves the cursor to the home position (row 1, column 1).
	AnsiHomeCursor = "\033[H"

	// \033[?25l: hides the terminal cursor to prevent cursor flicker during redraws.
	AnsiHideCursor = "\033[?25l"

	// \033[?25h: restores cursor visibility.
	AnsiShowCursor = "\033[?25h"

	// AnsiClearToEOL erases from the current cursor position to the end of the line.
	// Essential in raw mode to prevent artifacts when overwriting previous frames.
	AnsiClearToEOL = "\033[K"

	// AnsiClearToBottom erases from the current cursor position to the end of the screen/display.
	// Essential when switching from a taller view to a shorter view to prevent stale ghost lines.
	AnsiClearToBottom = "\033[J"
	AnsiClearToScreenBottom = AnsiClearToBottom

	// CRLF is the explicit Carriage Return + Line Feed required in raw terminal mode.
	// In raw mode, standard '\n' only performs line-feed without resetting column position,
	// causing the "staircase effect" cascading diagonally down and right.
	CRLF = "\r\n"
)

// Keyboard and Terminal Input Constants.
const (
	// KeyCtrlC is the ASCII ETX (End of Text, code 3) sent on Ctrl+C.
	KeyCtrlC byte = 3

	// KeyTab is the ASCII horizontal tab (code 9).
	KeyTab byte = '\t'

	// KeyEsc is the ASCII escape code (code 27 / 0x1b).
	KeyEsc byte = 27

	// KeySpace is the ASCII space character.
	KeySpace byte = ' '

	// KeyEnter is the ASCII carriage return character.
	KeyEnter byte = '\r'

	// KeyBackspace is the standard ASCII DEL character (127).
	KeyBackspace byte = 127
)

// ANSI Multi-Byte Control Sequence Introducer (CSI) codes.
const (
	// CSIPrefixEsc is the leading byte of a CSI sequence (\033).
	CSIPrefixEsc byte = 27

	// CSIPrefixBracket is the second byte of a CSI sequence ('[').
	CSIPrefixBracket byte = '['

	// CSI Final Bytes for cursor movement and function keys:
	SeqCodeArrowUp    byte = 'A' // Up cursor (\033[A)
	SeqCodeArrowDown  byte = 'B' // Down cursor (\033[B)
	SeqCodeArrowRight byte = 'C' // Right cursor (\033[C)
	SeqCodeArrowLeft  byte = 'D' // Left cursor (\033[D)
	SeqCodeShiftTab   byte = 'Z' // Shift+Tab (Backtab: \033[Z)
	SeqCodePageUp     byte = '5' // Page Up (\033[5~)
	SeqCodePageDown   byte = '6' // Page Down (\033[6~)
)
