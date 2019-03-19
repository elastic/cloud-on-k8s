// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/spf13/cobra"
	"os"
)

const (
	procNameFlag = "name"
	procCmdFlag  = "cmd"
)

// ProcessManager wraps a process server, a process controller and a process reaper.
type ProcessManager struct {
	server          *ProcessServer
	process         *Process
	reaper          *ProcessReaper
	keystoreUpdater *keystore.KeystoreUpdater
}

// NewProcessManager creates a new process manager.
func NewProcessManager(cmd *cobra.Command) (ProcessManager, error) {
	keystoreUpdaterCfg, err, msg := keystore.NewConfigFromFlags(cmd)
	if err != nil {
		logger.Error(err, "Error creating keystore-updater config from flags", "msg", msg)
		// FIXME
		// Continue ...
	}

	cfg, err := NewConfigFromEnv()
	if err != nil {
		return ProcessManager{}, err
	}

	process := NewProcess(cfg.ProcessName, cfg.ProcessCmd)

	return ProcessManager{
		NewServer(process),
		process,
		NewProcessReaper(),
		keystore.NewKeystoreUpdater(logger, keystoreUpdaterCfg),
	}, nil
}

// Start starts all processes, the process reaper and the HTTP server in a non-blocking way.
func (pm ProcessManager) Start() (string, error) {
	msg, err := pm.process.Start()
	if err != nil {
		return msg, err
	}

	//pm.reaper.Start()
	pm.server.Start()
	pm.keystoreUpdater.Start()

	logger.Info("Process manager started")

	return "Process manager started", nil
}

// Stop stops all processes, the process reaper and the HTTP server in a blocking way.
func (pm ProcessManager) Stop(sig os.Signal) (string, error) {
	pm.process.Kill(sig)
	pm.reaper.Stop()
	pm.server.Stop()

	logger.Info("Process manager stopped")

	return "Process manager stopped", nil
}
