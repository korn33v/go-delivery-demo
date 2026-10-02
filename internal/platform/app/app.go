package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func Context() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
func OpenDB(ctx context.Context, url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	maxConnections := 10
	if value := os.Getenv("DB_MAX_OPEN_CONNS"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 {
			_ = db.Close()
			return nil, errors.New("DB_MAX_OPEN_CONNS must be positive")
		}
		maxConnections = n
	}
	db.SetMaxOpenConns(maxConnections)
	idleConnections := 5
	if maxConnections < idleConnections {
		idleConnections = maxConnections
	}
	db.SetMaxIdleConns(idleConnections)
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = db.PingContext(check); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
func Migrate(url, path, direction string) error {
	m, err := migrate.New("file://"+path, url)
	if err != nil {
		return err
	}
	defer m.Close()
	if direction == "down" {
		err = m.Down()
	} else {
		err = m.Up()
	}
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

// Workers — два простых последовательных цикла: outbox и consumer. Без pool/channel magic.
func Serve(ctx context.Context, server *http.Server, logger *slog.Logger, workers ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	for _, work := range workers {
		wg.Add(1)
		go func(run func(context.Context) error) {
			defer wg.Done()
			if err := run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("worker stopped", "error", err.Error())
				cancel()
			}
		}(work)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdown, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("HTTP server started", "address", server.Addr)
	err := server.ListenAndServe()
	cancel()
	wg.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
