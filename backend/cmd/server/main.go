package main

import (
	"backend/internal/api"
	"backend/pkg/config"
	"backend/pkg/db"

	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// main entry point
func main() {
	// init config from env
	cfg := config.Load()

	// configure structured logging
	logger := log.New(os.Stdout, "SERVER: ", log.Ldate|log.Ltime|log.Lshortfile)
	logger.Println("starting backend server")

	// connect to db
	dbSession, err := db.InitScyllaDB(cfg.ScyllaHost, logger)
	if err != nil {
		logger.Fatalf("database connection failed: %v", err)
	}
	defer dbSession.Close()
	logger.Println("database connection established")

	// execute database schema migrations
	if err := db.RunMigrations(dbSession, logger); err != nil {
		logger.Fatalf("database migrations failed: %v", err)
	}
	logger.Println("database migrations applied")

	// initialize http router with all api endpoints
	router := api.SetupRouter(dbSession, cfg, logger)

	// configure http server with timeouts
	server := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		ReadTimeout:  15 * time.Second,  // prevent slowloris attacks (opening http connection but sending data slowly to exhaust connection pool)
		WriteTimeout: 30 * time.Second,  // allow time for processing
		IdleTimeout:  120 * time.Second, // close idle connections
	}

	// start server in separate goroutine
	go func() {
		logger.Printf("server listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	// setup graceful(controlled) shutdown handling
	shutdown := make(chan os.Signal, 1)                      // create channel
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM) // relay os signals to channel
	<-shutdown                                               // wait for termination signal

	logger.Println("initiating graceful shutdown...")

	// create context with timeout for shutdown operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("server shutdown failed: %v", err)
	}

	logger.Println("server exited gracefully")
}
