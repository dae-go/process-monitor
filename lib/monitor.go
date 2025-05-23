// monitorProcess handles the lifecycle of a single process
func (pm *ProcessMonitor) monitorProcess(name string, proc *ProcessInfo) {
	defer pm.wg.Done()

	pm.logger.Printf("Starting monitor for process: %s", name)

	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
			pm.ensureProcessRunning(name, proc)
			time.Sleep(5 * time.Second) // Check every 5 seconds
		}
	}
}

// ensureProcessRunning starts the process if it's not running
func (pm *ProcessMonitor) ensureProcessRunning(name string, proc *ProcessInfo) {
	proc.mutex.Lock()
	defer proc.mutex.Unlock()

	if proc.Status == StatusRunning && proc.cmd != nil {
		// Check if the process is still alive
		if proc.cmd.Process == nil {
			proc.Status = StatusStopped
		} else {
			// Process exists, check if it's still running
			if !pm.isProcessAlive(proc.cmd.Process.Pid) {
				proc.Status = StatusStopped
				proc.cmd = nil
			}
		}
	}

	if proc.Status == StatusStopped && pm.shouldRestart(proc) {
		pm.startProcess(name, proc)
	}
}

// startProcess starts a new instance of the process
func (pm *ProcessMonitor) startProcess(name string, proc *ProcessInfo) {
	pm.logger.Printf("Starting process: %s", name)
	proc.Status = StatusStarting

	// Create context for this process
	proc.ctx, proc.cancel = context.WithCancel(pm.ctx)

	// Prepare command
	proc.cmd = exec.CommandContext(proc.ctx, proc.Config.Command[0], proc.Config.Command[1:]...)

	// Set working directory
	if proc.Config.WorkingDir != "" {
		proc.cmd.Dir = proc.Config.WorkingDir
	}

	// Set environment variables
	if len(proc.Config.Environment) > 0 {
		env := os.Environ()
		for key, value := range proc.Config.Environment {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		proc.cmd.Env = env
	}

	// Start the process
	err := proc.cmd.Start()
	if err != nil {
		pm.logger.Printf("Failed to start process %s: %v", name, err)
		proc.Status = StatusFailed
		proc.LastError = err
		return
	}

	proc.PID = proc.cmd.Process.Pid
	proc.Status = StatusRunning
	proc.StartTime = time.Now()
	proc.RestartCount++
	proc.RestartHistory = append(proc.RestartHistory, proc.StartTime)
	proc.LastError = nil

	pm.logger.Printf("Process %s started with PID %d", name, proc.PID)

	// Start a goroutine to wait for the process to exit
	go pm.waitForProcess(name, proc)
}

// waitForProcess waits for a process to exit and handles the exit
func (pm *ProcessMonitor) waitForProcess(name string, proc *ProcessInfo) {
	err := proc.cmd.Wait()

	proc.mutex.Lock()
	defer proc.mutex.Unlock()

	if err != nil {
		pm.logger.Printf("Process %s exited with error: %v", name, err)
		proc.LastError = err
	} else {
		pm.logger.Printf("Process %s exited normally", name)
	}

	proc.Status = StatusStopped
	proc.cmd = nil
}

// stopProcess stops a running process
func (pm *ProcessMonitor) stopProcess(proc *ProcessInfo) {
	proc.mutex.Lock()
	defer proc.mutex.Unlock()

	if proc.cmd != nil && proc.cmd.Process != nil {
		// Try graceful shutdown first
		proc.cmd.Process.Signal(os.Interrupt)

		// Wait a bit for graceful shutdown
		done := make(chan bool, 1)
		go func() {
			proc.cmd.Wait()
			done <- true
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(10 * time.Second):
			// Force kill after timeout
			proc.cmd.Process.Kill()
		}
	}

	if proc.cancel != nil {
		proc.cancel()
	}

	proc.Status = StatusStopped
	proc.cmd = nil
}

// shouldRestart determines if a process should be restarted
func (pm *ProcessMonitor) shouldRestart(proc *ProcessInfo) bool {
	if !proc.Config.RestartOnExit {
		return false
	}

	// Check if we've exceeded the restart limit
	restartWindow, _ := time.ParseDuration(pm.config.RestartWindow)
	cutoff := time.Now().Add(-restartWindow)

	recentRestarts := 0
	for _, restartTime := range proc.RestartHistory {
		if restartTime.After(cutoff) {
			recentRestarts++
		}
	}

	if recentRestarts >= pm.config.MaxRestarts {
		pm.logger.Printf("Process %s exceeded restart limit (%d restarts in %s), not restarting",
			proc.Config.Name, recentRestarts, pm.config.RestartWindow)
		return false
	}

	return true
}

// monitoringLoop runs the main monitoring loop
func (pm *ProcessMonitor) monitoringLoop() {
	defer pm.wg.Done()

	interval, err := time.ParseDuration(pm.config.CheckInterval)
	if err != nil {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.performHealthChecks()
			pm.checkResourceUsage()
		}
	}
}

// performHealthChecks runs health checks on all processes
func (pm *ProcessMonitor) performHealthChecks() {
	pm.mutex.RLock()
	processes := make([]*ProcessInfo, 0, len(pm.processes))
	for _, proc := range pm.processes {
		processes = append(processes, proc)
	}
	pm.mutex.RUnlock()

	for _, proc := range processes {
		if proc.Config.HealthCheck != nil {
			go pm.runHealthCheck(proc)
		}
	}
}

// runHealthCheck performs a health check on a process
func (pm *ProcessMonitor) runHealthCheck(proc *ProcessInfo) {
	proc.mutex.Lock()
	defer proc.mutex.Unlock()

	if proc.Status != StatusRunning {
		return
	}

	hc := proc.Config.HealthCheck
	timeout, _ := time.ParseDuration(hc.Timeout)
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var healthy bool
	var err error

	switch hc.Type {
	case "http":
		healthy, err = pm.httpHealthCheck(ctx, hc.Target)
	case "tcp":
		healthy, err = pm.tcpHealthCheck(ctx, hc.Target)
	case "command":
		healthy, err = pm.commandHealthCheck(ctx, hc.Target)
	default:
		pm.logger.Printf("Unknown health check type: %s", hc.Type)
		return
	}

	proc.LastHealthCheck = time.Now()

	if err != nil {
		pm.logger.Printf("Health check failed for %s: %v", proc.Config.Name, err)
		proc.Status = StatusUnhealthy
		proc.LastError = err

		// Consider restarting on health check failure
		if pm.shouldRestart(proc) {
			pm.logger.Printf("Restarting unhealthy process: %s", proc.Config.Name)
			pm.stopProcess(proc)
		}
	} else if !healthy {
		pm.logger.Printf("Health check failed for %s", proc.Config.Name)
		proc.Status = StatusUnhealthy
	} else {
		if proc.Status == StatusUnhealthy {
			pm.logger.Printf("Process %s is healthy again", proc.Config.Name)
		}
		proc.Status = StatusRunning
		proc.LastError = nil
	}
}