package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

var (
	appVersion    = "0.0.4"
	excludeRegex  *regexp.Regexp
	includeRegex  *regexp.Regexp
	allowExtRegex *regexp.Regexp
	runCmd        *exec.Cmd
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Panic().Err(err.(error)).Msg("application panic")
			if runCmd != nil {
				killCommand(runCmd)
			}
			os.Exit(1)
		}
	}()
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:     os.Stdout,
		NoColor: false,
		FormatTimestamp: func(_ any) string {
			return ""
		},
		FormatLevel: func(l any) string {
			if level, ok := l.(string); ok {
				switch level {
				case zerolog.LevelErrorValue, zerolog.LevelFatalValue, zerolog.LevelPanicValue:
					return sprintRGB(255, 0, 0, strings.ToUpper(level))
				}
				return sprintRGB(102, 163, 255, strings.ToUpper(level))
			}
			return ""
		},
		FormatMessage: func(f any) string {
			if msg, ok := f.(string); ok {
				return sprintRGB(255, 192, 203, msg)
			}
			return ""
		},
	})

	commandCmd := &cli.Command{
		Aliases: []string{"cmd"},
		Name:    "command",
		Flags: []cli.Flag{
			&cli.DurationFlag{
				Aliases: []string{"n", "d"},
				Name:    "duration",
				Value:   time.Second,
				Usage:   "set duration for the command to run",
			},
		},
		Action: newCommandAction,
	}
	fileCmd := &cli.Command{
		Aliases: []string{"fs"},
		Name:    "file",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Aliases: []string{"e"},
				Name:    "exclude-dirs",
				Value:   []string{".git", ".DS_Store", ".idea", ".vscode", "node_modules", "script"},
				Usage:   "set exclude directories pattern",
			},
			&cli.StringSliceFlag{
				Aliases: []string{"i"},
				Name:    "include-dirs",
				Value:   []string{},
				Usage:   "set include directories pattern",
			},
			&cli.StringSliceFlag{
				Aliases: []string{"e"},
				Name:    "extension",
				Value:   []string{".go", ".env", ".mod"},
				Usage:   "set allow file extensions",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			excludeDirs := cmd.StringSlice("exclude-dirs")
			includeDirs := cmd.StringSlice("include-dirs")
			extensions := cmd.StringSlice("extension")

			raw := strings.Join(excludeDirs, "|")
			raw = strings.ReplaceAll(raw, ",", "|")
			raw = fmt.Sprintf("(%s)", raw)
			regex, err := regexp.Compile(raw)
			if err != nil {
				return nil, cli.Exit("invalid exclude directories regex", 1)
			}
			excludeRegex = regex
			log.Debug().Str("raw", raw).Msg("compile exclude regex")

			raw = strings.Join(excludeDirs, "|")
			raw = strings.ReplaceAll(raw, ",", "|")
			raw = fmt.Sprintf("(%s)", raw)
			regex, err = regexp.Compile(raw)
			if err != nil {
				return nil, cli.Exit("invalid include directories regex", 1)
			}
			includeRegex = regex
			log.Debug().Str("raw", raw).Msg("compile include regex")

			raw = strings.Join(extensions, "|")
			raw = strings.ReplaceAll(raw, ",", "|")
			raw = fmt.Sprintf("^(%s)$", raw)
			regex, err = regexp.Compile(raw)
			if err != nil {
				return nil, cli.Exit("invalid file extensions regex", 1)
			}
			allowExtRegex = regex
			log.Debug().Str("raw", raw).Msg("compile extension regex")

			return ctx, nil
		},
		Action: newFileAction,
	}
	cmds := map[string]string{
		"gorun":  "go run cmd/main.go",
		"golint": "golangci-lint run --config=~/.config/nvim/linters/golangci.yaml --output.tab.path stdout",
		"gotest": "go test ./...",
	}
	for name, cmd := range cmds {
		fields := strings.Fields(cmd)
		wrapAction := wrapAction(fields[0], fields[1:]...)
		fileCmd.Commands = append(fileCmd.Commands, &cli.Command{
			Name:                  name,
			EnableShellCompletion: true,
			Action:                wrapAction,
		})
	}

	rootCmd := &cli.Command{
		Version:                appVersion,
		EnableShellCompletion:  true,
		UseShortOptionHandling: true,
		ExitErrHandler: func(ctx context.Context, c *cli.Command, err error) {
		},
		Flags: []cli.Flag{
			&cli.Int8Flag{
				Aliases: []string{"l"},
				Name:    "log-level",
				Value:   1,
				Usage:   "set the log level (0: DEBUG, 1: INFO)",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			level := zerolog.Level(cmd.Int8("log-level"))
			zerolog.SetGlobalLevel(level)
			log.Debug().Str("log_level", level.String()).Msg("set log level")

			return ctx, nil
		},
		Commands: []*cli.Command{commandCmd, fileCmd},
	}
	if err := rootCmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal().Err(err).Msg("run application")
	}
}

func newCommandAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return cli.Exit("no command provided to watch", 1)
	}
	log.Info().Strs("cmd", args.Slice()).Msgf("watching command")

	go reapZombieProcesses()

	d := cmd.Duration("duration")
	ticker := time.NewTicker(d)
	defer ticker.Stop()

	runCommand := func() *exec.Cmd {
		cmd := exec.Command(args.First(), args.Tail()...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		runCmd = cmd
		if err := cmd.Run(); err != nil {
			log.Error().Err(err).Msg("run command")
		}
		return cmd
	}
	runCmd := runCommand()

	go func() {
		for range ticker.C {
			clearScreen()

			log.Info().Msgf("running command every %s\n", d)
			runCmd = runCommand()
		}
	}()

	sig := make(chan os.Signal, 1)
	defer close(sig)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	killCommand(runCmd)

	return nil
}

func newFileAction(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return cli.Exit("no command provided to watch", 1)
	}
	log.Info().Strs("cmd", args.Slice()).Msgf("watching command")

	action := wrapAction(args.First(), args.Tail()...)
	return action(ctx, cmd)
}

func wrapAction(name string, args ...string) cli.ActionFunc {
	return func(_ context.Context, cmd *cli.Command) error {
		go reapZombieProcesses()
		go runWatcher(name, args...)

		runCmd = startCommand(runCmd, name, args...)

		sig := make(chan os.Signal, 1)
		defer close(sig)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		killCommand(runCmd)

		return nil
	}
}

func runWatcher(name string, args ...string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal().Err(err).Msg("new watcher")
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			log.Error().Err(err).Msg("close watcher")
		}
	}()

	rootPath, err := os.Getwd()
	if err != nil {
		log.Fatal().Err(err).Msg("get working directory")
	}
	log.Debug().Str("wd", rootPath).Msg("get wd")

	if err := walkDir(rootPath, watcher); err != nil {
		log.Fatal().Err(err).Msg("walk directory")
	}

	var debouncer *time.Timer

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				log.Debug().Msg("channel closed")
				return
			}
			info, err := os.Stat(event.Name)
			if err != nil {
				continue
			}
			if info.IsDir() {
				if err := walkDir(event.Name, watcher); err != nil {
					log.Debug().Err(err).Str("path", event.Name).Msg("add path")
					continue
				}
				log.Info().Str("path", event.Name).Msg("add path")
				continue
			}
			ext := strings.ToLower(filepath.Ext(event.Name))
			if !allowExtRegex.MatchString(ext) {
				continue
			}
			switch event.Op {
			case fsnotify.Create, fsnotify.Write, fsnotify.Rename:
			default:
				continue
			}
			if debouncer != nil {
				debouncer.Stop()
			}
			debouncer = time.AfterFunc(500*time.Millisecond, func() {
				runCmd = startCommand(runCmd, name, args...)
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				log.Debug().Msg("channel closed")
				return
			}
			log.Error().Err(err).Msg("received error")
		}
	}
}

func walkDir(path string, watcher *fsnotify.Watcher) error {
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, _ error) error {
		if !d.IsDir() {
			return nil
		}
		if !includeRegex.MatchString(strings.ToLower(path)) &&
			excludeRegex.MatchString(strings.ToLower(path)) {
			return nil
		}
		log.Debug().Str("path", path).Msg("add path")
		return watcher.Add(path)
	})
}

func startCommand(cmd *exec.Cmd, name string, args ...string) *exec.Cmd {
	log.Info().Msg("reloading")
	killCommand(cmd)

	stdout := NewColoredWriter(os.Stdout, rgb(255, 219, 153))
	cmd = exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Msg("run command")
	}
	log.Info().Msgf("started (%d)", cmd.Process.Pid)

	return cmd
}

func killCommand(cmd *exec.Cmd) {
	if cmd != nil {
		log.Info().Msgf("killing (%d)", cmd.Process.Pid)
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			log.Debug().Err(err).Msg("kill previous process")
		}
		if err := cmd.Wait(); err != nil {
			log.Debug().Err(err).Msg("wait to finish process")
		}
	}
}

func reapZombieProcesses() {
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

func clearScreen() {
	fmt.Print(ClearScreen)
}
