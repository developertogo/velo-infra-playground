package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	portFlag := flag.Int("port", 6820, "Port to listen on (default 6820, standard slurmrestd port)")
	flag.Parse()

	// Initialize thread-safe data store
	store := NewClusterStore()

	// Initialize and start background job state reconciler scheduler
	scheduler := NewScheduler(store)
	scheduler.Start()
	defer scheduler.Stop()

	// Initialize HTTP handlers
	handler := NewSlurmHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Expose OpenAPI specs dynamically as mock endpoints
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"openapi":"3.0.2","info":{"title":"Slurm REST API","version":"0.0.38"},"paths":{}}`))
	})

	serverAddr := fmt.Sprintf(":%d", *portFlag)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	// Channel to listen for interrupts
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		fmt.Printf("[Slurm Simulator] Starting slurmrestd mock daemon on %s...\n", serverAddr)
		fmt.Println("[Slurm Simulator] Press Ctrl+C to stop.")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[Slurm Simulator] Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-shutdownChan
	fmt.Println("\n[Slurm Simulator] Shutting down gracefully...")

	// Graceful shutdown context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("[Slurm Simulator] Server shutdown error: %v\n", err)
	}

	fmt.Println("[Slurm Simulator] Daemon stopped successfully.")
}
