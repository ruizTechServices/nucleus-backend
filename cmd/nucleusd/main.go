package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	config, err := loadAppConfigFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "load nucleusd config: %v\n", err)
		os.Exit(1)
	}

	app, err := newApp(config)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "initialize nucleusd: %v\n", err)
		os.Exit(1)
	}

	if err := app.writeStartupInfo(os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "write startup info: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "run nucleusd: %v\n", err)
		os.Exit(1)
	}
}
