package scheduler

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"syscall"

	"github.com/lanceman/zqk/pkg/execwrap"
)

// Cmd represents an executable command.
type Cmd interface {
	Start() error
	Wait() error
	Run() error
	Output() ([]byte, error)
	SetDir(string)
	SetEnv([]string)
	SetStdin(io.Reader)
	SetStdout(io.Writer)
	SetStderr(io.Writer)
	SetSysProcAttr(*syscall.SysProcAttr)
	GetPid() int
}

// CommandExecutor abstracts OS-level process execution for deterministic testing.
type CommandExecutor interface {
	CommandContext(ctx context.Context, name string, args ...string) Cmd
}

// NativeCmd wraps exec.Cmd.
type NativeCmd struct {
	cmd *exec.Cmd
}

func (c *NativeCmd) Start() error                             { return c.cmd.Start() }
func (c *NativeCmd) Wait() error                              { return c.cmd.Wait() }
func (c *NativeCmd) Run() error                               { return c.cmd.Run() }
func (c *NativeCmd) Output() ([]byte, error)                  { return c.cmd.Output() }
func (c *NativeCmd) SetDir(dir string)                        { c.cmd.Dir = dir }
func (c *NativeCmd) SetEnv(env []string)                      { c.cmd.Env = env }
func (c *NativeCmd) SetStdin(r io.Reader)                     { c.cmd.Stdin = r }
func (c *NativeCmd) SetStdout(w io.Writer)                    { c.cmd.Stdout = w }
func (c *NativeCmd) SetStderr(w io.Writer)                    { c.cmd.Stderr = w }
func (c *NativeCmd) SetSysProcAttr(attr *syscall.SysProcAttr) { c.cmd.SysProcAttr = attr }
func (c *NativeCmd) GetPid() int {
	if c.cmd.Process != nil {
		return c.cmd.Process.Pid
	}
	return 0
}

// NativeExecutor implements CommandExecutor using the standard os/exec package.
type NativeExecutor struct{}

// CommandContext creates a new NativeCmd.
func (n *NativeExecutor) CommandContext(ctx context.Context, name string, args ...string) Cmd {
	return &NativeCmd{cmd: execwrap.CommandContext(ctx, name, args...)}
}

// MockCmd is a mock implementation of Cmd.
type MockCmd struct {
	OutputBytes []byte
	Err         error
	Stdout      io.Writer
	Stderr      io.Writer
	StartFn     func() error
	WaitFn      func() error
	RunFn       func() error
}

func (c *MockCmd) Start() error {
	if c.StartFn != nil {
		return c.StartFn()
	}
	if c.Stdout != nil && len(c.OutputBytes) > 0 {
		_, _ = c.Stdout.Write(c.OutputBytes)
	}
	return nil
}

func (c *MockCmd) Wait() error {
	if c.WaitFn != nil {
		return c.WaitFn()
	}
	return c.Err
}

func (c *MockCmd) Run() error {
	if c.RunFn != nil {
		// Prefer RunFn; still deliver OutputBytes once for callers that capture stdout.
		if c.Stdout != nil && len(c.OutputBytes) > 0 {
			_, _ = c.Stdout.Write(c.OutputBytes)
		}
		return c.RunFn()
	}
	// Do not call Start() here — Start also writes OutputBytes, and a double write
	// breaks json.Unmarshal on the combined buffer (trailing top-level value).
	if c.Stdout != nil && len(c.OutputBytes) > 0 {
		_, _ = c.Stdout.Write(c.OutputBytes)
	}
	return c.Wait()
}

func (c *MockCmd) Output() ([]byte, error) {
	if c.RunFn != nil {
		err := c.RunFn()
		return c.OutputBytes, err
	}
	return c.OutputBytes, c.Err
}

func (c *MockCmd) SetDir(dir string)                        {}
func (c *MockCmd) SetEnv(env []string)                      {}
func (c *MockCmd) SetStdin(r io.Reader)                     {}
func (c *MockCmd) SetStdout(w io.Writer)                    { c.Stdout = w }
func (c *MockCmd) SetStderr(w io.Writer)                    { c.Stderr = w }
func (c *MockCmd) SetSysProcAttr(attr *syscall.SysProcAttr) {}
func (c *MockCmd) GetPid() int                              { return 999999 }

// MockExecutor implements CommandExecutor for testing purposes.
type MockExecutor struct {
	// CommandContextFn allows overriding the default behavior.
	CommandContextFn func(ctx context.Context, name string, args ...string) Cmd
}

// CommandContext calls CommandContextFn if provided.
func (m *MockExecutor) CommandContext(ctx context.Context, name string, args ...string) Cmd {
	if m.CommandContextFn != nil {
		return m.CommandContextFn(ctx, name, args...)
	}
	return &MockCmd{OutputBytes: []byte(""), Err: nil}
}

// MemoryExecutor is a specialized MockExecutor that returns pre-programmed responses based on command name or args.
func NewMemoryExecutor(responses map[string]*MockCmd) CommandExecutor {
	return &MockExecutor{
		CommandContextFn: func(ctx context.Context, name string, args ...string) Cmd {
			// Try full command signature first
			fullCmd := name
			for _, arg := range args {
				fullCmd += " " + arg
			}
			if mockCmd, ok := responses[fullCmd]; ok {
				return mockCmd
			}
			// Try just the binary name
			if mockCmd, ok := responses[name]; ok {
				return mockCmd
			}
			return &MockCmd{OutputBytes: []byte(""), Err: nil}
		},
	}
}

// Execute is a helper wrapper for convenience (used in some tests).
func (n *NativeExecutor) Execute(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := n.CommandContext(ctx, name, args...)
	cmd.SetDir(dir)
	cmd.SetEnv(env)
	var buf bytes.Buffer
	cmd.SetStdout(&buf)
	cmd.SetStderr(&buf)
	err := cmd.Run()
	return buf.Bytes(), err
}

func (m *MockExecutor) Execute(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := m.CommandContext(ctx, name, args...)
	cmd.SetDir(dir)
	cmd.SetEnv(env)
	var buf bytes.Buffer
	cmd.SetStdout(&buf)
	cmd.SetStderr(&buf)
	err := cmd.Run()
	return buf.Bytes(), err
}
