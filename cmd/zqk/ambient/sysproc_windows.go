//go:build windows

package ambient

import "os/exec"

func setDaemonSysProcAttr(cmd *exec.Cmd) {}
