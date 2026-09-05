package ideadapter

import (
	"errors"
	"io"
	"net"
	"testing"
)

func TestHeartbeatDropAfterConsecutiveFails(t *testing.T) {
	t.Parallel()
	if heartbeatDropAfterConsecutiveFails(0) || heartbeatDropAfterConsecutiveFails(1) || heartbeatDropAfterConsecutiveFails(2) {
		t.Fatal("must not drop conn before three consecutive ping failures")
	}
	if !heartbeatDropAfterConsecutiveFails(3) || !heartbeatDropAfterConsecutiveFails(4) {
		t.Fatal("must drop conn on the third consecutive ping failure")
	}
}

func TestIsExpectedDaemonDisconnect(t *testing.T) {
	t.Parallel()
	if !isExpectedDaemonDisconnect(io.EOF) || !isExpectedDaemonDisconnect(net.ErrClosed) {
		t.Fatal("EOF and net.ErrClosed must be expected")
	}
	if !isExpectedDaemonDisconnect(errors.New("read tcp 127.0.0.1:1: use of closed network connection")) {
		t.Fatal("self-close must not Warn")
	}
	if isExpectedDaemonDisconnect(errors.New("tls handshake timeout")) {
		t.Fatal("real transport errors must still Warn")
	}
}
