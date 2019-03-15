package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	killHardTimeout     = 5 * time.Second
	killSoftSignal      = syscall.SIGTERM
	killHardSignal      = syscall.SIGKILL
	errNoSuchProcess    = "no such process"
	errNoChildProcesses = "waitid: no child processes"
)

type Process struct {
	id   string
	name string
	args []string
	cmd  *exec.Cmd
}

func (p *Process) isStarted() bool {
	pgid, _ := p.Pgid()
	return pgid != -1
}

func (p *Process) Start() error {
	if p.isStarted() {
		return fmt.Errorf("cannot start process %s already started", p.id)
	}

	cmd := exec.Command(p.name, p.args...)

	// Dedicated process group to forward signals to main process and all children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info("Start", "process", p.id, "args", strings.Join(p.args, " "))

	if err := cmd.Start(); err != nil {
		logger.Error(err, "Failed to start", "process", p.id)
		return err
	}

	// Set cmd
	p.cmd = cmd

	logger.Info("Started successfully", "process", p.id)

	return nil
}

func (p *Process) Stop(canBeStopped bool) error {
	if !p.isStarted() {
		if !canBeStopped {
			return fmt.Errorf("cannot stop process %s already stopped", p.id)
		} else {
			return nil
		}
	}

	cmd := p.cmd
	defer func() {
		// Reset cmd
		p.cmd = nil
	}()

	pgid, err := p.Pgid()
	if err != nil {
		return err
	}

	logger.Info("Stop", "process", p.id, "pid", p.cmd.Process.Pid, "group", pgid)

	err = syscall.Kill(-(pgid), killSoftSignal)
	if err != nil && err.Error() != errNoChildProcesses {
		logger.Error(err, "Failed to kill soft", "process", p.id, "group", pgid)
	}

	// Wait
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	// Kill hard because a timeout is reached
	case <-time.After(killHardTimeout):
		if err = syscall.Kill(-(pgid), killHardSignal); err != nil {
			logger.Error(err, "Failed to kill hard", "process", p.id, "pgid", pgid)
			return err
		}
		logger.Info("Kill as timeout reached", "process", p.id, "pgid", pgid)
		return nil

	// Kill hard to be sure to clean up every children processes
	case err := <-done:
		if err != nil && err.Error() != errNoChildProcesses {
			logger.Error(err, "Failed to kill soft", "process", p.id, "pgid", pgid)
		}

		err = syscall.Kill(-(pgid), killHardSignal)
		if err != nil {
			logger.Error(err, "Failed to kill hard (2)", "process", p.id, "pgid", pgid)
			return err
		}

		logger.Info("Stopped successfully", "process", p.id, "pgid", pgid)
		return nil
	}
}

func (p *Process) Pgid() (int, error) {
	if p.cmd == nil {
		return -1, fmt.Errorf("no process %s", p.id)
	}

	pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
	if err != nil {
		if err.Error() == errNoSuchProcess {
			return -1, fmt.Errorf("no process %s", p.id)
		}
		logger.Error(err, "Failed to get pgid", "process", p.id)
		return -1, err
	}

	return pgid, nil
}

func (p *Process) HardKill() error {
	pgid, err := p.Pgid()
	if err != nil {
		return err
	}

	logger.Info("Kill", "process", p.id, "pid", p.cmd.Process.Pid, "group", pgid)
	err = syscall.Kill(-(pgid), killHardSignal)
	if err != nil {
		logger.Info("Fail to kill", "process", p.id, "pid", p.cmd.Process.Pid, "group", pgid)
		return err
	}

	return nil
}
