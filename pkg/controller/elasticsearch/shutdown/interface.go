// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package shutdown

import "context"

type ShutdownStatus string

// TODO move to client?
type ShutdownResponse struct {
	Status ShutdownStatus
	Reason string
}

var (
	Started    ShutdownStatus = "STARTED"
	Complete   ShutdownStatus = "COMPLETE"
	Stalled    ShutdownStatus = "STALLED"
	NotStarted ShutdownStatus = "NOT_STARTED"
)

type Interface interface {
	RequestShutdown(ctx context.Context, leavingNodes []string) error
	ShutdownStatus(ctx context.Context, podName string) (ShutdownResponse, error)
}
