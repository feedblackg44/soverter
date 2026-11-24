//go:build !windows

package mediainfo

import "os/exec"

func hideWindow(c *exec.Cmd) {}
