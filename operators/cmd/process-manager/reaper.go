package main

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
	"os/signal"
	"sync"
)

// ProcessReaper is a reaper of child processes.
type ProcessReaper struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewProcessReaper() *ProcessReaper {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	return &ProcessReaper{
		ctx:    ctx,
		cancel: cancel,
		wg:     wg,
	}
}

func (d *ProcessReaper) Start() {
	d.wg.Add(1)
	go d.removeZombies()
	logger.Info("Process reaper started")
}

func (d *ProcessReaper) Stop() {
	d.cancel()
	d.wg.Wait()
	logger.Info("Process reaper stopped")
}

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
			/*if reapLock != nil {
				reapLock.Lock()
				defer reapLock.Unlock()
			}*/

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
					logger.Info("reap a child", "pid", pid)
					/*if pids != nil {
						pids <- pid
					}*/
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
				/*if errors != nil {
					errors <- err
				}*/
				return
			}
		}()
	}
}
