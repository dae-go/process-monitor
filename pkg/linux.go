//go:build linux
// +build linux

package pm

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"
)

// linuxMonitor implements ProcessMonitor for Linux systems
type linuxMonitor struct {
	pageSize      int64
	cpuTicks      int64
	lastCPUStats  map[int]cpuStats
	lastTotalCPU  uint64
	lastCheckTime time.Time
}

type cpuStats struct {
	utime uint64
	stime uint64
	total uint64
}

// newPlatformMonitor creates a new Linux-specific process monitor
func newPlatformMonitor() (ProcessMonitor, error) {
	pageSize := int64(os.Getpagesize())
	cpuTicks := int64(100) // Default Hz value, could read from sysconf

	return &linuxMonitor{
		pageSize:     pageSize,
		cpuTicks:     cpuTicks,
		lastCPUStats: make(map[int]cpuStats),
	}, nil
}

// GetProcesses returns all running processes
func (m *linuxMonitor) GetProcesses() ([]Process, error) {
	entries, err := ioutil.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	processes := make([]Process, 0)
	currentCPUStats := make(map[int]cpuStats)
	totalCPU := m.getTotalCPU()
	currentTime := time.Now()
	timeDelta := currentTime.Sub(m.lastCheckTime).Seconds()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // Not a PID directory
		}

		proc, cpuStat, err := m.readProcessInfo(pid)
		if err != nil {
			continue // Process might have terminated
		}

		// Calculate CPU usage
		if lastStat, ok := m.lastCPUStats[pid]; ok && timeDelta > 0 {
			cpuDelta := float64(cpuStat.total - lastStat.total)
			totalDelta := float64(totalCPU - m.lastTotalCPU)
			if totalDelta > 0 {
				proc.CPUPercent = (cpuDelta / totalDelta) * 100.0 * float64(m.getNumCPU())
			}
		}

		currentCPUStats[pid] = cpuStat
		processes = append(processes, *proc)
	}

	// Update last stats for next calculation
	m.lastCPUStats = currentCPUStats
	m.lastTotalCPU = totalCPU
	m.lastCheckTime = currentTime

	return processes, nil
}

// GetProcess returns information about a specific process
func (m *linuxMonitor) GetProcess(pid int) (*Process, error) {
	proc, _, err := m.readProcessInfo(pid)
	return proc, err
}

// GetSystemStats returns system-wide statistics
func (m *linuxMonitor) GetSystemStats() (*SystemStats, error) {
	stats := &SystemStats{}

	// Get process counts by state
	processes, err := m.GetProcesses()
	if err != nil {
		return nil, err
	}

	stats.TotalProcesses = len(processes)
	for _, p := range processes {
		switch p.State {
		case "R":
			stats.RunningProcesses++
		case "S", "D":
			stats.SleepingProcesses++
		case "T":
			stats.StoppedProcesses++
		case "Z":
			stats.ZombieProcesses++
		}
	}

	// Get memory stats
	memInfo, err := m.readMemInfo()
	if err != nil {
		return nil, err
	}

	stats.TotalMemory = memInfo["MemTotal"]
	stats.UsedMemory = stats.TotalMemory - memInfo["MemAvailable"]
	if stats.TotalMemory > 0 {
		stats.MemoryUsagePercent = float64(stats.UsedMemory) / float64(stats.TotalMemory) * 100
	}

	// Get CPU usage
	stats.CPUUsagePercent = m.getSystemCPUUsage()

	return stats, nil
}

// readProcessInfo reads process information from /proc/[pid]/
func (m *linuxMonitor) readProcessInfo(pid int) (*Process, cpuStats, error) {
	proc := &Process{PID: pid}
	stats := cpuStats{}

	// Read stat file
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	statData, err := ioutil.ReadFile(statPath)
	if err != nil {
		return nil, stats, err
	}

	// Parse stat file
	fields := strings.Fields(string(statData))
	if len(fields) < 52 {
		return nil, stats, fmt.Errorf("invalid stat format")
	}

	// Extract process name (field 1, in parentheses)
	nameStart := strings.Index(string(statData), "(")
	nameEnd := strings.LastIndex(string(statData), ")")
	if nameStart != -1 && nameEnd != -1 && nameEnd > nameStart {
		proc.Name = string(statData[nameStart+1 : nameEnd])
	}

	// Find the start of numeric fields after the name
	statFields := strings.Fields(string(statData[nameEnd+2:]))

	// State is the first field after name
	proc.State = statFields[0]

	// PPID is field 3 (index 1 after name)
	proc.PPID, _ = strconv.Atoi(statFields[1])

	// CPU times are fields 13-14 (indices 11-12 after name)
	utime, _ := strconv.ParseUint(statFields[11], 10, 64)
	stime, _ := strconv.ParseUint(statFields[12], 10, 64)
	stats.utime = utime
	stats.stime = stime
	stats.total = utime + stime

	// Virtual memory size is field 22 (index 20 after name)
	// vsize, _ := strconv.ParseUint(statFields[20], 10, 64)

	// RSS is field 23 (index 21 after name) in pages
	rss, _ := strconv.ParseInt(statFields[21], 10, 64)
	proc.Memory = uint64(rss * m.pageSize)

	// Start time is field 21 (index 19 after name)
	starttime, _ := strconv.ParseUint(statFields[19], 10, 64)
	bootTime := m.getBootTime()
	proc.StartTime = time.Unix(int64(bootTime+starttime/uint64(m.cpuTicks)), 0)

	// Read status for additional info
	statusPath := fmt.Sprintf("/proc/%d/status", pid)
	statusData, err := ioutil.ReadFile(statusPath)
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(statusData)))
		for scanner.Scan() {
			line := scanner.Text()
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch fields[0] {
			case "Uid:":
				uid, _ := strconv.Atoi(fields[1])
				if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
					proc.User = u.Username
				} else {
					proc.User = strconv.Itoa(uid)
				}
			}
		}
	}

	// Read cmdline
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	cmdlineData, err := ioutil.ReadFile(cmdlinePath)
	if err == nil {
		proc.Command = strings.ReplaceAll(string(cmdlineData), "\x00", " ")
		proc.Command = strings.TrimSpace(proc.Command)
		if proc.Command == "" {
			proc.Command = fmt.Sprintf("[%s]", proc.Name)
		}
	}

	// Calculate memory percentage
	memInfo, err := m.readMemInfo()
	if err == nil && memInfo["MemTotal"] > 0 {
		proc.MemoryPercent = float64(proc.Memory) / float64(memInfo["MemTotal"]*1024) * 100
	}

	return proc, stats, nil
}

// readMemInfo reads /proc/meminfo and returns a map of values
func (m *linuxMonitor) readMemInfo() (map[string]uint64, error) {
	data, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}

	info := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			key := strings.TrimSuffix(fields[0], ":")
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			info[key] = value
		}
	}

	return info, nil
}

// getTotalCPU reads total CPU jiffies from /proc/stat
func (m *linuxMonitor) getTotalCPU() uint64 {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			var total uint64
			for i := 1; i < len(fields); i++ {
				val, _ := strconv.ParseUint(fields[i], 10, 64)
				total += val
			}
			return total
		}
	}
	return 0
}

// getSystemCPUUsage calculates overall system CPU usage
func (m *linuxMonitor) getSystemCPUUsage() float64 {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				return 0
			}

			user, _ := strconv.ParseUint(fields[1], 10, 64)
			nice, _ := strconv.ParseUint(fields[2], 10, 64)
			system, _ := strconv.ParseUint(fields[3], 10, 64)
			idle, _ := strconv.ParseUint(fields[4], 10, 64)

			total := user + nice + system + idle
			busy := user + nice + system

			if total > 0 {
				return float64(busy) / float64(total) * 100
			}
		}
	}
	return 0
}

// getBootTime reads system boot time from /proc/stat
func (m *linuxMonitor) getBootTime() uint64 {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "btime" {
			bootTime, _ := strconv.ParseUint(fields[1], 10, 64)
			return bootTime
		}
	}
	return 0
}

// getNumCPU returns the number of CPU cores
func (m *linuxMonitor) getNumCPU() int {
	data, err := ioutil.ReadFile("/proc/cpuinfo")
	if err != nil {
		return 1
	}

	count := 0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "processor") {
			count++
		}
	}

	if count == 0 {
		return 1
	}
	return count
}
