package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/Songmu/gosimilarity"
)

func main() {
	err := gosimilarity.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
	if err != nil && err != flag.ErrHelp {
		handler := slog.NewTextHandler(os.Stderr, nil)
		_ = handler.Handle(context.Background(), slog.Record{Level: slog.LevelError, Message: err.Error()})
		exitCode := 1
		if ecoder, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = ecoder.ExitCode()
		}
		os.Exit(exitCode)
	}
}
