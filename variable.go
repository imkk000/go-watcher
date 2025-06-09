package main

import (
	"os/exec"
	"regexp"
)

var (
	appVersion     = "0.1.2"
	cmd            *exec.Cmd
	exclusionRegex *regexp.Regexp
	inclusionRegex *regexp.Regexp
	extensionRegex *regexp.Regexp
)

type (
	cancelKey   struct{}
	envFilesKey struct{}
)
