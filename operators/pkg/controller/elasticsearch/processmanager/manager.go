// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"os"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	"github.com/hashicorp/go-reap"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name = "process-manager"
	log  = logf.Log.WithName(name)
)

// ProcessManager wraps a process server, a process controller, a process reaper and a keystore updater.
type ProcessManager struct {
	server          *ProcessServer
	process         *Process
	enableReaper    bool
	keystoreUpdater *keystore.Updater
}

// NewProcessManager creates a new process manager.
func NewProcessManager() (ProcessManager, error) {
	cfg, err := NewConfigFromFlags()
	if err != nil {
		return ProcessManager{}, err
	}

	var ksu *keystore.Updater
	if cfg.EnableKeystoreUpdater {
		keystoreUpdaterCfg, err, reason := keystore.NewConfigFromFlags()
		if err != nil {
			log.Error(err, "Error creating keystore-updater config from flags", "reason", reason)
			return ProcessManager{}, err
		}

		ksu = keystore.NewUpdater(keystoreUpdaterCfg)
	}

	process := NewProcess(cfg.ProcessName, cfg.ProcessCmd)

	return ProcessManager{
		NewProcessServer(cfg, process, ksu),
		process,
		cfg.EnableReaper,
		ksu,
	}, nil
}

// Start starts all processes, the process reaper and the HTTP server in a non-blocking way.
func (pm ProcessManager) Start() error {
	if pm.enableReaper {
		go reap.ReapChildren(nil, nil, nil, nil)
	}

	pm.server.Start()

	_, err := pm.process.Start()
	if err != nil {
		return err
	}

	if pm.keystoreUpdater != nil {
		pm.keystoreUpdater.Start()
	}

	log.Info("Process manager started")
	return nil
}

// Stop stops the HTTP server, forwards a given signal to the process and wait for its termination.
func (pm ProcessManager) Stop(sig os.Signal) error {
	pm.server.Stop()

	_, err := pm.process.Kill(sig)
	if err != nil {
		return err
	}

	// Wait for the process to die
	for pm.process.isAlive("process manager signal forwarding") {
		time.Sleep(1 * time.Second)
	}

	log.Info("Process manager stopped")
	return err
}
