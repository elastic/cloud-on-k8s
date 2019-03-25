// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"context"
	"os"
	"os/signal"
	"sync"

	"golang.org/x/sys/unix"
)

// ProcessReaper is a reaper of child processes.
type ProcessReaper struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     *sync.WaitGroup
}

// NewProcessReaper creates a new reaper of child processes.
func NewProcessReaper() *ProcessReaper {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	return &ProcessReaper{
		ctx:    ctx,
		cancel: cancel,
		wg:     &wg,
	}
}

// Start starts to reap child processes.
func (d *ProcessReaper) Start() {
	d.wg.Add(1)
	go d.removeZombies()
	log.Info("Process reaper started")
}

// Stops stops the reaper of child processes.
func (d *ProcessReaper) Stop() {
	d.cancel()
	d.wg.Wait()
	log.Info("Process reaper stopped")
}

// removeZombies is a long-running routine that blocks waiting for child
// processes to exit and reaps them.
// Forked from https://github.com/hashicorp/go-reap/blob/master/reap_unix.go.
func (d *ProcessReaper) removeZombies() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, unix.SIGCHLD)

	for {
		// Block for an incoming signal that a child has exited.
		select {
		case <-c:
			// Got a child signal, drop out and reap.
		case <-d.ctx.Done():
			d.wg.Done()
			return
		}

		// Attempt to reap all abandoned child processes after getting
		// the reap lock, which makes sure the application isn't doing
		// any waiting of its own. Note that we do the full write lock
		// here.
		func() {
		POLL:
			// Try to reap children until there aren't any more. We
			// never block in here so that we are always responsive
			// to signals, at the expense of possibly leaving a
			// child behind if we get here too quickly. Any
			// stragglers should get reaped the next time we see a
			// signal, so we won't leak in the long run.
			var status unix.WaitStatus
			pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
			switch err {
			case nil:
				// Got a child, clean this up and poll again.
				if pid > 0 {
					log.Info("reap a child", "pid", pid)
					goto POLL
				}
				return

			case unix.ECHILD:
				// No more children, we are done.
				return

			case unix.EINTR:
				// We got interrupted, try again. This likely
				// can't happen since we are calling Wait4 in a
				// non-blocking fashion, but it's good to be
				// complete and handle this case rather than
				// fail.
				goto POLL

			default:
				// We got some other error we didn't expect.
				// Wait for another SIGCHLD so we don't
				// potentially spam in here and chew up CPU.
				return
			}
		}()
	}
}
