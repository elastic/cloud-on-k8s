// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type clientV8 struct {
	clientV7
}

func (c *clientV8) AddVotingConfigExclusions(ctx context.Context, nodeNames []string) error {
	path := fmt.Sprintf("/_cluster/voting_config_exclusions?node_names=%s", strings.Join(nodeNames, ","))

	if err := c.post(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to add to voting_config_exclusions")
	}
	return nil
}

func (c *clientV8) SyncedFlush(_ context.Context) error {
	return errors.New("synced flush is not supported in Elasticsearch 8.x")
}

func (c *clientV8) GetClusterState(ctx context.Context) (ClusterState, error) {
	var response ClusterState
	err := c.get(ctx, "/_cluster/state", &response)
	return response, err
}

// Equal returns true if c2 can be considered the same as c
func (c *clientV8) Equal(c2 Client) bool {
	other, ok := c2.(*clientV8)
	if !ok {
		return false
	}
	return c.baseClient.equal(&other.baseClient)
}

var _ Client = &clientV8{}
