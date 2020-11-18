// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/pkg/errors"
)

type clientV7 struct {
	clientV6
}

func (c *clientV7) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	err := c.get(ctx, "/_license", &license)
	return license.License, err
}

func (c *clientV7) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	err := c.post(ctx, "/_license?acknowledge=true", licenses, &response)
	return response, err
}

func (c *clientV7) StartBasic(ctx context.Context) (StartBasicResponse, error) {
	var response StartBasicResponse
	err := c.post(ctx, "/_license/start_basic?acknowledge=true", nil, &response)
	return response, err
}

func (c *clientV7) StartTrial(ctx context.Context) (StartTrialResponse, error) {
	var response StartTrialResponse
	err := c.post(ctx, "/_license/start_trial?acknowledge=true", nil, &response)
	return response, err
}

func (c *clientV7) AddVotingConfigExclusions(ctx context.Context, nodeNames []string) error {
	var path string
	if c.version.IsSameOrAfter(version.From(7, 8, 0)) {
		path = fmt.Sprintf("/_cluster/voting_config_exclusions?node_names=%s", strings.Join(nodeNames, ","))
	} else {
		// versions < 7.8.0 or unversioned clients which is OK as this deprecated API will be supported until 8.0
		path = fmt.Sprintf("/_cluster/voting_config_exclusions/%s", strings.Join(nodeNames, ","))
	}

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
