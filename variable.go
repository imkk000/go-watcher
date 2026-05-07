package main

import (
	"os/exec"
	"syscall"
)

var (
	appVersion = "0.4.1"
	cmd        *exec.Cmd
	killSig    syscall.Signal = syscall.SIGKILL
)

type (
	cancelKey   struct{}
	envFilesKey struct{}
	rulesKey    struct{}
)
