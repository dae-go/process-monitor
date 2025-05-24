package pkg

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// checkResourceUsage monitors CPU and memory usage of all processes
func (pm *ProcessMonitor) checkResourceUsage() {
	pm.mutex.RLock()
	processes := make([]*ProcessInfo, 0, len(pm.processes))
	for _, proc := range processes {
		processes = append(processes, proc)
	}
	pm.mutex.RUnlock()

	for _, proc := range processes {
		go pm.updateProcessStats(proc)
	}
}

// updateProcessStats updates CPU and memory statistics for a process
func (pm *ProcessMonitor) updateProcessStats(proc *ProcessInfo) {
	proc.mutex.Lock()
	defer proc.mutex.Unlock()

	if proc.Status != StatusRunning || proc.PID == 0 {
		return
	}

	// Get CPU and memory usage
	cpuPercent, memoryMB, err := pm.getProcessStats(proc.PID)
	if err != nil {
		pm.logger.Printf("Failed to get stats for process %s (PID %d): %v",
			proc.Config.Name, proc.PID, err)
		return
	}

	proc.CPUPercent = cpuPercent
	proc.MemoryMB = memoryMB

	// Check if resource limits are exceeded
	if proc.Config.MaxCPU > 0 && cpuPercent > proc.Config.MaxCPU {
		pm.logger.Printf("Process %s exceeded CPU limit (%.2f%% > %.2f%%)",
			proc.Config.Name, cpuPercent, proc.Config.MaxCPU)

		if pm.shouldRestart(proc) {
			pm.logger.Printf("Restarting process %s due to high CPU usage", proc.Config.Name)
			go pm.stopProcess(proc) // Restart will happen in ensureProcessRunning
		}
	}

	if proc.Config.MaxMemory > 0 && memoryMB > proc.Config.MaxMemory {
		pm.logger.Printf("Process %s exceeded memory limit (%d MB > %d MB)",
			proc.Config.Name, memoryMB, proc.Config.MaxMemory)

		if pm.shouldRestart(proc) {
			pm.logger.Printf("Restarting process %s due to high memory usage", proc.Config.Name)
			go pm.stopProcess(proc) // Restart will happen in ensureProcessRunning
		}
	}
}

// getProcessStats retrieves CPU and memory usage for a process
func (pm *ProcessMonitor) getProcessStats(pid int) (cpuPercent float64, memoryMB int64, err error) {
	// This is a Linux-specific implementation using /proc filesystem
	// For cross-platform support, you'd want to use a library like gopsutil

	// Read /proc/[pid]/stat for CPU info
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := ioutil.ReadFile(statPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read stat file: %w", err)
	}

	// Parse stat file (space-separated values)
	statFields := strings.Fields(string(statData))
	if len(statFields) < 24 {
		return 0, 0, fmt.Errorf("invalid stat file format")
	}

	// Get memory usage from /proc/[pid]/status
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	statusFile, err := os.Open(statusPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open status file: %w", err)
	}
	defer statusFile.Close()

	scanner := bufio.NewScanner(statusFile)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			// Extract memory usage in kB
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memKB, err := strconv.ParseInt(fields[1], 10, 64)
				if err == nil {
					memoryMB = memKB / 1024
				}
			}
			break
		}
	}

	// For CPU calculation, you'd need to track this over time
	// This is a simplified version - in practice, you'd need to:
	// 1. Read utime and stime from stat file
	// 2. Read system CPU time from /proc/stat
	// 3. Calculate percentage based on time delta
	// For now, we'll use a placeholder that calls ps command

	cpuPercent, err = pm.getCPUUsageWithPS(pid)
	if err != nil {
		cpuPercent = 0 // Don't fail the whole function for CPU calculation
	}

	return cpuPercent, memoryMB, nil
}

// getCPUUsageWithPS gets CPU usage using ps command (simpler but less efficient)
func (pm *ProcessMonitor) getCPUUsageWithPS(pid int) (float64, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pcpu", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	cpuStr := strings.TrimSpace(string(output))
	return strconv.ParseFloat(cpuStr, 64)
}

// isProcessAlive checks if a process is still running
func (pm *ProcessMonitor) isProcessAlive(pid int) bool {
	// Check if /proc/[pid] exists (Linux-specific)
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// Health check implementations

// httpHealthCheck performs an HTTP health check
func (pm *ProcessMonitor) httpHealthCheck(ctx context.Context, url string) (bool, error) {
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Consider 2xx and 3xx status codes as healthy
	return resp.StatusCode >= 200 && resp.StatusCode < 400, nil
}

// tcpHealthCheck performs a TCP connection health check
func (pm *ProcessMonitor) tcpHealthCheck(ctx context.Context, address string) (bool, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	return true, nil
}

// commandHealthCheck performs a command-based health check
func (pm *ProcessMonitor) commandHealthCheck(ctx context.Context, command string) (bool, error) {
	// Split command into parts
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false, fmt.Errorf("empty command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	err := cmd.Run()

	// Command is healthy if it exits with status 0
	return err == nil, err
}

// Additional utility functions

// RestartProcess manually restarts a specific process
func (pm *ProcessMonitor) RestartProcess(name string) error {
	pm.mutex.RLock()
	proc, exists := pm.processes[name]
	pm.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	pm.logger.Printf("Manual restart requested for process: %s", name)
	pm.stopProcess(proc)

	// The monitoring loop will restart it automatically
	return nil
}

// EnableProcess enables monitoring for a process
func (pm *ProcessMonitor) EnableProcess(name string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	proc, exists := pm.processes[name]
	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	proc.Config.Enabled = true
	pm.logger.Printf("Process %s enabled", name)
	return nil
}

// DisableProcess disables monitoring for a process
func (pm *ProcessMonitor) DisableProcess(name string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	proc, exists := pm.processes[name]
	if !exists {
		return fmt.Errorf("process %s not found", name)
	}

	proc.Config.Enabled = false
	pm.stopProcess(proc)
	pm.logger.Printf("Process %s disabled", name)
	return nil
}

// GetDetailedStatus returns detailed status information
func (pm *ProcessMonitor) GetDetailedStatus() map[string]interface{} {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	status := make(map[string]interface{})
	processes := make(map[string]interface{})

	for name, proc := range pm.processes {
		proc.mutex.RLock()
		processes[name] = map[string]interface{}{
			"status":            proc.Status.String(),
			"pid":               proc.PID,
			"cpu_percent":       proc.CPUPercent,
			"memory_mb":         proc.MemoryMB,
			"start_time":        proc.StartTime,
			"restart_count":     proc.RestartCount,
			"last_health_check": proc.LastHealthCheck,
			"enabled":           proc.Config.Enabled,
			"last_error": func() string {
				if proc.LastError != nil {
					return proc.LastError.Error()
				}
				return ""
			}(),
		}
		proc.mutex.RUnlock()
	}

	status["processes"] = processes
	status["monitor_uptime"] = time.Since(time.Now()) // This would be tracked separately in practice

	return status
}
