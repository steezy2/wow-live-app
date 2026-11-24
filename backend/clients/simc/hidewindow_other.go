//go:build !windows

package simc

import "os/exec"

// hideWindow does nothing off Windows, where no program opens a console window of its own.
func hideWindow(*exec.Cmd) {}
