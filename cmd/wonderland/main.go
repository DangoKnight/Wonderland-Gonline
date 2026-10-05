package main

import (
	"context"
	"encoding/json"
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
	"wonderland-go/internal/admin"
	"wonderland-go/internal/assetsql"
	"wonderland-go/internal/config"
	"wonderland-go/internal/server"
	"wonderland-go/internal/store"
)

func main() {
	if e := run(); e != nil {
		slog.Error("server stopped", "error", e)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "", "configuration JSON (defaults if omitted)")
	inspect := flag.Bool("inspect-data", false, "inspect SQL asset counts without starting listeners")
	debug := flag.Bool("debug", false, "log packet metadata, Lucky Draw packets and native fishing controls")
	logPath := flag.String("log-file", "", "append server logs to this file as well as stderr")
	flag.Parse()
	c, e := config.Load(*path)
	if e != nil {
		return e
	}
	var logOutput io.Writer = os.Stderr
	if *logPath != "" {
		file, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer file.Close()
		logOutput = io.MultiWriter(os.Stderr, file)
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(logOutput, &slog.HandlerOptions{Level: level}))
	a, e := assetsql.LoadDatabase(c.AssetsDatabase)
	if e != nil {
		return e
	}
	if *inspect {
		return json.NewEncoder(os.Stdout).Encode(a.Summary())
	}
	token := os.Getenv("WONDERLAND_ADMIN_TOKEN")
	if len(token) < 24 {
		return fmt.Errorf("set WONDERLAND_ADMIN_TOKEN to a secret of at least 24 characters")
	}
	db, e := store.Open(c.Database)
	if e != nil {
		return e
	}
	defer db.Close()
	s := server.New(c, db, a, log)
	s.SetConfigurationPath(*path)
	settings, e := db.Settings(context.Background())
	if e != nil {
		return e
	}
	if name := settings["server_name"]; name != "" {
		s.SetName(name)
	}
	if e = s.LoadRuntimeSettings(context.Background()); e != nil {
		return e
	}
	if e = s.LoadIPBans(context.Background()); e != nil {
		return e
	}
	handler, e := admin.New(s, token)
	if e != nil {
		return e
	}
	httpServer := &http.Server{Addr: c.HTTP, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	l, e := net.Listen("tcp", c.HTTP)
	if e != nil {
		return e
	}
	defer l.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errs := make(chan error, 2)
	go func() { errs <- httpServer.Serve(l) }()
	go func() { errs <- s.Run(ctx) }()
	log.Info("web administration listening", "address", l.Addr(), "parity", "incomplete", "assets", a.Summary())
	received := false
	select {
	case e = <-errs:
		received = true
	case <-ctx.Done():
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		httpServer.Close()
	}
	// Both workers must finish before the database is closed.
	if !received {
		<-errs
	}
	<-errs
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
