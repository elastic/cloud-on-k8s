// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_checkBackgroundTasksLicense(t *testing.T) {
	basic := license.MockLicenseChecker{EnterpriseEnabled: false}
	enterprise := license.MockLicenseChecker{EnterpriseEnabled: true}
	kb := kibanaWith("8.16.0", true)
	bgDeployment := &appsv1.Deployment{Namespace: kb.Namespace, Name: kbv1.BackgroundTasksDeployment(kb.Name)}
	empty := k8s.NewFakeClient()
	withDeployment := k8s.NewFakeClient(bgDeployment)
	failing := k8s.NewFailingClient(errors.New("lookup failed"))

	tests := []struct {
		name    string
		bg      bool
		client  k8s.Client
		checker license.Checker
		want    backgroundTasksState
		wantErr string
	}{
		{name: "no background tasks, basic license, deployment exists", bg: false, client: withDeployment, checker: basic, want: backgroundTasksDisabled},
		{name: "no background tasks, basic license, lookup would fail", bg: false, client: failing, checker: basic, want: backgroundTasksDisabled},
		{name: "background tasks, enterprise license, lookup would fail", bg: true, client: failing, checker: enterprise, want: backgroundTasksLicensed},
		{name: "background tasks, basic license, deployment exists", bg: true, client: withDeployment, checker: basic, want: backgroundTasksAlreadyEnabledUnlicensed},
		{name: "background tasks, basic license, no deployment", bg: true, client: empty, checker: basic, want: backgroundTasksNewlyEnabledUnlicensed},
		{name: "license check error is returned", bg: true, client: empty, checker: license.MockLicenseChecker{Err: errors.New("boom")}, wantErr: "boom"},
		{name: "deployment lookup error is returned", bg: true, client: failing, checker: basic, wantErr: "lookup failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkBackgroundTasksLicense(context.Background(), tt.client, kibanaWith("8.16.0", tt.bg), tt.checker)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
