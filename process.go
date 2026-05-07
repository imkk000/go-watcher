package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

func reapZombieProcess() {
	ch := make(chan os.Signal, 1)
	defer close(ch)
	signal.Notify(ch, syscall.SIGCHLD)

	var status syscall.WaitStatus
	for range ch {
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err != nil {
			if errors.Is(err, syscall.ECHILD) {
				continue
			}
			log.Error().Err(err).Msg("wait for child process")
			continue
		}
		log.Debug().Msgf("reaped zombie process (%d)", pid)
	}
}

func killProcess() {
	if cmd != nil {
		log.Debug().Str("signal", killSig.String()).Msg("killing")
		if err := syscall.Kill(-cmd.Process.Pid, killSig); err != nil {
			log.Debug().Err(err).Msg("kill command")
		}
		if err := cmd.Wait(); err != nil {
			log.Debug().Err(err).Msg("wait to kill command")
		}
		log.Info().Msgf("killed (%d)", cmd.Process.Pid)
	}
}

func parseSignal(s string) (syscall.Signal, error) {
	switch strings.ToUpper(strings.TrimPrefix(strings.ToUpper(s), "SIG")) {
	case "KILL":
		return syscall.SIGKILL, nil
	case "TERM":
		return syscall.SIGTERM, nil
	case "HUP":
		return syscall.SIGHUP, nil
	case "INT":
		return syscall.SIGINT, nil
	default:
		return 0, fmt.Errorf("unsupported signal %q: must be SIGKILL, SIGTERM, SIGHUP, or SIGINT", s)
	}
}

// filteredWriter buffers subprocess output and only forwards lines matching filter.
type filteredWriter struct {
	w      io.Writer
	filter *regexp.Regexp
	buf    []byte
}

func (f *filteredWriter) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	for {
		i := bytes.IndexByte(f.buf, '\n')
		if i < 0 {
			break
		}
		line := f.buf[:i+1]
		f.buf = f.buf[i+1:]
		if f.filter.Match(line) {
			if _, err := f.w.Write(line); err != nil {
				return 0, err
			}
		}
	}
	return len(p), nil
}

func startProcess(ctx context.Context, c Config) {
	log.Info().Msg("reloading")

	killProcess()

	if c.LineCh != nil {
		c.LineCh <- reloadMark
	}

	envs, err := readEnvs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("read environment")
	}

	var out io.Writer
	if c.LineCh != nil {
		out = &tuiWriter{ch: c.LineCh}
	} else {
		colored := NewColoredWriter(os.Stdout, rgb(168, 149, 90))
		if c.LogFilter != nil {
			out = &filteredWriter{w: colored, filter: c.LogFilter}
		} else {
			out = colored
		}
	}

	cmd = exec.Command(c.Name, c.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Env = envs
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Msg("start command")
	}
	log.Info().Msgf("started (%d)", cmd.Process.Pid)
}

func killSignal(ctx context.Context) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	log.Info().Msg("killing watcher")
	ctx.Value(cancelKey{}).(context.CancelFunc)()
}

func readEnvs(ctx context.Context) ([]string, error) {
	files := ctx.Value(envFilesKey{}).([]string)
	if len(files) == 0 {
		return nil, nil
	}
	env, err := godotenv.Read(files...)
	if err != nil {
		return nil, err
	}
	envs := make([]string, 0, len(env))
	for k, v := range env {
		envs = append(envs, k+"="+v)
	}
	return append(os.Environ(), envs...), nil
}
