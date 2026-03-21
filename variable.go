package main

import (
	"os/exec"
)

var (
	appVersion = "0.2.4"
	cmd        *exec.Cmd
)

type (
	cancelKey   struct{}
	envFilesKey struct{}
	patternsKey struct{}
)
