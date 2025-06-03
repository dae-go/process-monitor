# Process Monitor

A lightweight, efficient system process monitor for Linux written in pure Go with zero external dependencies.

[![Go Report Card](https://goreportcard.com/badge/github.com/dae-go/process-monitor)](https://goreportcard.com/report/github.com/dae-go/process-monitor)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Features

- 📊 Real-time process monitoring with CPU and memory usage
- 🚀 Zero external dependencies - uses only Go standard library
- 🐧 Linux-specific implementation using `/proc` filesystem
- 🔧 Modular architecture for easy OS portability
- 📈 System-wide statistics (CPU, memory, process states)
- 🎯 Flexible sorting options (CPU, memory, PID)
- ⚡ Efficient CPU usage calculation with minimal overhead
- 🖥️ Clean, top-like terminal UI with auto-refresh

## Installation

### From Source

```bash
git clone https://github.com/dae-go/process-monitor.git
cd process-monitor
go build -o pm cmd/main.go
```

### Using Go Install

```bash
go install github.com/dae-go/process-monitor/cmd@latest
```

## Usage

### Basic Usage

```bash
# Run with default settings (updates every 2s, shows top 20 processes by CPU)
sudo ./pm

# Show help
./pm -h
```

### Command Line Options

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `-interval` | Update interval | `2s` | `-interval 1s` |
| `-top` | Number of processes to display | `20` | `-top 10` |
| `-sort` | Sort by: cpu, memory, or pid | `cpu` | `-sort memory` |

### Examples

```bash
# Monitor top CPU consumers, update every second
sudo ./pm -sort cpu -interval 1s

# Find memory hogs, show top 15
sudo ./pm -sort memory -top 15

# Quick system check with 5-second updates
sudo ./pm -interval 5s

# Detailed monitoring with rapid updates
sudo ./pm -interval 500ms -top 50
```

## Output Format

```
Process Monitor - 2024-01-15 14:32:45
================================================================================
PID    NAME                 CPU%   MEM%   MEM(MB)   STATE   USER
---    ----                 ----   ----   -------   -----   ----
1234   firefox              15.2   8.5    1024.5    S       john
5678   chrome               12.1   6.2    756.3     S       john
9012   code                 8.5    4.1    498.2     S       john

================================================================================
System Stats:
Total Processes: 245 | Running: 2 | Sleeping: 240 | Stopped: 0 | Zombie: 3
CPU Usage: 23.4% | Memory Usage: 45.2% (7.2 GB / 16.0 GB)
```

### Process States

- **R**: Running
- **S**: Sleeping (interruptible)
- **D**: Disk sleep (uninterruptible)
- **T**: Stopped
- **Z**: Zombie

## Architecture

The project follows the standard Go project layout for better organization and maintainability:

```
process-monitor/
├── cmd/
│   └── main.go               # CLI interface and display logic
├── pkg/
│   ├── monitor.go            # Platform-agnostic interfaces and common logic
│   └── linux.go              # Linux-specific implementation
├── go.mod
├── go.sum
└── README.md
```

### Key Components

1. **ProcessMonitor Interface**: Defines the contract for OS-specific implementations
2. **Monitor Struct**: Provides a unified API regardless of the underlying OS
3. **Platform-specific Implementation**: Currently Linux-only, reads from `/proc` filesystem

### Adding OS Support

To add support for a new operating system:

1. Create a new file in `pkg/` (e.g., `windows.go`, `darwin.go`)
2. Add appropriate build tags: `// +build windows`
3. Implement the `ProcessMonitor` interface
4. The build system will automatically use the correct implementation

## API Usage

You can also use this as a library in your own Go projects:

```go
package main

import (
    "fmt"
    "log"
    "github.com/dae-go/process-monitor/pkg"
)

func main() {
    // Create a new monitor
    monitor, err := pkg.New()
    if err != nil {
        log.Fatal(err)
    }

    // Get all processes
    processes, err := monitor.GetProcesses()
    if err != nil {
        log.Fatal(err)
    }

    // Sort by CPU usage
    pkg.SortProcesses(processes, func(i, j int) bool {
        return processes[i].CPUPercent > processes[j].CPUPercent
    })

    // Display top 5
    for i := 0; i < 5 && i < len(processes); i++ {
        p := processes[i]
        fmt.Printf("%d: %s (CPU: %.1f%%, Mem: %.1f%%)\n", 
            p.PID, p.Name, p.CPUPercent, p.MemoryPercent)
    }

    // Get system statistics
    stats, err := monitor.GetSystemStats()
    if err == nil {
        fmt.Printf("System CPU: %.1f%%, Memory: %.1f%%\n", 
            stats.CPUUsagePercent, stats.MemoryUsagePercent)
    }
}
```

## Requirements

- Go 1.16 or higher
- Linux operating system (for current implementation)
- Root/sudo privileges for full process information

## Performance

- Minimal CPU overhead (~0.1-0.5% on modern systems)
- Memory usage: ~10-20MB depending on system process count
- Efficient `/proc` parsing with buffered I/O
- CPU percentage calculated using jiffy deltas between samples

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. For major changes, please open an issue first to discuss what you would like to change.

### Development Setup

```bash
# Clone the repository
git clone https://github.com/dae-go/process-monitor.git
cd process-monitor

# Run tests
go test ./...

# Build
go build -o pm cmd/main.go

# Run with race detector during development
go run -race cmd/main.go
```

### Code Style

- Follow standard Go formatting (`go fmt`)
- Add comments for exported functions
- Keep OS-specific code in separate files with build tags
- Write unit tests for new functionality

## Roadmap

- [ ] macOS support (using sysctl)
- [ ] Windows support (using Windows API)
- [ ] Process tree visualization
- [ ] Historical data tracking
- [ ] JSON/CSV output formats
- [ ] Process filtering by name/user
- [ ] Network connections per process
- [ ] Disk I/O statistics
- [ ] Docker container support
- [ ] Configuration file support

## Troubleshooting

### Permission Denied

Run with `sudo` for full process information:
```bash
sudo ./pm
```

### High CPU Usage

If the monitor itself is using too much CPU, increase the update interval:
```bash
./pm -interval 5s
```

### Missing Processes

Some processes may not be visible without root privileges. Always run with `sudo` for complete system visibility.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Inspired by classic Unix tools like `top` and `htop`
- Built with Go's excellent standard library
- Thanks to the Linux kernel developers for the `/proc` filesystem

## Author

Created by [icarus612](https://github.com/icarus612)

## Support

If you find this project useful, please consider giving it a ⭐ on GitHub!