package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// Transport handles reading and writing MCP messages
// It abstracts the underlying message format (raw JSON vs Content-Length framed)
type Transport interface {
	// ReadMessage reads a single message from the reader
	// Returns: (message bytes, format info, error)
	ReadMessage(reader *bufio.Reader) ([]byte, *MessageFormat, error)

	// WriteMessage writes a message to the writer
	// format determines whether to use headers or raw JSON
	WriteMessage(writer *bufio.Writer, data []byte, format *MessageFormat) error
}

// MessageFormat describes the format of a message
type MessageFormat struct {
	// IsRawJSON is true if the message is raw JSON (no headers)
	// If false, the message uses Content-Length headers
	IsRawJSON bool
}

// DefaultTransport is the default implementation that handles both formats
type DefaultTransport struct {
	dec *json.Decoder
}

// NewDefaultTransport creates a new default transport
func NewDefaultTransport() *DefaultTransport {
	return &DefaultTransport{}
}

// ReadMessage reads a message and detects its format
func (t *DefaultTransport) ReadMessage(reader *bufio.Reader) ([]byte, *MessageFormat, error) {
	// Skip leading whitespace
	for {
		b, err := reader.Peek(1)
		if err != nil {
			return nil, nil, err
		}
		if isWhitespace(b[0]) {
			if _, err := reader.ReadByte(); err != nil {
				return nil, nil, err
			}
			continue
		}
		break
	}

	// Peek at first character to determine message format
	first, err := reader.Peek(1)
	if err != nil {
		return nil, nil, err
	}

	// If starts with { or [, it's raw JSON
	if first[0] == '{' || first[0] == '[' {
		if t.dec == nil {
			t.dec = json.NewDecoder(reader)
		}
		var raw json.RawMessage
		if err := t.dec.Decode(&raw); err != nil {
			return nil, nil, errfmt.Newf("failed to decode raw JSON").Wrap(err)
		}
		return raw, &MessageFormat{IsRawJSON: true}, nil
	}

	// Otherwise, it's Content-Length framed
	msg, err := readContentLengthMessage(reader)
	if err != nil {
		return nil, nil, err
	}
	return msg, &MessageFormat{IsRawJSON: false}, nil
}

// WriteMessage writes a message using the specified format
// Handles partial writes by retrying until all data is written
func (t *DefaultTransport) WriteMessage(writer *bufio.Writer, data []byte, format *MessageFormat) error {
	if format.IsRawJSON {
		// Raw JSON: write JSON + newline
		// Handle partial writes
		if err := writeAll(writer, data); err != nil {
			return err
		}
		if err := writeAllString(writer, "\n"); err != nil {
			return err
		}
	} else {
		// Content-Length framed: write headers + JSON
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
		if err := writeAllString(writer, header); err != nil {
			return err
		}
		if err := writeAll(writer, data); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeAll(writer *bufio.Writer, data []byte) error {
	written := 0
	for written < len(data) {
		n, err := writer.Write(data[written:])
		if err != nil {
			return errfmt.Newf("short write: wrote %d of %d bytes", written, len(data)).Wrap(err)
		}
		written += n
	}
	return nil
}

func writeAllString(writer *bufio.Writer, data string) error {
	written := 0
	for written < len(data) {
		n, err := writer.WriteString(data[written:])
		if err != nil {
			return errfmt.Newf("short write: wrote %d of %d bytes", written, len(data)).Wrap(err)
		}
		written += n
	}
	return nil
}

// readContentLengthMessage reads a Content-Length framed message
func readContentLengthMessage(reader *bufio.Reader) ([]byte, error) {
	var contentLength = -1

	// Read headers until blank line
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == emptyValue {
			break
		}

		// Parse header (key: value)
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if key == "content-length" {
			length, err := strconv.Atoi(value)
			if err != nil {
				return nil, errfmt.Newf("invalid Content-Length value").Wrap(err)
			}
			contentLength = length
		}
	}

	if contentLength < 0 {
		return nil, errfmt.Errorf("missing Content-Length header")
	}

	// Read the message body
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, errfmt.Newf("failed to read message body").Wrap(err)
	}

	return body, nil
}

// isWhitespace checks if a byte is whitespace
func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
