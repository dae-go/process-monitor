package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Config represents the process monitor configuration
type Config struct {
	Processes     []ProcessConfig `json:"processes"`
	CheckInterval string          `json:"check_interval"` // e.g., "30s"
	LogFile       string          `json:"log_file"`
	MaxRestarts   int             `json:"max_restarts"`
	RestartWindow string          `json:"restart_window"` // e.g., "1h"
}

// ProcessConfig defines a process to monitor
type ProcessConfig struct {
	Name          string            `json:"name"`
	Command       []string          `json:"command"`
	WorkingDir    string            `json:"working_dir"`
	Environment   map[string]string `json:"environment"`
	MaxCPU        float64           `json:"max_cpu"`    // CPU percentage threshold
	MaxMemory     int64             `json:"max_memory"` // Memory in MB
	RestartOnExit bool              `json:"restart_on_exit"`
	HealthCheck   *HealthCheck      `json:"health_check"`
	Enabled       bool              `json:"enabled"`
}

// HealthCheck defines health check parameters
type HealthCheck struct {
	Type     string `json:"type"`     // "http", "tcp", "command"
	Target   string `json:"target"`   // URL, address, or command
	Interval string `json:"interval"` // e.g., "60s"
	Timeout  string `json:"timeout"`  // e.g., "10s"
}

// ProcessInfo holds runtime information about a monitored process
type ProcessInfo struct {
	Config          ProcessConfig
	PID             int
	Status          ProcessStatus
	CPUPercent      float64
	MemoryMB        int64
	StartTime       time.Time
	LastHealthCheck time.Time
	RestartCount    int
	RestartHistory  []time.Time
	LastError       error
	mutex           sync.RWMutex
	cmd             *exec.Cmd
	ctx             context.Context
	cancel          context.CancelFunc
}

type ProcessStatus int

const (
	StatusStopped ProcessStatus = iota
	StatusStarting
	StatusRunning
	StatusUnhealthy
	StatusRestarting
	StatusFailed
)

func (s ProcessStatus) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusUnhealthy:
		return "unhealthy"
	case StatusRestarting:
		return "restarting"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ProcessMonitor is the main monitoring service
type ProcessMonitor struct {
	config    Config
	processes map[string]*ProcessInfo
	logger    *log.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	mutex     sync.RWMutex
}

// NewProcessMonitor creates a new process monitor instance
func NewProcessMonitor(configPath string) (*ProcessMonitor, error) {
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	logger := setupLogger(config.LogFile)
	ctx, cancel := context.WithCancel(context.Background())

	pm := &ProcessMonitor{
		config:    config,
		processes: make(map[string]*ProcessInfo),
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Initialize process info structures
	for _, procConfig := range config.Processes {
		if procConfig.Enabled {
			pm.processes[procConfig.Name] = &ProcessInfo{
				Config:         procConfig,
				Status:         StatusStopped,
				RestartHistory: make([]time.Time, 0),
			}
		}
	}

	return pm, nil
}

// Start begins monitoring all configured processes
func (pm *ProcessMonitor) Start() error {
	pm.logger.Println("Starting process monitor...")

	// Start all enabled processes
	for name, proc := range pm.processes {
		pm.wg.Add(1)
		go pm.monitorProcess(name, proc)
	}

	// Start the main monitoring loop
	pm.wg.Add(1)
	go pm.monitoringLoop()

	pm.logger.Printf("Process monitor started, monitoring %d processes", len(pm.processes))
	return nil
}

// Stop gracefully shuts down the process monitor
func (pm *ProcessMonitor) Stop() {
	pm.logger.Println("Stopping process monitor...")
	pm.cancel()
	pm.wg.Wait()

	// Stop all managed processes
	for name, proc := range pm.processes {
		if proc.Status == StatusRunning {
			pm.logger.Printf("Stopping process: %s", name)
			pm.stopProcess(proc)
		}
	}

	pm.logger.Println("Process monitor stopped")
}

// GetStatus returns the current status of all monitored processes
func (pm *ProcessMonitor) GetStatus() map[string]ProcessStatus {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	status := make(map[string]ProcessStatus)
	for name, proc := range pm.processes {
		proc.mutex.RLock()
		status[name] = proc.Status
		proc.mutex.RUnlock()
	}
	return status
}

// GetProcessInfo returns detailed information about a specific process
func (pm *ProcessMonitor) GetProcessInfo(name string) (*ProcessInfo, error) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	proc, exists := pm.processes[name]
	if !exists {
		return nil, fmt.Errorf("process %s not found", name)
	}

	// Return a copy to avoid race conditions
	proc.mutex.RLock()
	defer proc.mutex.RUnlock()

	infoCopy := *proc
	return &infoCopy, nil
}

// loadConfig loads configuration from a JSON file
func loadConfig(configPath string) (Config, error) {
	var config Config

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	// Set defaults
	if config.CheckInterval == "" {
		config.CheckInterval = "30s"
	}
	if config.MaxRestarts == 0 {
		config.MaxRestarts = 5
	}
	if config.RestartWindow == "" {
		config.RestartWindow = "1h"
	}

	return config, nil
}

// setupLogger creates a logger instance
func setupLogger(logFile string) *log.Logger {
	if logFile == "" {
		return log.New(os.Stdout, "[ProcessMonitor] ", log.LstdFlags)
	}

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Printf("Failed to open log file %s, using stdout: %v", logFile, err)
		return log.New(os.Stdout, "[ProcessMonitor] ", log.LstdFlags)
	}

	return log.New(file, "[ProcessMonitor] ", log.LstdFlags)
}
