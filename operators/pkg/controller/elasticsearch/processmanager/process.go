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
	defaultKillHardTimeout = 1 * time.Hour
	killSoftSignal         = syscall.SIGTERM
	killHardSignal         = syscall.SIGKILL

	errNoChildProcesses = "waitid: no child processes"
	ErrNoSuchProcess    = "no such process"
	errSignalTerminated = "signal: terminated"
	errSignalKilled     = "signal: killed"

	stopAction  = "stop"
	startAction = "start"
	noSignal    = syscall.Signal(0)

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
	notInitialized ProcessState = "notInitialized"
	noProcess      ProcessState = "noProcess"
	unknown        ProcessState = "unknown"
	starting       ProcessState = "starting"
	started        ProcessState = "started"
	stopping       ProcessState = "stopping"
	stopped        ProcessState = "stopped"
	killing        ProcessState = "killing"
	killed         ProcessState = "killed"
	startFailed    ProcessState = "startFailed"
	stopFailed     ProcessState = "stopFailed"
	killFailed     ProcessState = "killFailed"
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

	cmd        *exec.Cmd
	state      ProcessState
	mutex      sync.RWMutex
	lastUpdate time.Time
}

func NewProcess(name string, cmd string) *Process {
	args := strings.Split(strings.Trim(cmd, " "), " ")
	return &Process{
		id:    name,
		name:  args[0],
		args:  args[1:],
		cmd:   nil,
		state: notInitialized,
		mutex: sync.RWMutex{},
	}
}

// Start starts a process in a non blocking way.
// The process is started only if it's not starting, started or stopping.
// It returns an error if the process is stopping.
func (p *Process) Start() (ProcessState, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Can start?
	switch p.state {
	case starting, started:
		return p.state, nil
	case stopping:
		return p.state, p.state.Error()
	default:
		p.updateState(startAction, starting, 0, syscall.Signal(0), nil)
	}

	go p.exec()

	return p.state, nil
}

// exec executes the process command and updates the process state.
func (p *Process) exec() ProcessState {
	cmd := exec.Command(p.name, p.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Dedicated process group to forward signals to the main process and all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	state := started
	if err != nil {
		state = startFailed
		cmd = nil
	}

	p.mutex.Lock()
	p.updateState(startAction, state, 0, syscall.Signal(0), err)
	p.cmd = cmd
	p.mutex.Unlock()

	return state
}

// Kill kills a process group by forwarding a signal.
// Useful to stop the process when the program ends.
func (p *Process) Kill(sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok {
		err := errors.New("os: unsupported signal type")
		log.Error(err, "Fail to kill process")
		return
	}

	p.mutex.RLock()
	defer p.mutex.RUnlock()
	if ok := canStop(p.state, s == killHardSignal); !ok {
		log.Info("Process not killed", "state", p.state)
		return
	}

	err := killProcessGroup(p.cmd.Process.Pid, s)
	if err != nil {
		if err.Error() != ErrNoSuchProcess {
			log.Error(err, "Fail to kill process", "state", p.state)
			return
		}
	}

	log.Info("Process killed", "signal", sig)
}

func canStop(state ProcessState, killHard bool) bool {
	switch state {
	case stopping:
		return killHard
	case stopped, killing, killed, noProcess, notInitialized, startFailed:
		return false
	default:
		return true
	}
}

// Stop stops a process in a non blocking way.
// The process is stopped only if it's not stopped, killing, killed, not found or not initialized.
// An error is returned if the process is starting or being killed.
func (p *Process) Stop(killHard bool, killHardTimeout time.Duration) (ProcessState, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	prevState := p.state

	ok := canStop(prevState, killHard)
	if !ok {
		return prevState, nil
	}
	pid := p.cmd.Process.Pid

	// Get the pgid to kill the process group.
	// This allows to check that the process is still alive.
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		p.updateState(stopAction, noProcess, pid, noSignal, err)
		p.cmd = nil
		return noProcess, err
	}

	state := stopping
	signal := killSoftSignal
	if killHard {
		signal = killHardSignal
		state = killing
	}

	p.updateState(stopAction, state, pid, signal, nil)

	err = killProcessGroup(pgid, signal)
	if err != nil {
		state = killFailed
		p.updateState(stopAction, state, pgid, signal, err)
		p.cmd = nil
		return state, err
	}

	if prevState != stopping {
		go p.wait(pgid, signal, killHard, killHardTimeout)
	}

	return state, nil
}

// wait waits for a process to complete with a timeout to trigger a kill hard.
func (p *Process) wait(pgid int, signal syscall.Signal, hard bool, killHardTimeout time.Duration) {
	if killHardTimeout == 0 {
		killHardTimeout = defaultKillHardTimeout
	}

	killTimer := time.AfterFunc(killHardTimeout, func() {
		log.Info("Killing process as timeout is reached", "timeout", killHardTimeout)
		err := killProcessGroup(pgid, killHardSignal)
		state := killed
		if err != nil {
			state = killFailed
		}

		p.mutex.Lock()
		p.updateState(stopAction, state, pgid, signal, err)
		p.cmd = nil
		p.mutex.Unlock()
	})

	err := p.cmd.Wait()
	killTimer.Stop()

	// Read again the process state
	p.mutex.RLock()
	prevState := p.state
	p.mutex.RUnlock()

	// Should be stopping or killing
	state := unknown
	switch prevState {
	case stopping:
		state = stopped
	case killing:
		state = killed
	}

	if err != nil {
		switch err.Error() {
		// No more children, we are done
		case errNoChildProcesses:
			err = nil
		// Normal stop signal
		case errSignalTerminated, errSignalKilled:
			err = nil
		// An error occurred
		default:
			switch state {
			case stopping:
				state = stopFailed
			case killing:
				state = killFailed
			}
		}
	}

	p.mutex.Lock()
	p.updateState(stopAction, state, pgid, signal, err)
	p.cmd = nil
	p.mutex.Unlock()
}

// updateState updates the process state and the last update time.
func (p *Process) updateState(action string, state ProcessState, pid int, signal syscall.Signal, err error) {
	p.state = state
	p.lastUpdate = time.Now()

	kv := []interface{}{"action", action, "id", p.id, "state", state}
	if signal != noSignal {
		kv = append(kv, "pid", pid, "signal", signal)
	}
	if err != nil {
		log.Error(err, "Update process state", kv...)
	} else {
		log.Info("Update process state", kv...)
	}
}

// Status returns the status of the process.
func (p *Process) Status() ProcessStatus {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Check that the process is still alive
	if p.state == started {
		pid := p.cmd.Process.Pid
		_, err := syscall.Getpgid(pid)
		if err != nil {
			p.updateState(stopAction, noProcess, pid, noSignal, err)
			p.cmd = nil
		}
	}

	cfgChecksum, _ := computeConfigChecksum()

	return ProcessStatus{
		p.state,
		time.Since(p.lastUpdate).String(),
		cfgChecksum,
	}
}

func computeConfigChecksum() (string, error) {
	data, err := ioutil.ReadFile(EsConfigFilePath)
	if err != nil {
		return "unknown", err
	}

	return fmt.Sprint(crc32.ChecksumIEEE(data)), nil
}

func killProcessGroup(pgid int, signal syscall.Signal) error {
	err := syscall.Kill(-(pgid), signal)
	if err != nil && err.Error() != errNoChildProcesses {
		return err
	}

	return nil
}
