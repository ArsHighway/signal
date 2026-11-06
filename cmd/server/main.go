package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"signalcare/db"
	"signalcare/internal/server"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := env("DATABASE_URL", "postgres://signal:signal_local_only@127.0.0.1:55439/signalcare?sslmode=disable")
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	goose.SetBaseFS(db.FS)
	if err = goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err = goose.UpContext(ctx, sqldb, "migrations"); err != nil {
		sqldb.Close()
		return err
	}
	sqldb.Close()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		return err
	}
	s := server.New(&server.Store{Pool: pool}, server.Config{
		UserPassword: env("DEMO_USER_PASSWORD", "signal-user-demo"), DispatcherPassword: env("DEMO_DISPATCHER_PASSWORD", "signal-dispatcher-demo"),
		WebDir: env("WEB_DIR", "dist"), SecureCookies: os.Getenv("COOKIE_SECURE") == "true",
	})
	go s.Worker(ctx)
	h := &http.Server{Addr: env("APP_ADDR", "127.0.0.1:8087"), Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 75 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = h.Shutdown(shutdown)
	}()
	slog.Info("Сигнал · учебный прототип", "url", "http://"+h.Addr, "mode", "training")
	err = h.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
