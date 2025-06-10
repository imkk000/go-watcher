package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
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
		log.Debug().Msg("killing")
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			log.Error().Err(err).Msg("kill command")
		}
		if err := cmd.Wait(); err != nil {
			log.Debug().Err(err).Msg("wait to kill command")
		}
		log.Info().Msgf("killed (%d)", cmd.Process.Pid)
	}
}

func startProcess(ctx context.Context, name string, args ...string) {
	log.Info().Msg("reloading")

	killProcess()

	envs, err := readEnvs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("read environment")
	}

	stdout := NewColoredWriter(os.Stdout, rgb(255, 219, 153))
	cmd = exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stdout
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
	return append(envs, os.Environ()...), nil
}
