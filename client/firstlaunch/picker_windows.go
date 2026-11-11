package firstlaunch

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000 // Windows CREATE_NO_WINDOW; retain native folder dialogs.

func hidePickerConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
