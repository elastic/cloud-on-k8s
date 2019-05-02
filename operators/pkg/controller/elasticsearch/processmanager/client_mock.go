// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"context"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/keystore"
)

type MockClient struct {
	status ProcessStatus
	err    error
}

func NewMockClient(status ProcessStatus, err error) Client {
	return &MockClient{
		status: status,
		err:    err,
	}
}

func (m *MockClient) Start(ctx context.Context) (ProcessStatus, error) {
	return m.status, m.err
}
func (m *MockClient) Stop(ctx context.Context) (ProcessStatus, error) {
	return m.status, m.err
}
func (m *MockClient) Kill(ctx context.Context) (ProcessStatus, error) {
	return m.status, m.err
}
func (m *MockClient) Status(ctx context.Context) (ProcessStatus, error) {
	return m.status, m.err
}
func (m *MockClient) KeystoreStatus(ctx context.Context) (keystore.Status, error) {
	return keystore.Status{
		State:  keystore.RunningState,
		Reason: keystore.KeystoreUpdatedReason,
	}, m.err
}
