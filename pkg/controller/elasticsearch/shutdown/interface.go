package shutdown

import "context"


type ShutdownStatus string

var (
	Started ShutdownStatus = "STARTED"
	Complete ShutdownStatus = "COMPLETE"
	Stalled ShutdownStatus = "STALLED"
	NotStarted ShutdownStatus = "NOT_STARTED"
)


type Interface interface {
	RequestShutdown(ctx context.Context, leavingNodes []string) error
	ShutdownStatus(ctx context.Context, podName string) (ShutdownStatus, error) 
} 
