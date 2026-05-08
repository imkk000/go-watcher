package main

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasShellMeta(t *testing.T) {
	tcs := []struct {
		name string
		cmd  string
		args []string
		want bool
	}{
		{"plain command", "go", []string{"run", "."}, false},
		{"plain with dots and dashes", "go", []string{"test", "-race", "./..."}, false},
		{"empty args", "ls", nil, false},
		{"name only, no args", "make", nil, false},

		{"pipe as standalone arg", "echo", []string{"a", "|", "base64"}, true},
		{"pipe inside one arg", "sh", []string{"echo a | b"}, true},
		{"double pipe (||) standalone", "echo", []string{"a", "||", "echo", "b"}, true},

		{"redirect >", "echo", []string{"hi", ">", "out"}, true},
		{"redirect <", "tee", []string{"<", "in"}, true},
		{"semicolon", "echo", []string{"a", ";", "echo", "b"}, true},

		{"&& standalone", "make", []string{"build", "&&", "make", "test"}, true},
		{"& standalone (background)", "long", []string{"&"}, true},

		{"name itself contains pipe", "weird|name", nil, true},

		{"url with & is NOT shell", "curl", []string{"https://x.test?a=1&b=2"}, false},
		{"hash arg is NOT shell", "echo", []string{"#commented"}, false},
		{"dollar sign arg is NOT auto-detected", "echo", []string{"$HOME"}, false},
		{"glob arg is NOT auto-detected", "ls", []string{"*.go"}, false},
		{"asterisk in path NOT auto-detected", "echo", []string{"a*b"}, false},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got := hasShellMeta(tc.cmd, tc.args)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildExec(t *testing.T) {
	tcs := []struct {
		name     string
		cfg      Config
		wantName string
		wantArgs []string
	}{
		{
			name:     "plain — direct exec",
			cfg:      Config{Name: "go", Args: []string{"run", "."}},
			wantName: "go",
			wantArgs: []string{"run", "."},
		},
		{
			name:     "shell flag forces wrap even without metachars",
			cfg:      Config{Name: "echo", Args: []string{"hi"}, Shell: true},
			wantName: "sh",
			wantArgs: []string{"-c", "echo hi"},
		},
		{
			name:     "shell flag with no args",
			cfg:      Config{Name: "make", Shell: true},
			wantName: "sh",
			wantArgs: []string{"-c", "make"},
		},
		{
			name:     "auto-wrap on standalone pipe",
			cfg:      Config{Name: "echo", Args: []string{"a", "|", "base64"}},
			wantName: "sh",
			wantArgs: []string{"-c", "echo a | base64"},
		},
		{
			name:     "auto-wrap on quoted command in single arg",
			cfg:      Config{Name: "sh-cmd", Args: []string{"echo a | base64"}},
			wantName: "sh",
			wantArgs: []string{"-c", "sh-cmd echo a | base64"},
		},
		{
			name:     "auto-wrap on &&",
			cfg:      Config{Name: "make", Args: []string{"build", "&&", "make", "test"}},
			wantName: "sh",
			wantArgs: []string{"-c", "make build && make test"},
		},
		{
			name:     "auto-wrap on redirect",
			cfg:      Config{Name: "echo", Args: []string{"hi", ">", "out.txt"}},
			wantName: "sh",
			wantArgs: []string{"-c", "echo hi > out.txt"},
		},
		{
			name:     "url with & stays direct",
			cfg:      Config{Name: "curl", Args: []string{"https://x.test?a=1&b=2"}},
			wantName: "curl",
			wantArgs: []string{"https://x.test?a=1&b=2"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotArgs := buildExec(tc.cfg)
			assert.Equal(t, tc.wantName, gotName)
			assert.Equal(t, tc.wantArgs, gotArgs)
		})
	}
}

func TestParseSignal(t *testing.T) {
	tcs := []struct {
		in      string
		want    syscall.Signal
		wantErr bool
	}{
		{"KILL", syscall.SIGKILL, false},
		{"sigkill", syscall.SIGKILL, false},
		{"SIGTERM", syscall.SIGTERM, false},
		{"term", syscall.SIGTERM, false},
		{"HUP", syscall.SIGHUP, false},
		{"INT", syscall.SIGINT, false},
		{"USR1", syscall.SIGUSR1, false},
		{"USR2", syscall.SIGUSR2, false},
		{"QUIT", syscall.SIGQUIT, false},

		{"9", syscall.SIGKILL, false},
		{"15", syscall.SIGTERM, false},
		{"1", syscall.SIGHUP, false},
		{"2", syscall.SIGINT, false},
		{"3", syscall.SIGQUIT, false},
		{" 9 ", syscall.SIGKILL, false},

		{"", 0, true},
		{"bogus", 0, true},
		{"99", 0, true},
		{"0", 0, true},
		{"-1", 0, true},
	}
	for _, tc := range tcs {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseSignal(tc.in)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
