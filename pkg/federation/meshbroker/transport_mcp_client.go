package meshbroker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/federation"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

type pooledClient struct {
	client *mcp.Client
	cmd    *exec.Cmd
	conn   net.Conn

	initOnce sync.Once
	initErr  error
}

func (p *pooledClient) close() {
	if p.client != nil {
		_ = p.client.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}

// MCPClientTransport implements federation.Transport using a real MCP client connected
// to a remote kernel via stdio (for local processes) or TCP/HTTP.
type MCPClientTransport struct {
	BinaryPath string // Used for stdio sub-process mode currently

	mu   sync.Mutex
	pool map[string]*pooledClient
}

func NewMCPClientTransport(binaryPath string) *MCPClientTransport {
	return &MCPClientTransport{
		BinaryPath: binaryPath,
		pool:       make(map[string]*pooledClient),
	}
}

func (t *MCPClientTransport) mcpArgv() (string, []string) {
	return paths.MCPServeArgv(t.BinaryPath, "")
}

func (t *MCPClientTransport) SendHandshake(ctx context.Context, endpoint string, req federation.HandshakeRequest) (*federation.HandshakeResponse, error) {
	cliTransport := federation.NewLocalCLITransport(paths.ResolveProductCLI(""))
	return cliTransport.SendHandshake(ctx, endpoint, req)
}

func (t *MCPClientTransport) SendHeartbeat(ctx context.Context, endpoint string, kernelID string) error {
	return nil
}

func (t *MCPClientTransport) getClient(ctx context.Context, endpoint string) (*mcp.Client, error) {
	var pc *pooledClient
	_ = concurrency.RunInLock(&t.mu, func() error {
		var exists bool
		pc, exists = t.pool[endpoint]
		if !exists {
			pc = &pooledClient{}
			t.pool[endpoint] = pc
		}
		return nil
	})

	pc.initOnce.Do(func() {
		pc.initErr = t.initializeClient(ctx, endpoint, pc)
	})

	if pc.initErr != nil {
		// If initialization failed, remove it so future calls can retry
		_ = concurrency.RunInLock(&t.mu, func() error {
			if existing, ok := t.pool[endpoint]; ok && existing == pc {
				delete(t.pool, endpoint)
			}
			return nil
		})
		return nil, pc.initErr
	}

	return pc.client, nil
}

func (t *MCPClientTransport) initializeClient(ctx context.Context, endpoint string, pc *pooledClient) error {
	if strings.HasPrefix(endpoint, "tcp://") {
		addr := strings.TrimPrefix(endpoint, "tcp://")
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return errfmt.Newf("failed to connect to %s", addr).Wrap(err)
		}
		pc.conn = conn
		pc.client = mcp.NewClient(conn, conn, mcp.NewDefaultTransport())
	} else {
		// Use a bounded context for subprocess lifecycle — prevents infinite hangs
		// if the MCP server binary never responds (fixes deadlock BLI-1781377164712779000).
		initTimeout := 15 * time.Second
		cmdCtx, cmdCancel := context.WithTimeout(ctx, initTimeout)
		// NOTE: cmdCancel is deferred further down after cmd.Start() succeeds,
		// but the exec.CommandContext will kill the process if the context expires.
		bin, args := t.mcpArgv()
		cmd := execwrap.CommandContext(cmdCtx, bin, args...)
		cmd.Env = os.Environ()
		if endpoint != "" && !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			cmd.Env = append(cmd.Env, zqkenv.ProjectRoot().Name()+"="+endpoint)
		}

		apiKey := zqkenv.APIKey().Get()
		if apiKey != "" {
			cmd.Env = append(cmd.Env, zqkenv.APIKey().Name()+"="+apiKey)
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cmdCancel()
			return errfmt.Newf("mcp stdout pipe").Wrap(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			cmdCancel()
			return errfmt.Newf("mcp stdin pipe").Wrap(err)
		}

		// Remove Stderr redirection because it causes deadlock in mcp.NewClient if the server logs a lot and pipe blocks.
		// Or pipe it to os.Stderr asynchronously.
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			cmdCancel()
			return errfmt.Newf("start mcp server").Wrap(err)
		}

		goroutinelabels.NewGoroutine("mcp_client_reap", "reap MCP server subprocess and release context on exit").
			StartSimple(func() {
				_ = cmd.Wait()
				cmdCancel() // Release context resources after process exits
			})

		pc.cmd = cmd
		pc.client = mcp.NewClient(stdout, stdin, mcp.NewDefaultTransport())
	}

	// Initialize the MCP connection with strict timeout per POL-WORKFLOW-006
	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientName := brand.ExecutableName() + "-meshbroker"

	initParams := mcp.InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]any{
			objects.FieldKeyClientID: brand.NamespacePrefix() + ":meshbroker",
		},
	}
	initParams.ClientInfo.Name = clientName
	initParams.ClientInfo.Version = "1.0.0"

	_, err := pc.client.Initialize(initCtx, initParams)
	if err != nil {
		pc.close()
		return errfmt.Newf("mcp initialize failed").Wrap(err)
	}

	return nil
}

func (t *MCPClientTransport) ExecuteTool(ctx context.Context, endpoint string, toolName string, arguments map[string]any) (json.RawMessage, error) {

	client, err := t.getClient(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := client.CallTool(callCtx, toolName, arguments)
	if err != nil {
		// If the connection is broken, we should remove it from the pool
		_ = concurrency.RunInLock(&t.mu, func() error {
			if pc, exists := t.pool[endpoint]; exists && pc.client == client {
				pc.close()
				delete(t.pool, endpoint)
			}
			return nil
		})
		return nil, errfmt.Newf("mcp tool call failed: %s", toolName).Wrap(err)
	}

	return result, nil
}

// Close shuts down all pooled clients and their underlying processes.
func (t *MCPClientTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ep, pc := range t.pool {
		pc.close()
		delete(t.pool, ep)
	}
	return nil
}
