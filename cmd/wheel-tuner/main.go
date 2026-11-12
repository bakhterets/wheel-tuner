// Command wheel-tuner serves the wheel truing solver API and web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bakhterets/wheel-tuner/internal/api"
	"github.com/bakhterets/wheel-tuner/internal/store"
	"github.com/bakhterets/wheel-tuner/web"
)

const shutdownTimeout = 10 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stderr, nil)
	stop()
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "wheel-tuner:", err)
		os.Exit(1)
	}
}

// run starts the server and blocks until ctx is cancelled. ready, if non-nil,
// receives the bound address once the listener is open.
func run(ctx context.Context, args []string, stderr io.Writer, ready func(addr string)) error {
	fset := flag.NewFlagSet("wheel-tuner", flag.ContinueOnError)
	fset.SetOutput(stderr)
	addr := fset.String("addr", ":8080", "listen address")
	dbPath := fset.String("db", "wheel-tuner.db", "SQLite database file")
	var level slog.Level
	fset.TextVar(&level, "log-level", slog.LevelInfo, "log level: debug, info, warn, error")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fset.Args())
	}

	handler := slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})
	log := slog.New(handler)

	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := st.Close(); cerr != nil {
			log.Error("close database", "err", cerr)
		}
	}()

	static, err := web.Static()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           api.New(st, api.WithLogger(log), api.WithStatic(static)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(handler, slog.LevelWarn),
	}

	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *addr)
	if err != nil {
		return err
	}
	log.Info("listening", "addr", ln.Addr().String(), "db", *dbPath)
	if ready != nil {
		ready(ln.Addr().String())
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
