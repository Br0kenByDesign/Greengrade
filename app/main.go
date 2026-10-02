// greengrade - private cannabis rating log with optional public ratings.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Br0kenByDesign/Greengrade/internal/api"
	"github.com/Br0kenByDesign/Greengrade/internal/config"
	"github.com/Br0kenByDesign/Greengrade/internal/db"
	"github.com/Br0kenByDesign/Greengrade/internal/mail"
	"github.com/Br0kenByDesign/Greengrade/internal/media"
	"github.com/Br0kenByDesign/Greengrade/webui"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		res, err := http.Get("http://127.0.0.1:8080/healthz")
		if err != nil || res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if err := run(); err != nil {
		slog.Error("start fehlgeschlagen", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	sender, err := mail.New(cfg)
	if err != nil {
		return err
	}
	store, err := media.NewStore(cfg.DataDir)
	if err != nil {
		return err
	}
	srv, err := api.New(cfg, pool, sender, store, webui.FS())
	if err != nil {
		return err
	}
	go srv.Housekeeping(ctx)

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		httpSrv.Shutdown(sh)
	}()
	slog.Info("greengrade läuft", "adresse", cfg.ListenAddr, "origin", cfg.AppOrigin, "mail", cfg.MailProvider)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
