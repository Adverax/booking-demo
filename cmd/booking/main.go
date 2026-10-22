// Command booking is the Service Bootstrap (COMP-007): it reads its configuration from environment
// variables (CON-005, ADR-0005), opens and migrates the SQLite store (ADR-0007), wires the
// components and serves the generated HTTP API on http.ServeMux (ADR-0009) until SIGINT or SIGTERM.
//
// Environment:
//
//	BOOKING_DB_PATH  path of the SQLite database file; required, created if missing
//	BOOKING_ADDR     TCP address to listen on; default ":8080"
//
// The service must run as a single instance: SQLite allows one writer process (ADR-0005). That is
// not enforced here (DEC-029).
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adverax/booking-demo/internal/booking"
	"github.com/adverax/booking-demo/internal/catalog"
	"github.com/adverax/booking-demo/internal/httpapi"
	"github.com/adverax/booking-demo/internal/store"
)

const defaultAddr = ":8080"

// HTTP server bounds. readTimeout bounds a slow or stalled client; writeTimeout is above the
// longest a handler can wait on the store (one busy timeout of 5 s per statement); shutdownTimeout
// is how long in-flight requests get to finish after a stop signal.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "booking:", err)
		os.Exit(1)
	}
}

type config struct {
	dbPath string
	addr   string
}

func loadConfig(getenv func(string) string) (config, error) {
	c := config{dbPath: getenv("BOOKING_DB_PATH"), addr: getenv("BOOKING_ADDR")}
	if c.dbPath == "" {
		return config{}, errors.New("BOOKING_DB_PATH is not set")
	}
	if c.addr == "" {
		c.addr = defaultAddr
	}
	return c, nil
}

// run serves until ctx is cancelled, then shuts down gracefully. It returns an error when the
// service could not start, failed while serving, or did not drain within shutdownTimeout.
func run(ctx context.Context, getenv func(string) string) (err error) {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}
	st, err := store.Open(ctx, cfg.dbPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, st.Close()) }()

	ln, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		return err
	}
	cat := catalog.New(st)
	srv := &http.Server{
		Handler:           httpapi.NewHandler(httpapi.NewServer(cat, booking.NewService(st, cat, time.Now))),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Handlers still running are abandoned, not stopped; store.Close below waits for the
		// statements they have in flight, each bounded by the store's busy timeout.
		return errors.Join(fmt.Errorf("shutdown: %w", err), srv.Close())
	}
	return nil
}
