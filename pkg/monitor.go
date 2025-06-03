package pm

import (
	"sort"
	"time"
)

// Process represents a system process
type Process struct {
	PID           int
	PPID          int
	Name          string
	State         string
	User          string
	Memory        uint64 // in bytes
	MemoryPercent float64
	CPUPercent    float64
	StartTime     time.Time
	Command       string
}

// SystemStats represents overall system statistics
type SystemStats struct {
	TotalProcesses     int
	RunningProcesses   int
	SleepingProcesses  int
	StoppedProcesses   int
	ZombieProcesses    int
	TotalMemory        uint64
	UsedMemory         uint64
	MemoryUsagePercent float64
	CPUUsagePercent    float64
}

// ProcessMonitor interface defines the contract for OS-specific implementations
type ProcessMonitor interface {
	GetProcesses() ([]Process, error)
	GetProcess(pid int) (*Process, error)
	GetSystemStats() (*SystemStats, error)
}

// Monitor is the main process monitor struct
type Monitor struct {
	impl ProcessMonitor
}

// New creates a new process monitor for the current OS
func New() (*Monitor, error) {
	impl, err := newPlatformMonitor()
	if err != nil {
		return nil, err
	}
	return &Monitor{impl: impl}, nil
}

// GetProcesses returns a list of all processes
func (m *Monitor) GetProcesses() ([]Process, error) {
	return m.impl.GetProcesses()
}

// GetProcess returns information about a specific process
func (m *Monitor) GetProcess(pid int) (*Process, error) {
	return m.impl.GetProcess(pid)
}

// GetSystemStats returns system-wide statistics
func (m *Monitor) GetSystemStats() (*SystemStats, error) {
	return m.impl.GetSystemStats()
}

// SortProcesses sorts processes using the provided comparison function
func SortProcesses(processes []Process, less func(i, j int) bool) {
	sort.Slice(processes, less)
}

// FilterProcesses filters processes based on a predicate function
func FilterProcesses(processes []Process, predicate func(p Process) bool) []Process {
	filtered := make([]Process, 0)
	for _, p := range processes {
		if predicate(p) {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// GroupProcessesByState groups processes by their state
func GroupProcessesByState(processes []Process) map[string][]Process {
	groups := make(map[string][]Process)
	for _, p := range processes {
		groups[p.State] = append(groups[p.State], p)
	}
	return groups
}

// CalculateCPUUsage calculates the CPU usage percentage between two process snapshots
func CalculateCPUUsage(prev, curr *Process, duration time.Duration) float64 {
	if prev == nil || curr == nil || duration == 0 {
		return 0
	}
	// This is a placeholder - actual implementation would depend on OS-specific metrics
	// Linux would use jiffies, Windows would use different counters
	return 0
}
