//go:build !windows

package firstlaunch

import "os/exec"

func hidePickerConsole(*exec.Cmd) {}
