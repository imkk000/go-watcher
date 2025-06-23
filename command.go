package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type Config struct {
	Name     string
	Args     []string
	Duration time.Duration
}

func defaultMatchPatterns() []string {
	return []string{
		"-w:**/.vscode/**",
		"-w:**/.git/**",
		"-w:**/.DS_Store/**",
		"-w:**/.idea/**",
		"-w:**/node_modules/**",
		"-w:**/script/**",
		`r:.+\.go$`,
		`r:.+\.env$`,
		`r:.+\.mod$`,
	}
}

var fileCmd = &cli.Command{
	Aliases: []string{"fs"},
	Name:    "file",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Aliases: []string{"m"},
			Name:    "match",
			Value:   defaultMatchPatterns(),
			Usage:   "set match patterns [+-][rew]:<pattern>",
		},
		&cli.DurationFlag{
			Aliases: []string{"n", "d"},
			Name:    "delay",
			Value:   500 * time.Millisecond,
			Usage:   "set delay duration",
		},
	},
	Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
		inputPatterns := c.StringSlice("match")
		if slices.Compare(inputPatterns, defaultMatchPatterns()) != 0 {
			inputPatterns = append(inputPatterns, defaultMatchPatterns()...)
		}
		patterns := parsePatterns(inputPatterns)
		if len(patterns) == 0 {
			return nil, cli.Exit("no match patterns provided", 1)
		}
		ctx = context.WithValue(ctx, patternsKey{}, patterns)
		log.Debug().
			Interface("patterns", patterns).
			Msg("parse patterns")

		return validateArgs(ctx, c)
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args()
		d := c.Duration("delay")
		log.Info().
			Str("version", appVersion).
			Int("pid", os.Getpid()).
			Strs("command", args.Slice()).
			Msgf("watching command")

		go runFileWatcher(ctx, Config{
			Duration: d,
			Name:     args.First(),
			Args:     args.Tail(),
		})
		killSignal(ctx)

		return nil
	},
}

var commandCmd = &cli.Command{
	Aliases: []string{"cmd"},
	Name:    "command",
	Flags: []cli.Flag{
		&cli.DurationFlag{
			Aliases: []string{"n", "d"},
			Name:    "duration",
			Value:   time.Second,
			Usage:   "set ticker duration",
		},
	},
	Before: validateArgs,
	Action: func(ctx context.Context, c *cli.Command) error {
		args := c.Args()
		d := c.Duration("duration")
		log.Info().
			Str("version", appVersion).
			Int("pid", os.Getpid()).
			Dur("duration", d).
			Strs("command", args.Slice()).
			Msgf("watching command")

		go runCommandWatcher(ctx, Config{
			Duration: d,
			Name:     args.First(),
			Args:     args.Tail(),
		})
		killSignal(ctx)

		return nil
	},
}

var rootCmd = &cli.Command{
	Version:                  appVersion,
	EnableShellCompletion:    true,
	UseShortOptionHandling:   true,
	Suggest:                  true,
	ExitErrHandler:           func(_ context.Context, _ *cli.Command, _ error) {},
	CommandNotFound:          func(context.Context, *cli.Command, string) {},
	OnUsageError:             func(_ context.Context, _ *cli.Command, _ error, _ bool) error { return nil },
	InvalidFlagAccessHandler: func(context.Context, *cli.Command, string) {},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "log-level",
			Value: "info",
			Usage: "set the log level",
		},
		&cli.StringSliceFlag{
			Name:  "env",
			Value: []string{"off"},
			Usage: "set env files",
		},
	},
	Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
		level, err := zerolog.ParseLevel(c.String("log-level"))
		if err != nil {
			return nil, cli.Exit(err, 1)
		}
		zerolog.SetGlobalLevel(level)
		log.Debug().
			Str("log_level", level.String()).
			Msg("set log level")

		envFiles := getEnvFiles(c.StringSlice("env"))

		return context.WithValue(ctx, envFilesKey{}, envFiles), nil
	},
	Commands: []*cli.Command{commandCmd, fileCmd},
}

func getEnvFiles(files []string) []string {
	for i, file := range files {
		if file == "off" {
			return nil
		}
		dir := filepath.Dir(file)
		base := filepath.Base(file)
		if base == "." {
			files[i] = filepath.Join(dir, ".env")
		}
	}

	return files
}

func validateArgs(ctx context.Context, c *cli.Command) (context.Context, error) {
	args := c.Args()
	if args.Len() == 0 {
		return nil, cli.Exit("no command provided to watch", 1)
	}
	return ctx, nil
}
