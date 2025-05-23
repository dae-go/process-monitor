package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	var (
		configPath = flag.String("config", "config.json", "Path to configuration file")
		httpPort   = flag.Int("port", 8090, "HTTP API port")
		daemon     = flag.Bool("daemon", false, "Run as daemon")
	)
	flag.Parse()

	// Create process monitor
	pm, err := NewProcessMonitor(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create process monitor: %v\n", err)
		os.Exit(1)
	}

	// Start HTTP API server
	go startHTTPServer(pm, *httpPort)

	// Start process monitoring
	err = pm.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start process monitor: %v\n", err)
		os.Exit(1)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if *daemon {
		// In daemon mode, wait for signal indefinitely
		<-sigChan
	} else {
		// In interactive mode, show status updates
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					printStatus(pm)
				case <-sigChan:
					return
				}
			}
		}()
		<-sigChan
	}

	fmt.Println("\nShutting down...")
	pm.Stop()
}

// HTTP API handlers

func startHTTPServer(pm *ProcessMonitor, port int) {
	mux := http.NewServeMux()

	// Status endpoints
	mux.HandleFunc("/status", handleStatus(pm))
	mux.HandleFunc("/status/detailed", handleDetailedStatus(pm))
	mux.HandleFunc("/process/", handleProcessInfo(pm))

	// Control endpoints
	mux.HandleFunc("/restart/", handleRestart(pm))
	mux.HandleFunc("/enable/", handleEnable(pm))
	mux.HandleFunc("/disable/", handleDisable(pm))

	// Health check endpoint
	mux.HandleFunc("/health", handleHealth(pm))

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	fmt.Printf("Starting HTTP API server on port %d\n", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("HTTP server error: %v\n", err)
	}
}

func handleStatus(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		status := pm.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

func handleDetailedStatus(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		status := pm.GetDetailedStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

func handleProcessInfo(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract process name from URL path
		processName := r.URL.Path[len("/process/"):]
		if processName == "" {
			http.Error(w, "Process name required", http.StatusBadRequest)
			return
		}

		info, err := pm.GetProcessInfo(processName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	}
}

func handleRestart(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		processName := r.URL.Path[len("/restart/"):]
		if processName == "" {
			http.Error(w, "Process name required", http.StatusBadRequest)
			return
		}

		err := pm.RestartProcess(processName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Restart initiated for process: %s", processName),
		})
	}
}

func handleEnable(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		processName := r.URL.Path[len("/enable/"):]
		if processName == "" {
			http.Error(w, "Process name required", http.StatusBadRequest)
			return
		}

		err := pm.EnableProcess(processName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Process enabled: %s", processName),
		})
	}
}

func handleDisable(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		processName := r.URL.Path[len("/disable/"):]
		if processName == "" {
			http.Error(w, "Process name required", http.StatusBadRequest)
			return
		}

		err := pm.DisableProcess(processName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Process disabled: %s", processName),
		})
	}
}

func handleHealth(pm *ProcessMonitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		status := pm.GetStatus()
		allHealthy := true

		for _, procStatus := range status {
			if procStatus != StatusRunning {
				allHealthy = false
				break
			}
		}

		response := map[string]interface{}{
			"healthy": allHealthy,
			"status":  status,
		}

		w.Header().Set("Content-Type", "application/json")
		if allHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(response)
	}
}

// CLI status display
func printStatus(pm *ProcessMonitor) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("Process Monitor Status - %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("=", 60))

	status := pm.GetDetailedStatus()
	processes, ok := status["processes"].(map[string]interface{})
	if !ok {
		fmt.Println("Error: Unable to get process status")
		return
	}

	fmt.Printf("%-15s %-10s %-8s %-8s %-8s %-15s\n",
		"NAME", "STATUS", "PID", "CPU%", "MEM(MB)", "RESTARTS")
	fmt.Println(strings.Repeat("-", 70))

	for name, procData := range processes {
		proc, ok := procData.(map[string]interface{})
		if !ok {
			continue
		}

		fmt.Printf("%-15s %-10s %-8v %-8.1f %-8v %-15v\n",
			truncate(name, 15),
			truncate(getString(proc, "status"), 10),
			proc["pid"],
			getFloat(proc, "cpu_percent"),
			proc["memory_mb"],
			proc["restart_count"],
		)
	}
	fmt.Println()
}

// Utility functions for CLI display
func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0
}

// Example usage and testing functions

func createSampleConfig() {
	config := Config{
		CheckInterval: "30s",
		LogFile:       "/tmp/process-monitor.log",
		MaxRestarts:   3,
		RestartWindow: "10m",
		Processes: []ProcessConfig{
			{
				Name:          "test-server",
				Command:       []string{"python3", "-m", "http.server", "8000"},
				WorkingDir:    "/tmp",
				MaxCPU:        50.0,
				MaxMemory:     100,
				RestartOnExit: true,
				HealthCheck: &HealthCheck{
					Type:     "http",
					Target:   "http://localhost:8000",
					Interval: "30s",
					Timeout:  "5s",
				},
				Enabled: true,
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Printf("Error creating sample config: %v\n", err)
		return
	}

	err = os.WriteFile("sample-config.json", data, 0644)
	if err != nil {
		fmt.Printf("Error writing sample config: %v\n", err)
		return
	}

	fmt.Println("Sample configuration created: sample-config.json")
}

// Add this to main() if you want to create a sample config
func init() {
	if len(os.Args) > 1 && os.Args[1] == "create-sample-config" {
		createSampleConfig()
		os.Exit(0)
	}
}
