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

	initAction      = "initialization"
	startAction     = "start"
	stopAction      = "stop"
	killAction      = "kill"
	terminateAction = "terminate"

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

	state := ReadProcessState()

	p := Process{
		id:    name,
		name:  args[0],
		args:  args[1:],
		state: state,
		mutex: sync.RWMutex{},
	}

	p.updateState(initAction, noSignal, nil)

	return &p
}

// Start a process.
// A goroutine is started to monitor the end of the process in the background and
// to report the status resulting from the execution to a given ExitStatus channel done.
func (p *Process) Start(done chan ExitStatus) (ProcessState, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Can start only if not started, stopping or killing,
	// and stopped or killed in non strict mode.
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
		p.updateState(startAction, noSignal, err)
		return p.state, err
	}

	p.pid = cmd.Process.Pid

	p.updateState(startAction, noSignal, err)

	// Waiting for the process to terminate
	go func() {
		err := cmd.Wait()

		// Update the state depending the previous state
		p.mutex.Lock()
		state := p.updateState(terminateAction, noSignal, nil)
		p.mutex.Unlock()

		code := exitCode(err)

		// If the done channel is defined, then send the exit status, else exit the program
		if done != nil {
			done <- ExitStatus{state, code, err}
		} else {
			Exit(fmt.Sprintf("process %s", state), code)
		}
	}()

	return p.state, nil
}

// Kill a process group by sending a signal.
func (p *Process) Kill(s os.Signal) (ProcessState, error) {
	sig, ok := s.(syscall.Signal)
	if !ok {
		err := errors.New("os: unsupported signal type")
		return stopFailed, err
	}
	killHard := sig == killHardSignal

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Can stop or kill?
	switch p.state {
	case stopping:
		if !killHard {
			return p.state, nil
		}
	case stopped, killing, killed, failed:
		return p.state, nil
	}

	var action string
	if killHard {
		action = killAction
	} else {
		action = stopAction
	}

	// Send signal to the whole process group
	err := syscall.Kill(-(p.pid), sig)
	if err != nil {
		if err.Error() == ErrNoSuchProcess {
			p.updateState(action, sig, err)
			// Looks like the process is already dead. This should not happen.
			// Normally the end of the process should have been intercepted and the program exited.
			Exit("failed to kill process already dead", 1)
		}
	}

	p.updateState(action, sig, err)

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

func computeConfigChecksum() (string, error) {
	data, err := ioutil.ReadFile(EsConfigFilePath)
	if err != nil {
		return "unknown", err
	}

	return fmt.Sprint(crc32.ChecksumIEEE(data)), nil
}

// exitCode tries to extract the exit code from an error
func exitCode(err error) int {
	exitCode := 0
	if err != nil {
		exitCode = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = waitStatus.ExitStatus()
			}
		} else {
			log.Info("Failed to terminate process", "err", err.Error())
			exitCode = 1
		}
	}
	return exitCode
}
