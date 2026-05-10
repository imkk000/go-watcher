package main

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var (
	appVersion = "0.4.3"
	cmd        *exec.Cmd
	killSig    syscall.Signal = syscall.SIGKILL

	procState struct {
		sync.RWMutex
		pid         int
		startedAt   time.Time
		reloadCount int
	}
)

type (
	cancelKey   struct{}
	envFilesKey struct{}
	rulesKey    struct{}
)

func setProcStarted(pid int) {
	procState.Lock()
	procState.pid = pid
	procState.startedAt = time.Now()
	procState.reloadCount++
	procState.Unlock()
}

func procStatusString() string {
	procState.RLock()
	defer procState.RUnlock()
	if procState.pid == 0 {
		return ""
	}
	d := time.Since(procState.startedAt).Truncate(time.Second)
	return fmt.Sprintf("pid %d · up %s · reloads %d", procState.pid, d, procState.reloadCount)
}
