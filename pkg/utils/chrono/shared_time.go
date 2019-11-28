// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package chrono

import (
	"sync"
	"time"
)

// SharedTime is a point in time shareable between multiple routines.
type SharedTime struct {
	until time.Time
	mutex sync.RWMutex
}

// SharedTime sets the point in time to now + dur.
func (d *SharedTime) Delay(dur time.Duration) {
	d.mutex.Lock()
	d.until = time.Now().Add(dur)
	d.mutex.Unlock()
}

// Ready checks whether the time has come.
func (d *SharedTime) Ready() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return time.Now().After(d.until)
}

// When returns the point in time until we want to delay.
func (d *SharedTime) When() time.Time {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.until
}
