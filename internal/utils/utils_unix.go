//go:build !windows
// +build !windows

package utils

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func SignalNum(sigDef string) (syscall.Signal, error) {
	sig := unix.SignalNum(sigDef)
	if sig == 0 {
		return nil, fmt.Errorf("invalid signal: %s", sigDef)
	}
	return sig, nil
}

func SysProcAttrForSetpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}

func Kill(pid int, sig syscall.Signal) (err error) {
	return syscall.Kill(-pid, sig)
}
