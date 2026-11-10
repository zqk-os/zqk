package mcpkeepalive

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	plistXMLHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
	plistDTDLine   = "<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n"
	plistOpenTag   = "<plist version=\"1.0\">\n"
	mcpSubcommand  = "mcp"
	superviseVerb  = "supervise"
	loopFlag       = "--loop"
	tcpFlag        = "--tcp"
)

// LaunchAgentTemplate renders a per-project launchd user agent that supervises
// the MCP KeepAlive daemon. KeepAlive=true plus RunAtLoad=true let launchd
// recycle the supervise loop without the daemon losing supervision, and
// ThrottleInterval bounds restart churn during crash loops.
type LaunchAgentTemplate struct {
	// Label is the launchd label (e.g. com.zqk.mcp-daemon-7100).
	Label string
	// ProjectRoot is the absolute project root the agent runs against.
	ProjectRoot string
	// TCPAddr is the daemon listen address (e.g. 127.0.0.1:7100).
	TCPAddr string
	// Home pins the agent HOME environment.
	Home string
}

// Render returns the fully substituted plist body.
//
// All placeholders are resolved here so shipped agents never contain
// __ZQK_*__ tokens.
func (t LaunchAgentTemplate) Render() string {
	binPath := paths.StableBinaryPath(t.ProjectRoot)
	logsDir := paths.LogsDirPath(t.ProjectRoot)
	pathEnv := filepath.Dir(binPath) + ":" + filepath.Join(t.ProjectRoot, paths.RepoBinDir) + ":/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
	home := t.Home
	var b strings.Builder
	b.WriteString(plistXMLHeader)
	b.WriteString(plistDTDLine)
	b.WriteString(plistOpenTag)
	b.WriteString("<dict>\n")
	b.WriteString("\t<key>Label</key>\n")
	b.WriteString("\t<string>" + t.Label + "</string>\n")
	b.WriteString("\t<key>ProgramArguments</key>\n")
	b.WriteString("\t<array>\n")
	b.WriteString("\t\t<string>" + binPath + "</string>\n")
	b.WriteString("\t\t<string>" + mcpSubcommand + "</string>\n")
	b.WriteString("\t\t<string>" + superviseVerb + "</string>\n")
	b.WriteString("\t\t<string>" + tcpFlag + "</string>\n")
	b.WriteString("\t\t<string>" + t.TCPAddr + "</string>\n")
	b.WriteString("\t\t<string>" + loopFlag + "</string>\n")
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>WorkingDirectory</key>\n")
	b.WriteString("\t<string>" + t.ProjectRoot + "</string>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n")
	b.WriteString("\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n")
	b.WriteString("\t<true/>\n")
	b.WriteString("\t<key>ThrottleInterval</key>\n")
	b.WriteString("\t<integer>5</integer>\n")
	b.WriteString("\t<key>StandardOutPath</key>\n")
	b.WriteString("\t<string>" + logsDir + "/mcp-daemon-keepalive.launchd.out</string>\n")
	b.WriteString("\t<key>StandardErrorPath</key>\n")
	b.WriteString("\t<string>" + logsDir + "/mcp-daemon-keepalive.launchd.err</string>\n")
	b.WriteString("\t<key>EnvironmentVariables</key>\n")
	b.WriteString("\t<dict>\n")
	b.WriteString("\t\t<key>" + zqkenv.ProjectRoot().Name() + "</key>\n")
	b.WriteString("\t\t<string>" + t.ProjectRoot + "</string>\n")
	b.WriteString("\t\t<key>" + zqkenv.IsDaemon().Name() + "</key>\n")
	b.WriteString("\t\t<string>1</string>\n")
	b.WriteString("\t\t<key>HOME</key>\n")
	b.WriteString("\t\t<string>" + home + "</string>\n")
	b.WriteString("\t\t<key>PATH</key>\n")
	b.WriteString("\t\t<string>" + pathEnv + "</string>\n")
	b.WriteString("\t</dict>\n")
	b.WriteString("</dict>\n")
	b.WriteString("</plist>\n")
	return b.String()
}
