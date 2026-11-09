package agent

import (
	"io"
	"os"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// safeDrainInput reads all bytes from in, avoiding blocking on character devices
// or open pipes without EOF deadlines.
func safeDrainInput(in io.Reader) ([]byte, error) {
	if in == nil {
		return nil, nil
	}
	if f, ok := in.(*os.File); ok {
		if stat, err := f.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) != 0 {
			return nil, nil
		}
	}

	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)
	goroutinelabels.NewGoroutine("safe_drain_input_reader", "drain stdin pipe with timeout").
		StartSimple(func() {
			data, err := io.ReadAll(in)
			ch <- readResult{data: data, err: err}
		})

	select {
	case res := <-ch:
		return res.data, res.err
	case <-time.After(200 * time.Millisecond):
		return nil, nil
	}
}
