package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"squareguardian/internal/api"
	"squareguardian/internal/config"
	"squareguardian/internal/detector"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("SpaceGuardian starting...")

	cfg := config.Load()

	det := detector.New(
		cfg.FrigateURL,
		cfg.TrackedItems,
		cfg.PollInterval,
		cfg.EventLogPath,
		cfg.MaxStorageGB,
		cfg.BufferGB,
		cfg.SaveIntervalS,
	)
	det.Start()
	defer det.Stop()

	handler := api.New(det)
	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("API server listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("SpaceGuardian stopped")
}
