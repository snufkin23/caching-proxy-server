package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/snufkin23/caching-proxy-server/internal/cache"
	"github.com/snufkin23/caching-proxy-server/internal/config"
	"github.com/snufkin23/caching-proxy-server/internal/middleware"
	"github.com/snufkin23/caching-proxy-server/internal/proxy"
)

func main() {
	// Create structured JSON logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting cache proxy server")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("configuration loaded",
		slog.String("port", cfg.Port),
		slog.Duration("cache_ttl", cfg.CacheTTL),
		slog.Int64("max_cache_size", cfg.MaxCacheSize),
	)

	// CREATE CACHE
	cacheStore := cache.NewMemoryCache(cfg.CacheTTL, cfg.MaxCacheSize)

	slog.Info("cache initialized",
		slog.Duration("ttl", cfg.CacheTTL),
		slog.Int64("max_size_bytes", cfg.MaxCacheSize),
	)

	// CREATE HANDLER
	proxyHandler := proxy.NewProxyHandler(cacheStore, logger)

	slog.Info("proxy handler created")

	// APPLY MIDDLEWARE
	stack := middleware.Chain(
		proxyHandler,
		middleware.Logging(logger),
		middleware.Recovery(logger),
	)

	slog.Info("middleware applied")

	// CONFIGURE HTTP SERVER
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      stack,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	// START SERVER
	serverErrors := make(chan error, 1)

	go func() {
		slog.Info("server starting", slog.String("port", cfg.Port))
		serverErrors <- srv.ListenAndServe()
	}()

	// SETUP GRACEFUL SHUTDOWN
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// WAIT FOR SHUTDOWN OR ERROR
	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}

	case sig := <-shutdown:
		slog.Info("shutdown signal received",
			slog.String("signal", sig.String()),
		)

		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("graceful shutdown failed",
				slog.String("error", err.Error()),
			)

			if err := srv.Close(); err != nil {
				slog.Error("force close failed",
					slog.String("error", err.Error()),
				)
			}
		}

		slog.Info("server shutdown complete")
	}
}
