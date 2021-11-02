// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import "context"

type LicenseClient interface {
	// GetLicense returns the currently applied license. Can be empty.
	GetLicense(ctx context.Context) (License, error)
	// UpdateLicense attempts to update cluster license with the given licenses.
	UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error)
	// StartBasic creates or reverts to a basic license.
	StartBasic(ctx context.Context) (StartBasicResponse, error)
	// StartTrial starts a 30-day trial period (which gives access to platinum features).
	StartTrial(ctx context.Context) (StartTrialResponse, error)
}
