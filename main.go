package main

import (
	"context"
	"os"

	"github.com/rs/zerolog/log"
)

func handleRecover() {
	if r := recover(); r != nil {
		if err, ok := r.(error); ok {
			log.Panic().Err(err).Msg("application panic")
		}
		log.Panic().Msgf("application panic: %v", r)
	}
}

func main() {
	log.Logger = logger

	defer handleRecover()
	defer killProcess()

	go reapZombieProcess()

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	ctx = context.WithValue(ctx, cancelKey{}, cancel)

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		log.Fatal().Err(err).Msg("run application")
	}
}
