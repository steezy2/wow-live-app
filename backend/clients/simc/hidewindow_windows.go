package simc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is Windows' CREATE_NO_WINDOW: the program gets no console window of its own.
const createNoWindow = 0x08000000

// HideWindow keeps cmd from opening a console window, which simc would otherwise flash on screen each time the app
// starts it.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
