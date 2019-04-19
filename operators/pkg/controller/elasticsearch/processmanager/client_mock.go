// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import "context"

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
	if m.err != nil {
		return ProcessStatus{}, m.err
	}
	return m.status, nil
}
func (m *MockClient) Stop(ctx context.Context) (ProcessStatus, error) {
	if m.err != nil {
		return ProcessStatus{}, m.err
	}
	return m.status, nil
}
func (m *MockClient) Kill(ctx context.Context) (ProcessStatus, error) {
	if m.err != nil {
		return ProcessStatus{}, m.err
	}
	return m.status, nil
}
func (m *MockClient) Status(ctx context.Context) (ProcessStatus, error) {
	if m.err != nil {
		return ProcessStatus{}, m.err
	}
	return m.status, nil
}
