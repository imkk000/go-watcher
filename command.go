package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

var fileCmd = &cli.Command{
	Aliases: []string{"fs"},
	Name:    "file",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Aliases: []string{"e"},
			Name:    "exclusions",
			Value: []string{
				".git", ".DS_Store", ".idea",
				".vscode", "node_modules", "script",
			},
			Usage: "set exclusion patterns",
		},
		&cli.StringSliceFlag{
			Aliases: []string{"i"},
			Name:    "inclusions",
			Value:   []string{},
			Usage:   "set inclusion patterns",
		},
		&cli.StringSliceFlag{
			Aliases: []string{"s"},
			Name:    "extensions",
			Value:   []string{".go", ".env", ".mod"},
			Usage:   "set allow file extensions",
		},
		&cli.DurationFlag{
			Aliases: []string{"n", "d"},
			Name:    "delay",
			Value:   500 * time.Millisecond,
			Usage:   "set delay duration",
		},
	},
	Before: func(ctx context.Context, c *cli.Command) (context.Context, error) {
		if err := parseRegexps(c); err != nil {
			return nil, err
		}
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

		go runFileWatcher(ctx, d, args.First(), args.Tail()...)
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

		go runCommandWatcher(ctx, d, args.First(), args.Tail()...)
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

		envFiles := c.StringSlice("env")
		if len(envFiles) > 0 {
			if err := parseEnvFiles(envFiles); err != nil {
				err = fmt.Errorf("parse env file: %w", err)
				return nil, cli.Exit(err, 1)
			}
		}
		return ctx, nil
	},
	Commands: []*cli.Command{commandCmd, fileCmd},
}

func parseEnvFiles(files []string) error {
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
	log.Info().
		Strs("env", files).
		Msg("parse env files")

	return godotenv.Load(files...)
}

func validateArgs(ctx context.Context, c *cli.Command) (context.Context, error) {
	args := c.Args()
	if args.Len() == 0 {
		return nil, cli.Exit("no command provided to watch", 1)
	}
	return ctx, nil
}

func parseRegexps(c *cli.Command) error {
	fn := func(r **regexp.Regexp, key string) error {
		raw := joinPipe(c.StringSlice(key))
		regex, err := regexp.Compile(raw)
		if err != nil {
			err := fmt.Errorf("invalid %s regex", key)
			return cli.Exit(err, 1)
		}
		*r = regex

		log.Debug().
			Str("rules", raw).
			Msgf("compile %s regex", key)

		return nil
	}
	if err := fn(&exclusionRegex, "exclusions"); err != nil {
		return err
	}
	if err := fn(&inclusionRegex, "inclusions"); err != nil {
		return err
	}
	if err := fn(&extensionRegex, "extensions"); err != nil {
		return err
	}
	return nil
}
