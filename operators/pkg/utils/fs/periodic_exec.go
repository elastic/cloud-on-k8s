// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"time"
)

// defaultExecPeriodicity is the default periodicity at which execFunction is executed.
const defaultPeriodicity = 1 * time.Second

// execFunction is a function to be executed periodically.
// If it returns an error, periodicExec will stop watching and return the error.
// If it returns true, periodicExec will stop watching with no error.
type execFunction func() (done bool, err error)

// periodicExec periodically executes a function.
type periodicExec struct {
	periodicity time.Duration
	stop        chan struct{}
	exec        execFunction
}

// newPeriodicExec creates a periodicExec with the default periodicity.
func newPeriodicExec(exec execFunction) *periodicExec {
	return &periodicExec{
		periodicity: defaultPeriodicity,
		stop:        make(chan struct{}),
		exec:        exec,
	}
}

// SetPeriodicity overrides the execution periodicity.
// It has no effect on a running periodicExec.
func (p *periodicExec) SetPeriodicity(periodicity time.Duration) {
	p.periodicity = periodicity
}

// Run the function periodically, until stopped, error or done.
func (p *periodicExec) Run() error {
	ticker := time.NewTicker(p.periodicity)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return nil
		case <-ticker.C:
			done, err := p.exec()
			if err != nil || done {
				return err
			}
		}
	}
}

// Stop the periodic execution (waiting for current one to be done first).
func (p *periodicExec) Stop() {
	p.stop <- struct{}{}
}
