// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	killSoftSignal = syscall.SIGTERM
	killHardSignal = syscall.SIGKILL
	noSignal       = syscall.Signal(0)

	ErrNoSuchProcess = "no such process"

	stopAction  = "stop"
	startAction = "start"
	waitAction  = "wait"

	EsConfigFilePath = "/usr/share/elasticsearch/config/elasticsearch.yml"
)

// ProcessStatus represents the status of a process with its state,
// the duration since when it is in this state and the checksum of
// the Elasticsearch configuration.
type ProcessStatus struct {
	State          ProcessState `json:"state"`
	Since          string       `json:"since"`
	ConfigChecksum string       `json:"config_checksum"`
}

// ProcessState represents the state of a process.
type ProcessState string

const (
	started     ProcessState = "started"
	stopping    ProcessState = "stopping"
	stopped     ProcessState = "stopped"
	killing     ProcessState = "killing"
	killed      ProcessState = "killed"
	startFailed ProcessState = "startFailed"
	stopFailed  ProcessState = "stopFailed"
	killFailed  ProcessState = "killFailed"
	failed      ProcessState = "failed"
)

func (s ProcessState) String() string {
	return string(s)
}

func (s ProcessState) Error() error {
	return fmt.Errorf("error: process %s", s)
}

type Process struct {
	id   string
	name string
	args []string

	pid        int
	state      ProcessState
	mutex      sync.RWMutex
	lastUpdate time.Time
}

// NewProcess create a new process.
func NewProcess(name string, cmd string) *Process {
	args := strings.Split(strings.Trim(cmd, " "), " ")
	return &Process{
		id:    name,
		name:  args[0],
		args:  args[1:],
		state: stopped,
		mutex: sync.RWMutex{},
	}
}

// Start starts a process.
// The process is started only if it's not starting, started or stopping.
// It returns an error if the process is stopping or killing.
// A goroutine is started to monitor the end of the process in the background.
func (p *Process) Start() (ProcessState, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Can start only if not started, stopping or killing
	switch p.state {
	case started:
		return p.state, nil
	case stopping, killing:
		return p.state, fmt.Errorf("error: cannot start process %s", p.state)
	}

	cmd := exec.Command(p.name, p.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Dedicated process group to forward signals to the main process and all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	if err != nil {
		return startFailed, err
	}

	state := started
	p.pid = cmd.Process.Pid

	p.updateState(startAction, state, p.pid, noSignal, err)

	go func() {
		_ = cmd.Wait()
		p.isAlive(waitAction)
	}()

	return p.state, err
}

// Kill kills a process group by forwarding a signal.
// The process is stopped only if it's not stopping, killing, stopped or killed.
func (p *Process) Kill(s os.Signal) (ProcessState, error) {
	sig, ok := s.(syscall.Signal)
	if !ok {
		err := errors.New("os: unsupported signal type")
		return stopFailed, err
	}
	killHard := sig == killHardSignal

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Can kill?
	switch p.state {
	case stopping:
		if !killHard {
			return p.state, nil
		}
	case killing:
		if killHard {
			return p.state, nil
		}
	case stopped, killed:
		return p.state, nil
	}

	state := stopping
	errState := stopFailed
	if killHard {
		state = killing
		errState = killFailed
	}

	err := syscall.Kill(-(p.pid), sig)
	if err != nil {
		if p.state == started && err.Error() == ErrNoSuchProcess {
			// Should not happen
			exitOnEsFailure(stopAction, err)
		}
		state = errState
	}

	p.updateState(stopAction, state, p.pid, sig, err)

	return p.state, err
}

// Status returns the status of the process.
func (p *Process) Status() ProcessStatus {
	cfgChecksum, _ := computeConfigChecksum()

	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return ProcessStatus{
		p.state,
		time.Since(p.lastUpdate).String(),
		cfgChecksum,
	}
}

// isAlive returns if the process is alive by trying to get the process group id.
// The process state is updated if the process is not alive.
func (p *Process) isAlive(action string) bool {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	_, err := syscall.Getpgid(p.pid)
	alive := err == nil

	// Update the state if the process is dead
	state := failed
	if !alive {
		switch p.state {
		case stopping:
			state = stopped
		case killing:
			state = killed
		case started:
			exitOnEsFailure(waitAction, err)
		}
	}

	p.updateState(action, state, p.pid, noSignal, err)

	return alive
}

// updateState updates the process state and the last update time.
func (p *Process) updateState(action string, state ProcessState, pid int, signal syscall.Signal, err error) {
	p.state = state
	p.lastUpdate = time.Now()

	kv := []interface{}{"action", action, "id", p.id, "state", state, "pid", pid}
	if signal != noSignal {
		kv = append(kv, "signal", signal)
	}
	if err != nil {
		kv = append(kv, "err", err)
	}
	log.Info("Update process state", kv...)
}

func computeConfigChecksum() (string, error) {
	data, err := ioutil.ReadFile(EsConfigFilePath)
	if err != nil {
		return "unknown", err
	}

	return fmt.Sprint(crc32.ChecksumIEEE(data)), nil
}

func exitOnEsFailure(action string, err error) {
	log.Info("Fatal error: process es failed", "action", action, "err", err)
	os.Exit(1)
}
