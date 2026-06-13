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
	portFlag := flag.Int("port", 8000, "Port to listen on (default 8000)")
	flag.Parse()

	// Initialize database
	store := NewHardwareStore()

	// Initialize handler and routes
	handler := NewRedfishHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Mock endpoint for Redfish metadata
	mux.HandleFunc("GET /redfish/v1/$metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx" Version="4.0"></edmx:Edmx>`))
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
		fmt.Printf("[Redfish Simulator] Starting Redfish mock daemon on %s...\n", serverAddr)
		fmt.Println("[Redfish Simulator] Available systems: 'system-1' (DGX H100), 'system-2' (DGX A100)")
		fmt.Println("[Redfish Simulator] Press Ctrl+C to stop.")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[Redfish Simulator] Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-shutdownChan
	fmt.Println("\n[Redfish Simulator] Shutting down gracefully...")

	// Graceful shutdown context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("[Redfish Simulator] Server shutdown error: %v\n", err)
	}

	fmt.Println("[Redfish Simulator] Daemon stopped successfully.")
}
