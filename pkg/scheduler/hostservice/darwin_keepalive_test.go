//go:build darwin

package hostservice

import (
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestKeepAlivePlistXML_CrashOnlyNotUnconditional(t *testing.T) {
	t.Parallel()
	xml := keepAlivePlistXML()
	if strings.Contains(xml, "<true/>") {
		t.Fatalf("KeepAlive must not be unconditional <true/> (overrides intentional stop): %s", xml)
	}
	if !strings.Contains(xml, "SuccessfulExit") || !strings.Contains(xml, "<false/>") {
		t.Fatalf("expected SuccessfulExit=false crash-only KeepAlive, got: %s", xml)
	}
}

func TestSchedulerEnvironmentPlistXMLMarksLongLivedDaemon(t *testing.T) {
	t.Parallel()

	xml := schedulerEnvironmentPlistXML(Entry{AbsRoot: "/tmp/zqk-root"})
	if !strings.Contains(xml, zqkenv.SchedulerDaemonMode().Name()) || !strings.Contains(xml, "<string>1</string>") {
		t.Fatalf("scheduler service must disable the child idle watchdog: %s", xml)
	}
}

// TestDarwinStopDisarmsKeepAlive documents why Stop must bootout (not launchctl stop):
// SuccessfulExit=false KeepAlive respawns after SIGTERM non-zero exits.
func TestDarwinStopDisarmsKeepAlive(t *testing.T) {
	t.Parallel()
	src, err := fileutil.ReadFile("darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	stopIdx := strings.Index(body, "func (a DarwinAdapter) Stop")
	if stopIdx < 0 {
		t.Fatal("DarwinAdapter.Stop not found")
	}
	next := strings.Index(body[stopIdx+1:], "\nfunc ")
	chunk := body[stopIdx:]
	if next > 0 {
		chunk = body[stopIdx : stopIdx+1+next]
	}
	if !strings.Contains(chunk, "bootout") {
		t.Fatalf("DarwinAdapter.Stop must launchctl bootout so KeepAlive cannot respawn; got:\n%s", chunk)
	}
	// Plain stop alone is insufficient with crash-only KeepAlive.
	if strings.Contains(chunk, `exec.Command("launchctl", "stop"`) && !strings.Contains(chunk, "bootout") {
		t.Fatal("launchctl stop without bootout re-breaks intentional service stop")
	}
}
