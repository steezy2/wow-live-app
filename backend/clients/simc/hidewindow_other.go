//go:build !windows

package simc

import "os/exec"

// HideWindow does nothing off Windows, where no program opens a console window of its own.
func HideWindow(*exec.Cmd) {}
