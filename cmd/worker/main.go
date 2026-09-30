package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	infraCfg "github.com/BoSuY0/tetracore-worker/internal/infrastructure/config"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/mysql"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/observability"
	infraRedis "github.com/BoSuY0/tetracore-worker/internal/infrastructure/redis"
	"github.com/BoSuY0/tetracore-worker/internal/transport/telegram"

	"github.com/BoSuY0/tetracore-worker/internal/actions"
	"github.com/BoSuY0/tetracore-worker/internal/worker"
)

func main() {
	// ----- CLI flags -----
	workerID := flag.String("worker-id", "", "Unique worker identifier (default: worker-<uuid>)")
	configPath := flag.String("config", "config/worker.yaml", "Path to the configuration file")
	flag.Parse()

	if *workerID == "" {
		*workerID = "worker-" + uuid.New().String()
	}

	// ----- Configuration -----
	cfg, err := infraCfg.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Якщо worker_id задано через конфіг, але CLI-прапорець має пріоритет.
	if cfg.Worker.ID == "" {
		cfg.Worker.ID = *workerID
	}

	// ----- Logger -----
	logger := observability.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
	log.Logger = logger
	zerolog.DefaultContextLogger = &logger

	logger.Info().
		Str("worker_id", cfg.Worker.ID).
		Str("config", *configPath).
		Msg("starting tetracore-worker")

	// ----- Redis -----
	redisClient, err := infraRedis.NewClient(cfg.Redis)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisClient.Close()
	logger.Info().Msg("redis connected")

	// ----- MySQL -----
	db, err := mysql.NewClient(cfg.MySQL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to mysql")
	}
	defer mysql.Close(db)
	logger.Info().Msg("mysql connected")

	// ----- Telegram HTTP Client -----
	telegramClient := telegram.NewClient(telegram.Config{
		BotToken: cfg.Telegram.BotToken,
		APIURL:   cfg.Telegram.APIURL,
		Timeout:  cfg.Telegram.Timeout,
	})
	logger.Info().Msg("telegram client initialized")

	// ----- Metrics (optional) -----
	var metrics *observability.Metrics
	if cfg.Metrics.Enabled {
		metrics = observability.NewMetrics("tetracore_worker")
		go observability.ServeMetrics(cfg.Metrics.Port, logger)
		logger.Info().Int("port", cfg.Metrics.Port).Msg("metrics server started")
	}

	// ----- Action Registry -----
	registry := actions.NewRegistry()
	actions.RegisterAll(registry, redisClient, db, telegramClient, logger)
	logger.Info().Int("count", registry.Len()).Msg("actions registered")

	// ----- Worker -----
	w, err := worker.New(cfg, registry, redisClient, logger, metrics)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create worker")
	}

	// ----- Graceful Shutdown -----
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info().Str("signal", sig.String()).Msg("shutdown signal received")
		cancel()
	}()

	// ----- Run (blocks until context cancelled) -----
	if err := w.Run(ctx); err != nil {
		logger.Error().Err(err).Msg("worker exited with error")
		os.Exit(1)
	}

	logger.Info().Msg("worker stopped gracefully")
}
