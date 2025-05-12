package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/CiaranMcAleer/roxy/internal/config"
	"github.com/CiaranMcAleer/roxy/internal/proxy"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize proxy server
	server, err := proxy.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy server: %v", err)
	}

	// Start server in a goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	fmt.Printf("Roxy proxy server started on %s\n", cfg.ListenAddr)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down gracefully...")
	if err := server.Shutdown(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}
}
