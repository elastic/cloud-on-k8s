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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func kibanaWith(version string, bg bool) *kbv1.Kibana {
	kb := &kbv1.Kibana{
		Name:      "kb",
		Namespace: "ns",
		Spec:      kbv1.KibanaSpec{Version: version},
	}
	if bg {
		kb.Spec.BackgroundTasks = &kbv1.KibanaBackgroundTasks{}
	}
	return kb
}

func Test_webhookValidator_validate(t *testing.T) {
	basic := license.MockLicenseChecker{EnterpriseEnabled: false}
	enterprise := license.MockLicenseChecker{EnterpriseEnabled: true}
	kb := kibanaWith("8.16.0", true)
	bgDeployment := &appsv1.Deployment{Namespace: kb.Namespace, Name: kbv1.BackgroundTasksDeployment(kb.Name)}
	tests := []struct {
		name    string
		checker license.Checker
		objs    []client.Object
		old     *kbv1.Kibana
		new     *kbv1.Kibana
		wantErr string
	}{
		{name: "create without background tasks", checker: basic, new: kibanaWith("8.16.0", false)},
		{name: "create with background tasks, basic license", checker: basic, new: kibanaWith("8.16.0", true), wantErr: backgroundTasksLicenseRejectMsg},
		{name: "create with background tasks, basic license, deployment exists", checker: basic, objs: []client.Object{bgDeployment}, new: kibanaWith("8.16.0", true), wantErr: backgroundTasksLicenseRejectMsg},
		{name: "create with background tasks, enterprise license", checker: enterprise, new: kibanaWith("8.16.0", true)},
		{name: "update enabling background tasks, basic license", checker: basic, old: kibanaWith("8.16.0", false), new: kibanaWith("8.16.0", true), wantErr: backgroundTasksLicenseRejectMsg},
		{name: "update keeping background tasks, basic license, deployment exists", checker: basic, objs: []client.Object{bgDeployment}, old: kibanaWith("8.16.0", true), new: func() *kbv1.Kibana {
			kb := kibanaWith("8.16.0", true)
			kb.Spec.Count = 2
			return kb
		}(), wantErr: backgroundTasksLicenseRejectMsg},
		{name: "update keeping background tasks, basic license, deployment missing", checker: basic, old: kibanaWith("8.16.0", true), new: kibanaWith("8.16.0", true), wantErr: backgroundTasksLicenseRejectMsg},
		{name: "update keeping background tasks, enterprise license", checker: enterprise, old: kibanaWith("8.16.0", true), new: kibanaWith("8.16.0", true)},
		{name: "update disabling background tasks, basic license", checker: basic, objs: []client.Object{bgDeployment}, old: kibanaWith("8.16.0", true), new: kibanaWith("8.16.0", false)},
		{name: "update enabling background tasks, enterprise license", checker: enterprise, old: kibanaWith("8.16.0", false), new: kibanaWith("8.16.0", true)},
		{name: "license check error fails open", checker: license.MockLicenseChecker{Err: errors.New("boom")}, new: kibanaWith("8.16.0", true)},
		{name: "unsupported version is rejected by structural validation", checker: enterprise, new: kibanaWith("8.15.0", true), wantErr: "requires Kibana >= 8.16.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &webhookValidator{client: k8s.NewFakeClient(tt.objs...), licenseChecker: tt.checker}
			var warnings admission.Warnings
			var err error
			if tt.old == nil {
				warnings, err = v.ValidateCreate(context.Background(), tt.new)
			} else {
				warnings, err = v.ValidateUpdate(context.Background(), tt.old, tt.new)
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Empty(t, warnings)
		})
	}
}

func Test_webhookValidator_lookupErrorFailsOpen(t *testing.T) {
	v := &webhookValidator{client: k8s.NewFailingClient(errors.New("lookup failed")), licenseChecker: license.MockLicenseChecker{EnterpriseEnabled: false}}
	warnings, err := v.ValidateCreate(context.Background(), kibanaWith("8.16.0", true))
	require.NoError(t, err)
	require.Empty(t, warnings)
}
