// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"os"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
	reap "github.com/hashicorp/go-reap"
	"github.com/pkg/errors"
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
func NewProcessManager(cfg *Config) (*ProcessManager, error) {
	var ksu *keystore.Updater
	if cfg.EnableKeystoreUpdater {
		ksCfg, err, reason := keystore.NewConfigFromFlags()
		if err != nil {
			log.Error(err, "Failed to create keystore updater config from flags", "reason", reason)
			return nil, err
		}

		ksu, err = keystore.NewUpdater(
			ksCfg,
			keystore.NewEsClient(ksCfg),
			keystore.NewKeystore(ksCfg),
		)
		if err != nil {
			log.Error(err, "Failed to create keystore updater")
			return nil, err
		}
	}

	process := NewProcess(cfg.ProcessName, cfg.ProcessCmd)

	return &ProcessManager{
		NewProcessServer(cfg, process, ksu),
		process,
		cfg.EnableReaper,
		ksu,
	}, nil
}

// Start the process reaper, the HTTP server, the managed process and the keystore updater.
func (pm ProcessManager) Start(done chan ExitStatus) error {
	log.Info("Starting...")

	if pm.enableReaper {
		go reap.ReapChildren(nil, nil, nil, nil)
	}

	pm.server.Start()

	if pm.process.ShouldBeStarted() {
		_, err := pm.process.Start(done)
		if err != nil {
			return err
		}
	} else {
		log.Info("Process not restarted")
	}

	if pm.keystoreUpdater != nil {
		pm.keystoreUpdater.Start()
	}

	log.Info("Started")
	return nil
}

// Forward a given signal to the process to kill it.
func (pm ProcessManager) Forward(sig os.Signal) error {
	log.Info("Forwarding signal", "sig", sig)

	if pm.process.CanBeStopped() {
		err := pm.process.Kill(sig)
		if err != nil {
			return err
		}
	} else {
		return errors.New("process not started to forward signal")
	}

	return nil
}

type ExitStatus struct {
	processState ProcessState
	exitCode     int
	err          error
}

// WaitToExit waits to exit the HTTP server and the program.
func (pm ProcessManager) WaitToExit(done chan ExitStatus) {
	s := <-done
	pm.server.Exit()
	Exit("process "+s.processState.String(), s.exitCode)
}

// Exit the program.
func Exit(reason string, code int) {
	log.Info("Exit", "reason", reason, "code", code)
	os.Exit(code)
}
