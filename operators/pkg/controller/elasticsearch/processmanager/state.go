// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

// File to persist the state of the process between restarts
var processStateFile = filepath.Join(volume.ProcessManagerEmptyDirMountPath, "process.state")

// ProcessState represents the state of a process.
type ProcessState string

const (
	NotInitialized ProcessState = "notInitialized"
	Started        ProcessState = "started"
	StartFailed    ProcessState = "startFailed"
	Stopping       ProcessState = "stopping"
	Stopped        ProcessState = "stopped"
	StopFailed     ProcessState = "stopFailed"
	Killing        ProcessState = "killing"
	Killed         ProcessState = "killed"
	KillFailed     ProcessState = "killFailed"
	Failed         ProcessState = "failed"
)

func (s ProcessState) String() string {
	return string(s)
}

// ReadProcessState reads the process state in the processStateFile.
// The state is notInitialized if the file does not exist or an IO error occurs.
func ReadProcessState() ProcessState {
	if _, err := os.Stat(processStateFile); os.IsNotExist(err) {
		return NotInitialized
	}

	data, err := ioutil.ReadFile(processStateFile)
	if err != nil {
		log.Error(err, "Failed to read process state file")
		return NotInitialized
	}

	return ProcessState(string(data))
}

// Write the process state in the processStateFile.
func (s ProcessState) Write() error {
	return ioutil.WriteFile(processStateFile, []byte(s), 0644)
}

func (s ProcessState) Error() error {
	return fmt.Errorf("error: process %s", s)
}

// ShouldBeStarted returns if the process should be started regarding its actual state.
// It should be started if it's not stopping, stopped, killing or killed.
// Used when the process manager must decide whether to start the process.
func (p *Process) ShouldBeStarted() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	switch p.state {
	case Stopping, Stopped, Killing, Killed:
		return false
	}

	return true
}

// CanBeStopped returns if the process can be stopped regarding its actual state.
// It can be stopped only if it's not stopped or killed.
// Used when the process manager must decide whether to stop the process by forwarding
// a signal.
func (p *Process) CanBeStopped() bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	switch p.state {
	case Stopped, Killed:
		return false
	}

	return true
}

// updateState updates the process state to the next state given an action and an error.
func (p *Process) updateState(action string, signal syscall.Signal, lastErr error) ProcessState {
	p.state = nextState(p.state, action, lastErr)
	p.lastUpdate = time.Now()

	err := p.state.Write()
	if err != nil {
		Exit("Failed to write process state", 1)
	}

	kv := []interface{}{"action", action, "id", p.id, "state", p.state, "pid", p.pid}
	if signal != noSignal {
		kv = append(kv, "signal", signal)
	}
	if lastErr != nil {
		kv = append(kv, "err", lastErr)
	}
	log.Info("Update process state", kv...)

	return p.state
}

// nextState returns the next state given an action and an error.
func nextState(state ProcessState, action string, err error) ProcessState {
	switch action {
	case initAction:
		// If the state is still started, the process must have been failed or restarted by an external program
		if state == Started {
			log.Info("Process marked 'started' on init must have been 'failed'")
			return Failed
		}
		return state
	case startAction:
		if err != nil {
			return StartFailed
		}
		return Started
	case stopAction:
		if err != nil {
			return StopFailed
		}
		return Stopping
	case killAction:
		if err != nil {
			return KillFailed
		}
		return Killing
	case terminateAction:
		switch state {
		case Stopping:
			return Stopped
		case Killing:
			return Killed
		case Started:
			return Failed
		}
	default:
		panic(fmt.Sprintf("Unknown action: %s", action))
	}

	return state
}
