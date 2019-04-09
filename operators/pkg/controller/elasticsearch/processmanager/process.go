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
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
)

const (
	killSoftSignal = syscall.SIGTERM
	killHardSignal = syscall.SIGKILL
	noSignal       = syscall.Signal(0)

	ErrNoSuchProcess = "no such process"

	startAction     = "start"
	stopAction      = "stop"
	killAction      = "kill"
	terminateAction = "terminate"

	EsConfigFilePath = "/usr/share/elasticsearch/config/elasticsearch.yml"
)

// File to persist the state of the process between restart
var processStateFile = filepath.Join(volume.ExtraBinariesPath, "process.state")

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
	started        ProcessState = "started"
	startFailed    ProcessState = "startFailed"
	failed         ProcessState = "failed"
	stopping       ProcessState = "stopping"
	stopped        ProcessState = "stopped"
	stopFailed     ProcessState = "stopFailed"
	killing        ProcessState = "killing"
	killed         ProcessState = "killed"
	killFailed     ProcessState = "killFailed"
)

func (s ProcessState) String() string {
	return string(s)
}

// ReadProcessState reads the process state in the processStateFile.
// The state is notInitialized if the file does not exist.
func ReadProcessState() ProcessState {
	data, err := ioutil.ReadFile(processStateFile)
	if err != nil {
		return notInitialized
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
	// If the state is still started, the process must have been killed
	if state == started {
		log.Info("Process marked 'started' must have been 'killed'")
		state = killed
		_ = state.Write()
	}

	p := Process{
		id:    name,
		name:  args[0],
		args:  args[1:],
		state: state,
		mutex: sync.RWMutex{},
	}

	return &p
}

// Start a process.
// A goroutine is started to monitor the end of the process in the background and
// to report the status resulting from the execution to a given ExitStatus channel done.
func (p *Process) Start(done chan ExitStatus, strict bool) (ProcessState, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Can start only if not started, stopping or killing,
	// and stopped or killed in non strict mode.
	switch p.state {
	case started:
		return p.state, nil
	case stopping, killing:
		return p.state, fmt.Errorf("error: cannot start process %s", p.state)
	case stopped, killed:
		if strict {
			log.Info("Strict mode, process stopped is not restarted")
			return p.state, nil
		}
	}

	cmd := exec.Command(p.name, p.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Dedicated process group to forward signals to the main process and all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := cmd.Start()
	if err != nil {
		p.updateState(startAction, startFailed, p.pid, noSignal, err)
		return startFailed, err
	}

	state := started
	p.pid = cmd.Process.Pid

	p.updateState(startAction, state, p.pid, noSignal, err)

	// Waiting for the process to terminate
	go func() {
		err := cmd.Wait()

		processStatus := "completed"
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				processStatus = "exited"
				if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					if waitStatus.Signal() == os.Kill {
						processStatus = "killed"
					}
					exitCode = waitStatus.ExitStatus()
				}
			} else {
				log.Info("Failed to terminate process", "err", err.Error())
				processStatus = "failed"
				exitCode = 1
			}
		}

		// Update the state depending the previous state
		p.mutex.Lock()
		switch p.state {
		case stopping:
			state = stopped
		case killing:
			state = killed
		case started:
			state = failed
		}
		p.updateState(terminateAction, state, p.pid, noSignal, nil)
		p.mutex.Unlock()

		// If the done channel is defined, then send the exit status, else exit the program
		if done != nil {
			done <- ExitStatus{processStatus, exitCode, err}
		} else {
			Exit("process "+processStatus, exitCode)
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

	action := stopAction
	state := stopping
	errState := stopFailed
	if killHard {
		action = killAction
		state = killing
		errState = killFailed
	}

	// Send signal to the whole process group
	err := syscall.Kill(-(p.pid), sig)
	if err != nil {
		state = errState
		if err.Error() == ErrNoSuchProcess {
			p.updateState(action, state, p.pid, sig, err)
			// Looks like the process is already dead. This should not happen.
			// Normally the end of the process should have been intercepted and the program exited.
			Exit("failed to kill process already dead", 1)
		}
	}

	p.updateState(action, state, p.pid, sig, err)

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

// updateState updates the process state and the last update time.
func (p *Process) updateState(action string, state ProcessState, pid int, signal syscall.Signal, err error) {
	p.state = state
	p.lastUpdate = time.Now()

	err2 := p.state.Write()
	if err2 != nil {
		log.Error(err2, "Fail to write process state")
	}

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
