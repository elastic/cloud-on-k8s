// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"errors"
	"fmt"
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
	errNoChildProcesses    = "waitid: no child processes"
	errNoSuchProcess       = "no such process"
	errSignalTerminated    = "signal: terminated"
	errSignalKilled        = "signal: killed"

	notInitialized ProcessState = "notInitialized"
	starting       ProcessState = "starting"
	started        ProcessState = "started"
	stopping       ProcessState = "stopping"
	stopped        ProcessState = "stopped"
	startFailed    ProcessState = "startFailed"
	killFailed     ProcessState = "killFailed"
	noProcess      ProcessState = "noProcess"

	stopAction            = "stop"
	startAction           = "start"
	hardKillTimeoutAction = "hardKillTimeout"
	noSignal              = syscall.Signal(0)
)

type ProcessState string

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

	cmd   *exec.Cmd
	state ProcessState
	mutex sync.RWMutex
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

func canStart(state ProcessState) (bool, ProcessState, error) {
	switch state {
	case starting, started:
		return false, state, nil
	case stopping:
		return false, state, state.Error()
	default:
		return true, state, nil
	}
}

func (p *Process) Start() (string, error) {
	p.mutex.Lock()

	ok, state, err := canStart(p.state)
	if !ok {
		p.mutex.Unlock()
		return state.String(), err
	}

	p.updateState(startAction, starting, 0, syscall.Signal(0), nil)
	p.mutex.Unlock()

	go func() {
		newState := p.exec()
		p.mutex.Lock()
		p.updateState(startAction, newState, 0, syscall.Signal(0), err)
		p.mutex.Unlock()
	}()

	return starting.String(), nil
}

func (p *Process) exec() ProcessState {
	cmd := exec.Command(p.name, p.args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Dedicated process group to forward signals to the main process and all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return startFailed
	}

	p.cmd = cmd
	return started
}

func (p *Process) Kill(sig os.Signal) {
	s, ok := sig.(syscall.Signal)
	if !ok {
		err := errors.New("os: unsupported signal type")
		logger.Error(err, "Fail to kill process")
	}

	err := killProcessGroup(p.cmd.Process.Pid, s)
	if err != nil {
		if err.Error() != errNoSuchProcess {
			logger.Error(err, "Fail to kill process")
		}
	}
	logger.Info("Process killed")
}

func canStop(state ProcessState) (bool, ProcessState, error) {
	switch state {
	case stopped, noProcess, notInitialized:
		return false, state, nil
	case stopping, starting:
		return false, state, state.Error()
	default:
		return true, state, nil
	}
}

func (p *Process) Stop(hard bool, killHardTimeout time.Duration) (string, error) {
	p.mutex.RLock()
	ok, state, err := canStop(p.state)
	if !ok {
		defer p.mutex.RUnlock()
		return state.String(), err
	}
	pid := p.cmd.Process.Pid
	p.mutex.RUnlock()

	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		p.mutex.Lock()
		p.updateState(stopAction, noProcess, pid, noSignal, err)
		p.mutex.Unlock()
		return noProcess.String(), err
	}

	p.mutex.Lock()
	p.updateState(stopAction, stopping, pid, noSignal, nil)
	p.mutex.Unlock()

	signal := killSoftSignal
	if hard {
		signal = killHardSignal
	}

	err = killProcessGroup(pgid, signal)
	if err != nil {
		p.mutex.Lock()
		p.updateState(stopAction, killFailed, pgid, noSignal, err)
		p.mutex.Unlock()
		return killFailed.String(), err
	}

	go func() {
		s := <-p.wait(pgid, signal, hard, killHardTimeout)
		p.mutex.Lock()
		p.updateState(stopAction, s.state, pgid, signal, s.err)
		p.mutex.Unlock()
	}()

	/*
		done := make(chan error, 1)
		go func() {
			done <- p.cmd.Wait()
		}()

			select {
			// Kill hard because a timeout is reached
			case <-time.After(killHardTimeout):
				err := killProcessGroup(pgid, killHardSignal)
				if err != nil {
					p.mutex.Lock()
					p.updateState(stopAction, killFailed, pgid, noSignal, err)
					p.mutex.Unlock()
				}
				p.mutex.Lock()
				p.updateState(hardKillTimeoutAction, stopped, pgid, noSignal, nil)
				p.mutex.Unlock()

			case err := <-done:
				// Process completed
				if err != nil {
					if (soft && err.Error() != errSignalTerminated || !soft && err.Error() != errSignalKilled) &&
						err.Error() != errNoChildProcesses {
						p.mutex.Lock()
						p.updateState(stopAction, killFailed, pgid, noSignal, err)
						p.mutex.Unlock()
					}
				}

				p.mutex.Lock()
				logger.Info("stopppped")
				p.updateState(stopAction, stopped, pgid, signal, nil)
				p.mutex.Unlock()
			}*/

	return stopping.String(), nil
}

type stopState struct {
	state ProcessState
	err   error
}

func (p *Process) wait(pgid int, signal syscall.Signal, soft bool, killHardTimeout time.Duration) chan stopState {
	state := make(chan stopState, 1)
	done := make(chan error, 1)

	if killHardTimeout == 0 {
		killHardTimeout = defaultKillHardTimeout
	}

	go func() {
		done <- p.cmd.Wait()
	}()

	go func() {
		defer close(state)
		select {

		// Kill hard because a timeout is reached
		case <-time.After(killHardTimeout):
			err := killProcessGroup(pgid, killHardSignal)
			if err != nil {
				state <- stopState{killFailed, err}
				return
			}
			state <- stopState{stopped, nil}
			return

		// Process completed
		case err := <-done:
			if err != nil {
				if (soft && err.Error() != errSignalTerminated || !soft && err.Error() != errSignalKilled) &&
					err.Error() != errNoChildProcesses {
					state <- stopState{killFailed, err}
					return
				}
			}
			state <- stopState{stopped, nil}
			return
		}
	}()

	return state
}

func (p *Process) updateState(action string, state ProcessState, pid int, signal syscall.Signal, err error) {
	p.state = state
	if err != nil {
		logger.Error(err, fmt.Sprintf("error: %s process", action),
			"id", p.id, "state", state, "pid", pid, "signal", signal)
	} else {
		logger.Info(fmt.Sprintf("ok: %s process", action),
			"id", p.id, "state", state, "pid", pid, "signal", signal)
	}
}

func (p *Process) Status() (ProcessState, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return p.state, nil
}

func killProcessGroup(pgid int, signal syscall.Signal) error {
	err := syscall.Kill(-(pgid), signal)
	if err != nil && err.Error() != errNoChildProcesses {
		return err
	}

	return nil
}
