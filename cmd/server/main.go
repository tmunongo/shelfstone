package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/tmunongo/shelfstone/internal/auth"
	"github.com/tmunongo/shelfstone/internal/db"
	"github.com/tmunongo/shelfstone/internal/scanner"
	"github.com/tmunongo/shelfstone/internal/server"
)

func main() {
	// Load .env only in local environment
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: failed to load .env file: %v", err)
		}
	}

	cfg := loadConfig()

	// Open database
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// Set up auth
	authService := auth.New(database, cfg.AuthUsername, cfg.AuthPassword)

	// Set up scanner
	sc := scanner.New(database, cfg.AudiobookDataDir)

	// Run initial scan in background
	go func() {
		log.Println("running initial library scan...")
		if err := sc.Scan(context.Background()); err != nil {
			log.Printf("initial scan error: %v", err)
		}
		log.Println("initial scan complete")
	}()

	// Schedule periodic rescans
	go func() {
		ticker := time.NewTicker(cfg.ScanInterval)
		defer ticker.Stop()
		for range ticker.C {
			log.Println("running scheduled library scan...")
			if err := sc.Scan(context.Background()); err != nil {
				log.Printf("scheduled scan error: %v", err)
			}
		}
	}()

	// Build HTTP server
	srv := server.New(server.Config{
		DB:      database,
		Auth:    authService,
		Scanner: sc,
		DataDir: cfg.AudiobookDataDir,
		BaseURL: cfg.BaseURL,
	})

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.AppPort),
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("shelfstone listening on :%s", cfg.AppPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("shutdown complete")
}
