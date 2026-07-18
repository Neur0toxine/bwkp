package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Neur0toxine/bwkp/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := cli.New(os.Stdout, os.Stderr).Run(ctx, os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "bwkp:", err)
	}
	return cli.ExitCode(err)
}
