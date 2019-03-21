// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package main

import (
	"fmt"
	"sync"
)

// ProcessController is a thread-safe controller of a set of processes.
type ProcessController struct {
	processes map[string]*Process
	lock      sync.Mutex
}

func (c ProcessController) Register(p *Process) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.processes[p.id] = p
}

func (c *ProcessController) StartAll() {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, p := range c.processes {
		_ = p.Start()
	}
}

func (c *ProcessController) StopAll() {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, p := range c.processes {
		_ = p.Stop(true)
	}
}

func (c *ProcessController) Start(id string) error {
	return c.exec(id, func(p *Process) error {
		return p.Start()
	})
}

func (c *ProcessController) Stop(id string, canBeStopped bool) error {
	return c.exec(id, func(p *Process) error {
		return p.Stop(canBeStopped)
	})
}

func (c *ProcessController) HardKill(id string) error {
	return c.exec(id, func(p *Process) error {
		return p.HardKill()
	})
}

func (c *ProcessController) Pgid(id string) (int, error) {
	return c.exec2(id, func(p *Process) (int, error) {
		return p.Pgid()
	})
}

func (c *ProcessController) exec2(id string, do func(p *Process) (int, error)) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	p, ok := c.processes[id]
	if !ok {
		return -1, fmt.Errorf("process %s not found", id)
	}

	return do(p)
}

func (c *ProcessController) exec(id string, do func(p *Process) error) error {
	_, err := c.exec2(id, func(p *Process) (int, error) {
		return -1, do(p)
	})
	return err
}
