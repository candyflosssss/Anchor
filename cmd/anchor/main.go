package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/candyflosssss/Anchor/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	code := cli.Main(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
