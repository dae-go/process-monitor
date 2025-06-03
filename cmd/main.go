package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	pm "github.com/dae-go/process-monitor/v3/pkg"
)

func main() {
	// Command line flags
	interval := flag.Duration("interval", 2*time.Second, "Update interval")
	top := flag.Int("top", 20, "Number of top processes to show")
	sortBy := flag.String("sort", "cpu", "Sort by: cpu, memory, pid")
	flag.Parse()

	// Create process monitor
	monitor, err := pm.New()
	if err != nil {
		log.Fatal("Failed to create process monitor:", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create ticker for periodic updates
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	// Clear screen function
	clearScreen := func() {
		fmt.Print("\033[H\033[2J")
	}

	// Main monitoring loop
	fmt.Println("Process Monitor Started. Press Ctrl+C to exit.")
	fmt.Printf("Update interval: %v, Showing top %d processes\n\n", *interval, *top)

	for {
		select {
		case <-ticker.C:
			// Get all processes
			processes, err := monitor.GetProcesses()
			if err != nil {
				log.Printf("Error getting processes: %v\n", err)
				continue
			}

			// Sort processes
			var sortFunc func(i, j int) bool
			switch *sortBy {
			case "memory":
				sortFunc = func(i, j int) bool {
					return processes[i].Memory > processes[j].Memory
				}
			case "pid":
				sortFunc = func(i, j int) bool {
					return processes[i].PID < processes[j].PID
				}
			default: // cpu
				sortFunc = func(i, j int) bool {
					return processes[i].CPUPercent > processes[j].CPUPercent
				}
			}
			pm.SortProcesses(processes, sortFunc)

			// Clear screen and display header
			clearScreen()
			displayHeader()

			// Display processes
			displayProcesses(processes, *top)

			// Display system stats
			stats, err := monitor.GetSystemStats()
			if err == nil {
				displaySystemStats(stats)
			}

		case <-sigChan:
			fmt.Println("\nShutting down process monitor...")
			return
		}
	}
}

func displayHeader() {
	fmt.Printf("Process Monitor - %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(string(make([]byte, 80, 80)))
}

func displayProcesses(processes []pm.Process, limit int) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "PID\tNAME\tCPU%\tMEM%\tMEM(MB)\tSTATE\tUSER")
	fmt.Fprintln(w, "---\t----\t----\t----\t-------\t-----\t----")

	count := limit
	if len(processes) < limit {
		count = len(processes)
	}

	for i := 0; i < count; i++ {
		p := processes[i]
		fmt.Fprintf(w, "%d\t%s\t%.1f\t%.1f\t%.1f\t%s\t%s\n",
			p.PID,
			truncateString(p.Name, 20),
			p.CPUPercent,
			p.MemoryPercent,
			float64(p.Memory)/1024/1024,
			p.State,
			p.User,
		)
	}
	w.Flush()
}

func displaySystemStats(stats *pm.SystemStats) {
	fmt.Println("\n" + string(make([]byte, 80, 80)))
	fmt.Printf("System Stats:\n")
	fmt.Printf("Total Processes: %d | Running: %d | Sleeping: %d | Stopped: %d | Zombie: %d\n",
		stats.TotalProcesses,
		stats.RunningProcesses,
		stats.SleepingProcesses,
		stats.StoppedProcesses,
		stats.ZombieProcesses,
	)
	fmt.Printf("CPU Usage: %.1f%% | Memory Usage: %.1f%% (%.1f GB / %.1f GB)\n",
		stats.CPUUsagePercent,
		stats.MemoryUsagePercent,
		float64(stats.UsedMemory)/1024/1024/1024,
		float64(stats.TotalMemory)/1024/1024/1024,
	)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
