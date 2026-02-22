package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hellqvio/hec2logstashhttp/internal/app"
	"github.com/hellqvio/hec2logstashhttp/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, stdout io.Writer) int {
	flagSet := flag.NewFlagSet("hec2logstashhttp", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	showVersion := flagSet.Bool("version", false, "print version and exit")
	if err := flagSet.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "version=%s commit=%s date=%s\n", version.Version, version.Commit, version.Date)
		return 0
	}

	cfg := app.LoadConfigFromEnv()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))

	server := app.NewServer(cfg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "err", err)
		}
	}()

	logger.Info(
		"starting hec shim",
		"listen_addr", cfg.ListenAddr,
		"forward_url", cfg.ForwardURL,
		"version", version.Version,
		"commit", version.Commit,
		"build_date", version.Date,
	)

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server exited with error", "err", err)
		return 1
	}

	time.Sleep(50 * time.Millisecond)
	logger.Info("server stopped")
	return 0
}
