package main

import (
	"context"
	"fmt"
	"godelivery/contracts/events"
	"godelivery/internal/platform/app"
	"godelivery/internal/platform/bus"
	"godelivery/internal/platform/config"
	"godelivery/internal/platform/telemetry"
	"godelivery/internal/platform/web"
	"godelivery/services/fulfillment/internal/delivery"
	"godelivery/services/fulfillment/internal/repository"
	"godelivery/services/fulfillment/internal/usecase"
	"net/http"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := app.Context()
	defer cancel()
	dbURL := config.Env("DB_URL", "")
	if dbURL == "" {
		return fmt.Errorf("DB_URL is required")
	}
	migrations := config.Env("MIGRATIONS_PATH", "services/fulfillment/migrations")
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		direction := "up"
		if len(os.Args) > 2 {
			direction = os.Args[2]
		}
		return app.Migrate(dbURL, migrations, direction)
	}
	// Composition Root: здесь явно строится весь граф зависимостей.
	// configuration -> telemetry -> DB -> adapters -> use case -> delivery -> server.
	t, err := telemetry.New(ctx, "fulfillment-service", config.Env("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel-collector:4318"))
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = t.Shutdown(closeCtx)
	}()
	// Schema version — часть приложения. Go-migrate защищает concurrent запуск lock'ом.
	if err = app.Migrate(dbURL, migrations, "up"); err != nil {
		return err
	}
	db, err := app.OpenDB(ctx, dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	store := repository.NewPostgres(db, t)
	producer := bus.NewProducer(config.Env("KAFKA_BROKER", "kafka:9092"), t)
	defer producer.Close()
	consumer := bus.NewConsumer(config.Env("KAFKA_BROKER", "kafka:9092"), events.PaidTopic, "fulfillment-paid-v1", t)
	defer consumer.Close()
	service := usecase.New(store, t)
	handler := delivery.New(service)
	mux := http.NewServeMux()
	handler.Routes(mux)
	web.Technical(mux, db.PingContext, config.Env("OPENAPI_PATH", "services/fulfillment/api/openapi.yaml"), config.Env("SWAGGER_PATH", "deploy/swagger"))
	server := &http.Server{Addr: config.Env("HTTP_ADDR", ":8080"), Handler: web.Instrument(mux, t), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	return app.Serve(ctx, server, t.Logger,
		func(ctx context.Context) error { return store.RunOutbox(ctx, producer) },
		func(ctx context.Context) error { return consumer.Run(ctx, delivery.PaidConsumer(service)) })

}
