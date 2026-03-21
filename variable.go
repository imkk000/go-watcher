package main

import (
	"os/exec"
)

var (
	appVersion = "0.2.3"
	cmd        *exec.Cmd
)

type (
	cancelKey   struct{}
	envFilesKey struct{}
	patternsKey struct{}
)
