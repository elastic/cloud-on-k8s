package shutdown

import (
	"context"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)


type NodeShutdown struct {
	podToNodeID map[string]string
	c esclient.Client
	
}

var _ Interface = &NodeShutdown{}

func (ns *NodeShutdown) RequestShutdown(ctx context.Context, leavingNodes []string) error {
	return nil
}

func (ns *NodeShutdown) ShutdownStatus(ctx context.Context, podName string) (ShutdownStatus, error) {
return "", nil
}


func PrepareShutdown(
	ctx context.Context,
	es esv1.Elasticsearch,
	allocationSetter esclient.Client,
	leavingNodes []string,
) error {
return nil
}

