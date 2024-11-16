package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// parseArguments handles command-line argument parsing and validation.
func parseArguments() (string, *Config, error) {
	var simulationID string
	var method string
	cfg := &Config{}

	// TODO make sim positional argument (mandatory)
	flag.StringVar(&simulationID, "sim", "", "")
	flag.BoolVar(&cfg.UseTLS, "tls", false, "")
	flag.StringVar(&cfg.ClientRequestMethod, "method", "GET", "")
	flag.Usage = displayHelp
	flag.Parse()

	if simulationID == "" {
		return "", nil, fmt.Errorf("simulationID cannot be empty")
	}

	if !isValidMethod(method) {
		return "", nil, fmt.Errorf("invalid method: %s. Allowed methods are GET, POST, DELETE, HEAD", method)
	}

	return simulationID, cfg, nil
}

// isValidMethod checks if the provided method is one of the allowed HTTP methods.
func isValidMethod(method string) bool {
	allowedMethods := map[string]bool{
		"":       true,
		"GET":    true,
		"POST":   true,
		"DELETE": true,
		"HEAD":   true,
	}
	return allowedMethods[method]
}

// loadConfiguration loads the appropriate configuration based on the parsed arguments.
func loadConfiguration(simulationID string, argsCfg *Config) *Config {
	cfg, exists := simulations.get(simulationID)
	if !exists {
		fmt.Printf("Invalid simulation ID: %s\n\n", simulationID)
		displaySimulationsList()
		os.Exit(1)
	}
	if cfg.ServerType != ServerTypeHTTP && argsCfg.UseTLS {
		fmt.Printf("HTTPS is not supported by the selected scenario: %s\n\n", simulationID)
		os.Exit(1)
	}
	if argsCfg.ClientRequestMethod != "" {
		cfg.ClientRequestMethod = argsCfg.ClientRequestMethod
	}
	cfg.UseTLS = argsCfg.UseTLS
	if cfg.UseHTTP2 {
		cfg.UseTLS = true // http2 forces https
	}

	// Set URLs based on HTTPS mode
	if cfg.UseTLS {
		cfg.ServerAddress = srvIP + ":" + portTLS
		cfg.ClientRequestURL = "https://" + cfg.ServerAddress
	} else {
		cfg.ServerAddress = srvIP + ":" + port
		cfg.ClientRequestURL = "http://" + cfg.ServerAddress
	}

	return cfg
}

func displaySimulationsList() {
	fmt.Println("Available Simulations:")
	for _, cfg := range simulations {
		fmt.Printf("  %s: %s %s\n", cfg.ID, cfg.Description, httpsInfo(&cfg))
	}
}

// displayHelp prints the help message with available options and scenarios.
func displayHelp() {
	fmt.Println("Usage: go run . [OPTIONS]")
	fmt.Println("\nOptions:")
	fmt.Println("  -sim        (mandatory) Simulation scenario ID (e.g., '01')")
	fmt.Println("  -tls        (optional)  Ad hoc change to HTTPS for the selected simulation (not supported by all simulations)")
	fmt.Println("  -method     (optional)  Ad hoc change of HTTP request method (GET, POST, DELETE, HEAD)")
	fmt.Println("  -h          Show help and exit")
	fmt.Println()

	displaySimulationsList()

	fmt.Println("\nExample:")
	fmt.Println("  go run main.go -sim 01")
	fmt.Println("")
}

func httpsInfo(cfg *Config) string {
	if cfg.ServerType == ServerTypeHTTP {
		return "(can HTTPS)"
	}
	return "(no HTTPS)"
}

// runSimulation sets up and executes the simulation by starting the server and client.
func runSimulation(cfg *Config) {
	var wg sync.WaitGroup

	go startServer(cfg)

	wg.Add(1)
	go func() {
		defer wg.Done()
		// wait a bit for the server to be ready
		time.Sleep(1 * time.Second)
		startClient(cfg)
	}()

	// Create a channel to listen for interrupt signals
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM) // press Ctrl+C to stop

	// Create a done channel to signal when the client finishes
	doneChan := make(chan struct{})

	// Start a goroutine to wait for the WaitGroup to finish
	go func() {
		wg.Wait()
		close(doneChan)
	}()

	select {
	case <-stopChan: // Handle Ctrl+C or termination signal
		fmt.Printf("\nprogram: shutdown signal received. Exiting...\n")
	case <-doneChan: // Goroutines finished normally
		fmt.Printf("\nprogram: all tasks completed successfully. Exiting...\n")
	}

	fmt.Println("Simulation stopped.")
}

func main() {
	simulationID, cfg, err := parseArguments()
	if err != nil {
		displayHelp()
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		return
	}
	cfg = loadConfiguration(simulationID, cfg)
	cfg.print()
	runSimulation(cfg)
}
