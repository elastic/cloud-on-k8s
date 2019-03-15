package main

import (
	"strings"
	"sync"
)

const HTTPPort = ":8080"

// ProcessManager wraps a process server, a process controller and a process reaper.
type ProcessManager struct {
	server     *ProcessServer
	controller *ProcessController
	reaper     *ProcessReaper
}

// NewProcessManager creates a new process manager.
func NewProcessManager() ProcessManager {
	controller := &ProcessController{
		processes: map[string]*Process{},
		lock:      sync.RWMutex{},
	}

	return ProcessManager{
		NewServer(controller),
		controller,
		NewProcessReaper(),
	}
}

// Register registers a new process given a name and a command.
func (pm ProcessManager) Register(name string, cmd string) {
	cmdArgs := strings.Split(strings.Trim(processCmd, " "), " ")
	pm.controller.Register(&Process{id: processName, name: cmdArgs[0], args: cmdArgs[1:]})
}

// Start starts all processes, the process reaper and the HTTP server in a non-blocking way.
func (pm ProcessManager) Start() {
	pm.controller.StartAll()
	pm.reaper.Start()
	pm.server.Start()
	logger.Info("Process manager started")
}

// Stop stops all processes, the process reaper and the HTTP server in a blocking way.
func (pm ProcessManager) Stop() {
	pm.controller.StopAll()
	pm.reaper.Stop()
	pm.server.Stop()
	logger.Info("Process manager stopped")
}
