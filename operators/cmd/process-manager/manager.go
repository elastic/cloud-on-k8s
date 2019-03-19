// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/elastic/k8s-operators/operators/cmd/keystore-updater"
	"github.com/go-logr/logr"
	"strings"
	"sync"
)

const HTTPPort = ":8080"

// ProcessManager wraps a process server, a process controller and a process reaper.
type ProcessManager struct {
	server          *ProcessServer
	controller      *ProcessController
	reaper          *ProcessReaper
	keystoreUpdater *keystore.KeystoreUpdater
}

// NewProcessManager creates a new process manager.
func NewProcessManager() ProcessManager {
	controller := &ProcessController{
		processes: map[string]*Process{},
		lock:      sync.Mutex{},
	}

	keystoreUpdaterCfg, err, msg := keystore.NewConfigFromFlags(nil)
	if err != nil {
		logger.Error(err, "Error reading keystore-updater config from flags", "msg", msg)
		// FIXME
		// Continue ...
	}

	return ProcessManager{
		NewServer(controller),
		controller,
		NewProcessReaper(),
		keystore.NewKeystoreUpdater(logger, keystoreUpdaterCfg),
	}
}

// Register registers a new process given a name and a command.
func (pm ProcessManager) Register(name string, cmd string) {
	cmdArgs := strings.Split(strings.Trim(cmd, " "), " ")
	pm.controller.Register(&Process{id: name, name: cmdArgs[0], args: cmdArgs[1:]})
}

// Start starts all processes, the process reaper and the HTTP server in a non-blocking way.
func (pm ProcessManager) Start() {
	pm.controller.StartAll()
	//pm.reaper.Start()
	pm.server.Start()
	pm.keystoreUpdater.Run()
	pm.logger.Info("Process manager started")
}

// Stop stops all processes, the process reaper and the HTTP server in a blocking way.
func (pm ProcessManager) Stop() {
	pm.controller.StopAll()
	//pm.reaper.Stop()
	pm.server.Stop()
	logger.Info("Process manager stopped")
}
