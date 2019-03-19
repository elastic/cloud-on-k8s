// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sync

import (
	gosync "sync"
	"sync/atomic"
)

// SuccessOnce is an object that will perform an action until it was exactly once successful.
type SuccessOnce struct {
	m    gosync.Mutex
	done uint32
}

// Do calls the function f if and only if Do is being called for the
// first time for this instance of SuccessOnce or previous call have
// returned errors.
func (o *SuccessOnce) Do(f func() error) error {
	if atomic.LoadUint32(&o.done) == 1 {
		return nil
	}
	// Slow-path.
	o.m.Lock()
	defer o.m.Unlock()
	if o.done == 0 {
		if err := f(); err != nil {
			return err
		}
		atomic.StoreUint32(&o.done, 1)
	}
	return nil
}
