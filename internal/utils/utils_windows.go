//go:build windows
// +build windows

package utils

import (
	"fmt"
	"syscall"
)

var (
	libkernel32                  = syscall.MustLoadDLL("kernel32")
	procGenerateConsoleCtrlEvent = libkernel32.MustFindProc("GenerateConsoleCtrlEvent")
)

func SignalNum(sigDef string) (syscall.Signal, error) {
	switch sigDef {
	case "SIGINT":
		return syscall.SIGINT, nil
	case "SIGTERM":
		return syscall.SIGTERM, nil
	default:
		return 0, fmt.Errorf("invalid signal: %s", sigDef)
	}
}

func SysProcAttrForSetpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func Kill(pid int, sig syscall.Signal) error {
	var num uintptr = syscall.CTRL_C_EVENT
	if sig == syscall.SIGTERM {
		num = syscall.CTRL_BREAK_EVENT
	}
	r, _, err := procGenerateConsoleCtrlEvent.Call(num, uintptr(pid))
	if r == 0 {
		return err
	}
	return nil
}
