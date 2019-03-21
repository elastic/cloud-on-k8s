// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type clientV7 struct {
	clientV6
}

func (c *clientV7) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	return license.License, c.get(ctx, "/_license", &license)
}

func (c *clientV7) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	return response, c.post(ctx, "/_license", licenses, &response)
}

func (c *clientV7) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	if timeout == "" {
		timeout = DefaultVotingConfigExclusionsTimeout
	}
	path := fmt.Sprintf(
		"/_cluster/voting_config_exclusions/%s?timeout=%s",
		strings.Join(nodeNames, ","),
		timeout,
	)

	if err := c.post(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to add to voting_config_exclusions")
	}
	return nil
}

func (c *clientV7) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	path := fmt.Sprintf(
		"/_cluster/voting_config_exclusions?wait_for_removal=%s",
		strconv.FormatBool(waitForRemoval),
	)

	if err := c.delete(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to delete /_cluster/voting_config_exclusions")
	}
	return nil
}

func (c *clientV7) Equal(c2 Client) bool {
	other, ok := c2.(*clientV7)
	if !ok {
		return false
	}
	return c.baseClient.equal(&other.baseClient)
}

var _ Client = &clientV7{}
